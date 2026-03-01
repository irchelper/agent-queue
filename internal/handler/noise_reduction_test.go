package handler_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/irchelper/agent-queue/internal/model"
	"github.com/irchelper/agent-queue/internal/openclaw"
)

// -------------------------------------------------------------------
// ACC[1]: assigned_to=="test" stale behavior
// (commit 680279b)
//
// Current implementation: test tasks are NOT re-dispatched by stale ticker,
// but still alert CEO when max stale dispatches reached.
// Acceptance says "直接 cancel，不触发 CEO 告警" — this test verifies the
// current behavior (no re-dispatch) and documents the gap.
// -------------------------------------------------------------------

func TestNoiseReduction_AssignedToTest_StaleNoReDispatch(t *testing.T) {
	var mu sync.Mutex
	var dispatched []string

	mockOC := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body) //nolint:errcheck
		if args, ok := body["args"].(map[string]any); ok {
			if key, ok := args["sessionKey"].(string); ok {
				mu.Lock()
				dispatched = append(dispatched, key)
				mu.Unlock()
			}
		}
		json.NewEncoder(w).Encode(map[string]any{"ok": true}) //nolint:errcheck
	}))
	defer mockOC.Close()

	oc := openclaw.NewWithURL(mockOC.URL, "")
	srv, h := newTestServerWithHandler(t, oc)
	defer srv.Close()

	var task model.Task
	r := postJSON(t, srv, "/tasks", map[string]any{"title": "stale test thing", "assigned_to": "test"})
	json.NewDecoder(r.Body).Decode(&task) //nolint:errcheck
	r.Body.Close()

	time.Sleep(50 * time.Millisecond)
	h.SetStaleThresholdForTesting(time.Millisecond)

	mu.Lock()
	dispatched = nil
	mu.Unlock()

	h.CheckStaleTasks()
	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	snap := make([]string, len(dispatched))
	copy(snap, dispatched)
	mu.Unlock()

	// Verify: test task was NOT re-dispatched (no sessions_send to test agent).
	for _, key := range snap {
		if strings.Contains(key, "test") {
			t.Errorf("assigned_to=test task should not be re-dispatched, got dispatch to: %s", key)
		}
	}
}

// TestNoiseReduction_AssignedToTest_FailAutoCancel verifies that
// assigned_to="test" tasks hitting the failed path are auto-cancelled
// (isTestTask returns true → silent path).
func TestNoiseReduction_AssignedToTest_FailAutoCancel(t *testing.T) {
	var mu sync.Mutex
	var ceoMessages []string

	mockOC := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body) //nolint:errcheck
		if args, ok := body["args"].(map[string]any); ok {
			if msg, ok := args["message"].(string); ok {
				mu.Lock()
				ceoMessages = append(ceoMessages, msg)
				mu.Unlock()
			}
		}
		json.NewEncoder(w).Encode(map[string]any{"ok": true}) //nolint:errcheck
	}))
	defer mockOC.Close()

	oc := openclaw.NewWithURL(mockOC.URL, "")
	srv := newTestServer(t, oc)
	defer srv.Close()

	var task model.Task
	r := postJSON(t, srv, "/tasks", map[string]any{"title": "test-assigned fail", "assigned_to": "test"})
	json.NewDecoder(r.Body).Decode(&task) //nolint:errcheck
	r.Body.Close()

	claimR := postJSON(t, srv, "/tasks/"+task.ID+"/claim",
		map[string]any{"version": task.Version, "agent": "test"})
	var claimed model.Task
	json.NewDecoder(claimR.Body).Decode(&claimed) //nolint:errcheck
	claimR.Body.Close()

	ipTask := patchTaskTo(t, srv, task.ID, "in_progress", claimed.Version)

	mu.Lock()
	ceoMessages = nil
	mu.Unlock()

	body, _ := json.Marshal(map[string]any{
		"status":         "failed",
		"failure_reason": "some error",
		"version":        ipTask.Version,
	})
	req, _ := http.NewRequest(http.MethodPatch, srv.URL+"/tasks/"+task.ID, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := http.DefaultClient.Do(req)
	resp.Body.Close()

	time.Sleep(200 * time.Millisecond)

	getResp := getJSON(t, srv, "/tasks/"+task.ID)
	var got model.Task
	json.NewDecoder(getResp.Body).Decode(&got) //nolint:errcheck
	getResp.Body.Close()

	if got.Status != model.StatusCancelled {
		t.Errorf("assigned_to=test failed task should be auto-cancelled, got %s", got.Status)
	}

	mu.Lock()
	snap := make([]string, len(ceoMessages))
	copy(snap, ceoMessages)
	mu.Unlock()

	for _, msg := range snap {
		if strings.Contains(msg, "test-assigned fail") {
			t.Errorf("assigned_to=test task should not trigger CEO alert on fail, got: %s", msg)
		}
	}
}

