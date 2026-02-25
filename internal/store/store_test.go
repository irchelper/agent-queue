package store_test

import (
	"fmt"
	"os"
	"sync"
	"testing"

	"github.com/irchelper/agent-queue/internal/db"
	"github.com/irchelper/agent-queue/internal/model"
	"github.com/irchelper/agent-queue/internal/store"
)

// openTestDB creates a temporary SQLite database for each test.
func openTestDB(t *testing.T) *store.Store {
	t.Helper()
	f, err := os.CreateTemp("", "agent-queue-test-*.db")
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	f.Close()
	t.Cleanup(func() { os.Remove(f.Name()) })

	database, err := db.Open(f.Name())
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	return store.New(database)
}

func makeTask(t *testing.T, s *store.Store, title string) model.Task {
	t.Helper()
	task, err := s.CreateTask(model.CreateTaskRequest{Title: title})
	if err != nil {
		t.Fatalf("create task %q: %v", title, err)
	}
	return task
}

// -------------------------------------------------------------------
// CRUD
// -------------------------------------------------------------------

func TestCreateAndGet(t *testing.T) {
	s := openTestDB(t)
	task, err := s.CreateTask(model.CreateTaskRequest{
		Title:       "hello",
		Description: "world",
		DependsOn:   nil,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if task.ID == "" {
		t.Fatal("expected non-empty ID")
	}
	if task.Status != model.StatusPending {
		t.Fatalf("expected pending, got %s", task.Status)
	}
	if task.Version != 1 {
		t.Fatalf("expected version=1, got %d", task.Version)
	}

	got, err := s.GetByID(task.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Title != "hello" {
		t.Fatalf("title mismatch: %s", got.Title)
	}
}

func TestGetNotFound(t *testing.T) {
	s := openTestDB(t)
	_, err := s.GetByID("nonexistent")
	if !store.IsNotFound(err) {
		t.Fatalf("expected not-found, got %v", err)
	}
}

func TestListTasks_FilterByStatus(t *testing.T) {
	s := openTestDB(t)
	makeTask(t, s, "t1")
	makeTask(t, s, "t2")

	tasks, err := s.ListTasks("pending", "", "", nil)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(tasks) != 2 {
		t.Fatalf("expected 2 pending tasks, got %d", len(tasks))
	}

	tasks, err = s.ListTasks("done", "", "", nil)
	if err != nil {
		t.Fatalf("list done: %v", err)
	}
	if len(tasks) != 0 {
		t.Fatalf("expected 0 done tasks, got %d", len(tasks))
	}
}

func TestDeleteTask(t *testing.T) {
	s := openTestDB(t)
	task := makeTask(t, s, "delete-me")

	if err := s.DeleteTask(task.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	_, err := s.GetByID(task.ID)
	if !store.IsNotFound(err) {
		t.Fatalf("expected not-found after delete, got %v", err)
	}
}

// -------------------------------------------------------------------
// F2: Optimistic lock claim
// -------------------------------------------------------------------

func TestClaim_Success(t *testing.T) {
	s := openTestDB(t)
	task := makeTask(t, s, "claim-me")

	claimed, err := s.Claim(task.ID, task.Version, "agent-1")
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if claimed.Status != model.StatusClaimed {
		t.Fatalf("expected claimed, got %s", claimed.Status)
	}
	if claimed.AssignedTo != "agent-1" {
		t.Fatalf("expected agent-1, got %s", claimed.AssignedTo)
	}
	if claimed.Version != task.Version+1 {
		t.Fatalf("version not incremented: %d", claimed.Version)
	}
}

func TestClaim_WrongVersion_ReturnsConflict(t *testing.T) {
	s := openTestDB(t)
	task := makeTask(t, s, "conflict-me")

	_, err := s.Claim(task.ID, task.Version+99, "agent-x")
	if !store.IsConflict(err) {
		t.Fatalf("expected conflict, got %v", err)
	}
}

func TestClaim_AlreadyClaimed_ReturnsConflict(t *testing.T) {
	s := openTestDB(t)
	task := makeTask(t, s, "double-claim")

	if _, err := s.Claim(task.ID, task.Version, "agent-1"); err != nil {
		t.Fatalf("first claim: %v", err)
	}
	// Second claim with the original version should conflict.
	_, err := s.Claim(task.ID, task.Version, "agent-2")
	if !store.IsConflict(err) {
		t.Fatalf("expected conflict on second claim, got %v", err)
	}
}

func TestClaim_Concurrent_OnlyOneSucceeds(t *testing.T) {
	s := openTestDB(t)
	task := makeTask(t, s, "concurrent")
	ver := task.Version

	const n = 10
	results := make([]error, n)
	var wg sync.WaitGroup

	for i := 0; i < n; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, results[i] = s.Claim(task.ID, ver, fmt.Sprintf("agent-%d", i))
		}()
	}
	wg.Wait()

	successes := 0
	for _, err := range results {
		if err == nil {
			successes++
		} else if !store.IsConflict(err) {
			t.Errorf("unexpected error: %v", err)
		}
	}
	if successes != 1 {
		t.Fatalf("expected exactly 1 successful claim, got %d", successes)
	}
}

// -------------------------------------------------------------------
// F4: State machine via PatchTask
// -------------------------------------------------------------------

func TestPatch_ValidTransition(t *testing.T) {
	s := openTestDB(t)
	task := makeTask(t, s, "state-test")

	// pending → claimed (via Claim)
	claimed, err := s.Claim(task.ID, task.Version, "agent-1")
	if err != nil {
		t.Fatalf("claim: %v", err)
	}

	// claimed → in_progress
	st := model.StatusInProgress
	resp, _, err := s.PatchTask(claimed.ID, model.PatchTaskRequest{
		Status:  &st,
		Version: claimed.Version,
	})
	if err != nil {
		t.Fatalf("patch in_progress: %v", err)
	}
	if resp.Status != model.StatusInProgress {
		t.Fatalf("expected in_progress, got %s", resp.Status)
	}

	// in_progress → done (requires_review=false by default)
	st2 := model.StatusDone
	result := "finished"
	resp2, _, err := s.PatchTask(resp.ID, model.PatchTaskRequest{
		Status:  &st2,
		Result:  &result,
		Version: resp.Version,
	})
	if err != nil {
		t.Fatalf("patch done: %v", err)
	}
	if resp2.Status != model.StatusDone {
		t.Fatalf("expected done, got %s", resp2.Status)
	}
	if resp2.Result != "finished" {
		t.Fatalf("result not saved: %q", resp2.Result)
	}
}

func TestPatch_InvalidTransition_Returns422(t *testing.T) {
	s := openTestDB(t)
	task := makeTask(t, s, "invalid-transition")

	// pending → done is NOT allowed (skip states).
	st := model.StatusDone
	_, _, err := s.PatchTask(task.ID, model.PatchTaskRequest{
		Status:  &st,
		Version: task.Version,
	})
	if err == nil {
		t.Fatal("expected error for invalid transition, got nil")
	}
}

func TestPatch_RequiresReview_BlocksDirectDone(t *testing.T) {
	s := openTestDB(t)
	task, err := s.CreateTask(model.CreateTaskRequest{
		Title:          "needs-review",
		RequiresReview: true,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	// Advance to in_progress.
	claimed, _ := s.Claim(task.ID, task.Version, "agent-1")
	st := model.StatusInProgress
	inProg, _, _ := s.PatchTask(claimed.ID, model.PatchTaskRequest{
		Status: &st, Version: claimed.Version,
	})

	// in_progress → done must be blocked.
	st2 := model.StatusDone
	_, _, err = s.PatchTask(inProg.ID, model.PatchTaskRequest{
		Status: &st2, Version: inProg.Version,
	})
	if err == nil {
		t.Fatal("expected error: requires_review=true should block in_progress→done")
	}
}

func TestPatch_RequiresReview_AllowsReviewPath(t *testing.T) {
	s := openTestDB(t)
	task, _ := s.CreateTask(model.CreateTaskRequest{
		Title:          "review-path",
		RequiresReview: true,
	})

	claimed, _ := s.Claim(task.ID, task.Version, "agent-1")
	st := model.StatusInProgress
	inProg, _, _ := s.PatchTask(claimed.ID, model.PatchTaskRequest{
		Status: &st, Version: claimed.Version,
	})

	// in_progress → review (allowed when requires_review=true)
	st2 := model.StatusReview
	reviewed, _, err := s.PatchTask(inProg.ID, model.PatchTaskRequest{
		Status: &st2, Version: inProg.Version,
	})
	if err != nil {
		t.Fatalf("patch review: %v", err)
	}

	// review → done
	st3 := model.StatusDone
	done, _, err := s.PatchTask(reviewed.ID, model.PatchTaskRequest{
		Status: &st3, Version: reviewed.Version,
	})
	if err != nil {
		t.Fatalf("patch done from review: %v", err)
	}
	if done.Status != model.StatusDone {
		t.Fatalf("expected done, got %s", done.Status)
	}
}

func TestPatch_TimeoutRelease_ClearsAssignedTo(t *testing.T) {
	s := openTestDB(t)
	task := makeTask(t, s, "timeout-test")
	claimed, _ := s.Claim(task.ID, task.Version, "agent-1")
	st := model.StatusInProgress
	inProg, _, _ := s.PatchTask(claimed.ID, model.PatchTaskRequest{
		Status: &st, Version: claimed.Version,
	})

	// in_progress → pending (timeout/release)
	rel := model.StatusPending
	released, _, err := s.PatchTask(inProg.ID, model.PatchTaskRequest{
		Status: &rel, Version: inProg.Version,
		Note: "timeout",
	})
	if err != nil {
		t.Fatalf("release: %v", err)
	}
	if released.AssignedTo != "" {
		t.Fatalf("expected assigned_to cleared, got %q", released.AssignedTo)
	}
}

func TestPatch_VersionConflict(t *testing.T) {
	s := openTestDB(t)
	task := makeTask(t, s, "version-conflict")
	st := model.StatusCancelled

	_, _, err := s.PatchTask(task.ID, model.PatchTaskRequest{
		Status:  &st,
		Version: task.Version + 10, // wrong version
	})
	if !store.IsConflict(err) {
		t.Fatalf("expected conflict, got %v", err)
	}
}

// -------------------------------------------------------------------
// F3: Dependency auto-advance
// -------------------------------------------------------------------

func TestDepsAutoAdvance_Serial(t *testing.T) {
	s := openTestDB(t)

	taskA := makeTask(t, s, "A")
	taskB, err := s.CreateTask(model.CreateTaskRequest{
		Title:     "B",
		DependsOn: []string{taskA.ID},
	})
	if err != nil {
		t.Fatalf("create B: %v", err)
	}
	taskC, err := s.CreateTask(model.CreateTaskRequest{
		Title:     "C",
		DependsOn: []string{taskB.ID},
	})
	if err != nil {
		t.Fatalf("create C: %v", err)
	}

	// B and C should not be deps_met yet.
	metB, _ := s.DepsMet(taskB.ID)
	if metB {
		t.Fatal("B deps should not be met before A is done")
	}

	// Advance A → done.
	claimedA, _ := s.Claim(taskA.ID, taskA.Version, "agent-1")
	st1 := model.StatusInProgress
	inProgA, _, _ := s.PatchTask(claimedA.ID, model.PatchTaskRequest{
		Status: &st1, Version: claimedA.Version,
	})
	st2 := model.StatusDone
	_, triggered, err := s.PatchTask(inProgA.ID, model.PatchTaskRequest{
		Status: &st2, Version: inProgA.Version,
	})
	if err != nil {
		t.Fatalf("A→done: %v", err)
	}
	// B should be in triggered list.
	found := false
	for _, tid := range triggered {
		if tid == taskB.ID {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected B in triggered list, got %v", triggered)
	}

	// Now B deps should be met.
	metB, _ = s.DepsMet(taskB.ID)
	if !metB {
		t.Fatal("B deps should be met after A is done")
	}
	// C still not met.
	metC, _ := s.DepsMet(taskC.ID)
	if metC {
		t.Fatal("C deps should not be met yet")
	}

	// Advance B → done.
	claimedB, _ := s.Claim(taskB.ID, taskB.Version, "agent-2")
	st3 := model.StatusInProgress
	inProgB, _, _ := s.PatchTask(claimedB.ID, model.PatchTaskRequest{
		Status: &st3, Version: claimedB.Version,
	})
	st4 := model.StatusDone
	_, triggeredB, _ := s.PatchTask(inProgB.ID, model.PatchTaskRequest{
		Status: &st4, Version: inProgB.Version,
	})

	foundC := false
	for _, tid := range triggeredB {
		if tid == taskC.ID {
			foundC = true
		}
	}
	if !foundC {
		t.Fatalf("expected C in triggered list after B done, got %v", triggeredB)
	}

	metC, _ = s.DepsMet(taskC.ID)
	if !metC {
		t.Fatal("C deps should be met after B is done")
	}
}

func TestDepsMet_ParallelFanIn(t *testing.T) {
	s := openTestDB(t)
	p1 := makeTask(t, s, "P1")
	p2 := makeTask(t, s, "P2")
	p3 := makeTask(t, s, "P3")

	summary, err := s.CreateTask(model.CreateTaskRequest{
		Title:     "Summary",
		DependsOn: []string{p1.ID, p2.ID, p3.ID},
	})
	if err != nil {
		t.Fatalf("create summary: %v", err)
	}

	metSummary, _ := s.DepsMet(summary.ID)
	if metSummary {
		t.Fatal("summary deps should not be met yet")
	}

	doneThem := func(task model.Task) {
		claimed, _ := s.Claim(task.ID, task.Version, "a")
		st := model.StatusInProgress
		ip, _, _ := s.PatchTask(claimed.ID, model.PatchTaskRequest{Status: &st, Version: claimed.Version})
		st2 := model.StatusDone
		s.PatchTask(ip.ID, model.PatchTaskRequest{Status: &st2, Version: ip.Version}) //nolint:errcheck
	}

	doneThem(p1)
	doneThem(p2)
	metSummary, _ = s.DepsMet(summary.ID)
	if metSummary {
		t.Fatal("summary should not be met until P3 is done too")
	}
	doneThem(p3)
	metSummary, _ = s.DepsMet(summary.ID)
	if !metSummary {
		t.Fatal("summary deps should be met once all P1/P2/P3 done")
	}
}

func TestListTasks_DepsMet_Filter(t *testing.T) {
	s := openTestDB(t)
	taskA := makeTask(t, s, "A-filter")
	taskB, _ := s.CreateTask(model.CreateTaskRequest{
		Title:     "B-filter",
		DependsOn: []string{taskA.ID},
	})

	trueVal := true
	tasks, err := s.ListTasks("", "", "", &trueVal)
	if err != nil {
		t.Fatalf("list deps_met=true: %v", err)
	}
	// A has no deps → deps_met=true. B depends on A → deps_met=false.
	for _, task := range tasks {
		if task.ID == taskB.ID {
			t.Fatalf("B should not appear in deps_met=true list")
		}
	}

	falseVal := false
	tasks, err = s.ListTasks("", "", "", &falseVal)
	if err != nil {
		t.Fatalf("list deps_met=false: %v", err)
	}
	foundB := false
	for _, task := range tasks {
		if task.ID == taskB.ID {
			foundB = true
		}
	}
	if !foundB {
		t.Fatalf("B should appear in deps_met=false list")
	}
}
