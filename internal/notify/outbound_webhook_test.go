package notify_test

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/irchelper/agent-queue/internal/model"
	"github.com/irchelper/agent-queue/internal/notify"
)

// safeCapture captures HTTP requests received by a test server using a mutex.
type safeCapture struct {
	mu   sync.Mutex
	body []byte
	sig  string
}

func (c *safeCapture) handler(w http.ResponseWriter, r *http.Request) {
	b, _ := io.ReadAll(r.Body)
	c.mu.Lock()
	c.body = b
	c.sig = r.Header.Get("X-Signature")
	c.mu.Unlock()
	w.WriteHeader(http.StatusOK)
}

func (c *safeCapture) read() ([]byte, string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.body, c.sig
}

func TestOutboundWebhookNotifier_Done(t *testing.T) {
	cap := &safeCapture{}
	srv := httptest.NewServer(http.HandlerFunc(cap.handler))
	defer srv.Close()

	secret := "test-secret-123"
	n := notify.NewOutboundWebhookNotifier(srv.URL, secret)

	task := model.Task{
		ID:         "abc123",
		Title:      "Test task",
		AssignedTo: "coder",
		Status:     model.StatusDone,
		Result:     "done ok",
		ChainID:    "chain_xyz",
	}

	if err := n.Notify(task); err != nil {
		t.Fatalf("Notify: %v", err)
	}

	time.Sleep(200 * time.Millisecond)

	received, receivedSig := cap.read()
	if len(received) == 0 {
		t.Fatal("no request received by test server")
	}

	var payload notify.OutboundWebhookEvent
	if err := json.Unmarshal(received, &payload); err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	if payload.Event != "task.done" {
		t.Errorf("expected event=task.done, got %q", payload.Event)
	}
	if payload.TaskID != task.ID {
		t.Errorf("expected task_id=%s, got %s", task.ID, payload.TaskID)
	}
	if payload.Result != "done ok" {
		t.Errorf("expected result='done ok', got %q", payload.Result)
	}

	// Verify HMAC signature.
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(received)
	expected := "sha256=" + hex.EncodeToString(mac.Sum(nil))
	if receivedSig != expected {
		t.Errorf("signature mismatch: want %s, got %s", expected, receivedSig)
	}

	if !notify.SignatureValid(received, secret, receivedSig) {
		t.Error("SignatureValid returned false for valid signature")
	}
}

func TestOutboundWebhookNotifier_Failed(t *testing.T) {
	cap := &safeCapture{}
	srv := httptest.NewServer(http.HandlerFunc(cap.handler))
	defer srv.Close()

	n := notify.NewOutboundWebhookNotifier(srv.URL, "")
	task := model.Task{
		ID:     "fail-task",
		Title:  "Failing task",
		Status: model.StatusFailed,
	}
	_ = n.Notify(task)
	time.Sleep(200 * time.Millisecond)

	received, _ := cap.read()
	var payload notify.OutboundWebhookEvent
	json.Unmarshal(received, &payload)
	if payload.Event != "task.failed" {
		t.Errorf("expected task.failed, got %q", payload.Event)
	}
}

func TestOutboundWebhookNotifier_SkipsNonTerminal(t *testing.T) {
	called := false
	var mu sync.Mutex
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		called = true
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	n := notify.NewOutboundWebhookNotifier(srv.URL, "")
	task := model.Task{ID: "t1", Title: "t", Status: model.StatusInProgress}
	_ = n.Notify(task)
	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	wasCalled := called
	mu.Unlock()
	if wasCalled {
		t.Error("webhook should not be called for in_progress status")
	}
}

func TestOutboundWebhookNotifier_BestEffortOnServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	n := notify.NewOutboundWebhookNotifier(srv.URL, "")
	task := model.Task{ID: "t2", Title: "t", Status: model.StatusDone}
	err := n.Notify(task)
	if err != nil {
		t.Errorf("Notify should return nil on server error (best-effort), got: %v", err)
	}
	time.Sleep(200 * time.Millisecond)
}

func TestMultiNotifier(t *testing.T) {
	var mu sync.Mutex
	var calls []string

	srv1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		calls = append(calls, "n1")
		mu.Unlock()
		w.WriteHeader(200)
	}))
	srv2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		calls = append(calls, "n2")
		mu.Unlock()
		w.WriteHeader(200)
	}))
	defer srv1.Close()
	defer srv2.Close()

	n1 := notify.NewOutboundWebhookNotifier(srv1.URL, "")
	n2 := notify.NewOutboundWebhookNotifier(srv2.URL, "")
	multi := notify.NewMultiNotifier(n1, n2)

	task := model.Task{ID: "m1", Title: "m", Status: model.StatusDone}
	_ = multi.Notify(task)
	time.Sleep(300 * time.Millisecond)

	mu.Lock()
	n := len(calls)
	callsCopy := append([]string{}, calls...)
	mu.Unlock()

	if n != 2 {
		t.Errorf("expected 2 notifiers called, got %d: %v", n, callsCopy)
	}
	callsStr := strings.Join(callsCopy, ",")
	if !strings.Contains(callsStr, "n1") || !strings.Contains(callsStr, "n2") {
		t.Errorf("expected both n1 and n2 called, got: %v", callsCopy)
	}
}