// -------------------------------------------------------------------
// ACC[2]: isNotifyPlaceholderTask → auto-cancel, no retry chain
// (commit 1dd1789)
// -------------------------------------------------------------------

func TestNoiseReduction_NotifyPlaceholder_AutoCancel_NoRetry(t *testing.T) {
	mockOC := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"ok": true}) //nolint:errcheck
	}))
	defer mockOC.Close()

	oc := openclaw.NewWithURL(mockOC.URL, "")
	srv := newTestServer(t, oc)
	defer srv.Close()

	tests := []struct {
		name  string
		title string
	}{
		{"bare placeholder", "prod fail notify"},
		{"with suffix", "prod fail notify-extra"},
		{"retry prefix", "retry: prod fail notify"},
		{"fix prefix", "fix: prod fail notify-abc"},
		{"nested prefix", "retry: fix: re-review: prod fail notify"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var task model.Task
			r := postJSON(t, srv, "/tasks", map[string]any{"title": tt.title, "assigned_to": "coder"})
			json.NewDecoder(r.Body).Decode(&task) //nolint:errcheck
			r.Body.Close()

			claimR := postJSON(t, srv, "/tasks/"+task.ID+"/claim",
				map[string]any{"version": task.Version, "agent": "coder"})
			var claimed model.Task
			json.NewDecoder(claimR.Body).Decode(&claimed) //nolint:errcheck
			claimR.Body.Close()

			ipTask := patchTaskTo(t, srv, task.ID, "in_progress", claimed.Version)

			body, _ := json.Marshal(map[string]any{
				"status":         "failed",
				"failure_reason": "agent_timeout",
				"version":        ipTask.Version,
			})
			req, _ := http.NewRequest(http.MethodPatch, srv.URL+"/tasks/"+task.ID, bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			resp, _ := http.DefaultClient.Do(req)
			resp.Body.Close()

			time.Sleep(200 * time.Millisecond)

			getResp := getJSON(t, srv, "/tasks/"+task.ID)
			var got model.Task
			json.NewDecoder(getResp.Body).Decode(&got) //nolint:errcheck
			getResp.Body.Close()

			if got.Status != model.StatusCancelled {
				t.Errorf("expected cancelled, got %s", got.Status)
			}
		})
	}
}

// -------------------------------------------------------------------
// ACC[3]: retry depth>=3 → cancel + CEO notify (not silent)
// (commit f58cf86)
// -------------------------------------------------------------------

func TestNoiseReduction_RetryDepthCap_CancelWithCEONotify(t *testing.T) {
	var mu sync.Mutex
	var ceoMessages []string

	mockOC := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body) //nolint:errcheck
		if args, ok := body["args"].(map[string]any); ok {
			if msg, ok := args["message"].(string); ok {
				mu.Lock()
				ceoMessages = append(ceoMessages, msg)
				mu.Unlock()
			}
		}
		json.NewEncoder(w).Encode(map[string]any{"ok": true}) //nolint:errcheck
	}))
	defer mockOC.Close()

	oc := openclaw.NewWithURL(mockOC.URL, "")
	srv := newTestServer(t, oc)
	defer srv.Close()

	title := "retry: fix: re-review: deep failure task"
	var task model.Task
	r := postJSON(t, srv, "/tasks", map[string]any{"title": title, "assigned_to": "coder"})
	json.NewDecoder(r.Body).Decode(&task) //nolint:errcheck
	r.Body.Close()

	claimR := postJSON(t, srv, "/tasks/"+task.ID+"/claim",
		map[string]any{"version": task.Version, "agent": "coder"})
	var claimed model.Task
	json.NewDecoder(claimR.Body).Decode(&claimed) //nolint:errcheck
	claimR.Body.Close()

	ipTask := patchTaskTo(t, srv, task.ID, "in_progress", claimed.Version)

	body, _ := json.Marshal(map[string]any{
		"status":         "failed",
		"failure_reason": "real production failure",
		"version":        ipTask.Version,
	})
	req, _ := http.NewRequest(http.MethodPatch, srv.URL+"/tasks/"+task.ID, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := http.DefaultClient.Do(req)
	resp.Body.Close()

	time.Sleep(300 * time.Millisecond)

	getResp := getJSON(t, srv, "/tasks/"+task.ID)
	var got model.Task
	json.NewDecoder(getResp.Body).Decode(&got) //nolint:errcheck
	getResp.Body.Close()

	if got.Status != model.StatusCancelled {
		t.Errorf("expected cancelled for depth>=3 task, got %s", got.Status)
	}

	mu.Lock()
	snap := make([]string, len(ceoMessages))
	copy(snap, ceoMessages)
	mu.Unlock()

	found := false
	for _, msg := range snap {
		if strings.Contains(msg, "deep failure task") || strings.Contains(msg, task.ID) {
			found = true
		}
	}
	if !found {
		t.Errorf("CEO should have been notified about depth-cap cancellation, got %d messages: %v", len(snap), snap)
	}
}

// -------------------------------------------------------------------
// ACC[4]: alert payload traceability
// (commit b7d0249)
// -------------------------------------------------------------------

func TestNoiseReduction_AlertPayload_TraceFields(t *testing.T) {
	var mu sync.Mutex
	var ceoMessages []string

	mockOC := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body) //nolint:errcheck
		if args, ok := body["args"].(map[string]any); ok {
			if msg, ok := args["message"].(string); ok {
				mu.Lock()
				ceoMessages = append(ceoMessages, msg)
				mu.Unlock()
			}
		}
		json.NewEncoder(w).Encode(map[string]any{"ok": true}) //nolint:errcheck
	}))
	defer mockOC.Close()

	oc := openclaw.NewWithURL(mockOC.URL, "")
	srv, h := newTestServerWithHandler(t, oc)
	defer srv.Close()
	h.StartRetryQueue()
	defer h.StopRetryQueue()

	var task model.Task
	r := postJSON(t, srv, "/tasks", map[string]any{"title": "trace test task", "assigned_to": "coder"})
	json.NewDecoder(r.Body).Decode(&task) //nolint:errcheck
	r.Body.Close()

	claimR := postJSON(t, srv, "/tasks/"+task.ID+"/claim",
		map[string]any{"version": task.Version, "agent": "coder"})
	var claimed model.Task
	json.NewDecoder(claimR.Body).Decode(&claimed) //nolint:errcheck
	claimR.Body.Close()

	patchTaskTo(t, srv, task.ID, "in_progress", claimed.Version)

	old := time.Now().UTC().Add(-2 * time.Minute)
	h.DB().Exec("UPDATE tasks SET started_at=?, updated_at=? WHERE id=?",
		old, old, task.ID) //nolint:errcheck

	h.SetAgentTimeoutMinutesForTesting(1)

	mu.Lock()
	ceoMessages = nil
	mu.Unlock()

	h.CheckAgentTimeouts()
	time.Sleep(300 * time.Millisecond)

	getResp := getJSON(t, srv, "/tasks/"+task.ID)
	var got model.Task
	json.NewDecoder(getResp.Body).Decode(&got) //nolint:errcheck
	getResp.Body.Close()

	mu.Lock()
	snap := make([]string, len(ceoMessages))
	copy(snap, ceoMessages)
	mu.Unlock()

	combined := got.FailureReason
	for _, m := range snap {
		combined += "\n" + m
	}

	checks := map[string]string{
		"original_task_id": "original_task_id:",
		"matched_rule":     "matched_rule:",
		"recent_history":   "recent_history:",
	}
	for field, needle := range checks {
		if !strings.Contains(combined, needle) {
			t.Errorf("trace field %s missing.\nfailure_reason: %s\nceo msgs: %v",
				field, got.FailureReason, snap)
		}
	}
}
