package notify

// Internal tests for unexported helpers: parseAgentWebhooks, resolveWebhookURL.

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/irchelper/agent-queue/internal/model"
)

// -------------------------------------------------------------------
// V11: agent_channel_map webhook routing tests
// -------------------------------------------------------------------

func TestParseAgentWebhooks_Empty(t *testing.T) {
	m := parseAgentWebhooks("")
	if len(m) != 0 {
		t.Fatalf("expected empty map, got %v", m)
	}
}

func TestParseAgentWebhooks_Single(t *testing.T) {
	m := parseAgentWebhooks("coder=https://hooks.discord/coder")
	if m["coder"] != "https://hooks.discord/coder" {
		t.Fatalf("unexpected map: %v", m)
	}
}

func TestParseAgentWebhooks_Multiple(t *testing.T) {
	m := parseAgentWebhooks("coder=https://c,thinker=https://t,qa=https://q")
	if len(m) != 3 {
		t.Fatalf("expected 3 entries, got %d: %v", len(m), m)
	}
	if m["thinker"] != "https://t" {
		t.Fatalf("thinker url wrong: %v", m["thinker"])
	}
}

func TestParseAgentWebhooks_MalformedSkipped(t *testing.T) {
	m := parseAgentWebhooks("coder=https://c,bad-entry,=empty-key,noequals")
	// only "coder=https://c" is valid
	if len(m) != 1 || m["coder"] != "https://c" {
		t.Fatalf("expected only coder entry, got %v", m)
	}
}

func TestDiscordNotifier_ResolveWebhookURL_Hit(t *testing.T) {
	d := &DiscordNotifier{
		webhookURL:    "https://default",
		agentWebhooks: map[string]string{"coder": "https://coder-specific"},
	}
	if got := d.resolveWebhookURL("coder"); got != "https://coder-specific" {
		t.Fatalf("expected coder-specific URL, got %s", got)
	}
}

func TestDiscordNotifier_ResolveWebhookURL_Miss_Fallback(t *testing.T) {
	d := &DiscordNotifier{
		webhookURL:    "https://default",
		agentWebhooks: map[string]string{"coder": "https://coder-specific"},
	}
	if got := d.resolveWebhookURL("thinker"); got != "https://default" {
		t.Fatalf("expected default URL for miss, got %s", got)
	}
}

func TestDiscordNotifier_ResolveWebhookURL_EmptyMap(t *testing.T) {
	d := &DiscordNotifier{
		webhookURL:    "https://default",
		agentWebhooks: map[string]string{},
	}
	if got := d.resolveWebhookURL("coder"); got != "https://default" {
		t.Fatalf("expected default URL for empty map, got %s", got)
	}
}

func TestDiscordNotifier_Notify_RoutesToAgentWebhook(t *testing.T) {
	var hitURLs []string
	var mu sync.Mutex

	agentSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		hitURLs = append(hitURLs, "agent")
		mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	}))
	defer agentSrv.Close()

	defaultSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		hitURLs = append(hitURLs, "default")
		mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	}))
	defer defaultSrv.Close()

	d := &DiscordNotifier{
		webhookURL:    defaultSrv.URL,
		agentWebhooks: map[string]string{"coder": agentSrv.URL},
		client:        &http.Client{Timeout: 5 * time.Second},
	}

	// Task assigned to coder → should hit agent server
	err := d.Notify(model.Task{ID: "t1", Title: "test", AssignedTo: "coder", Status: "done"})
	if err != nil {
		t.Fatalf("Notify error: %v", err)
	}

	// Task assigned to thinker (no entry) → should hit default server
	err = d.Notify(model.Task{ID: "t2", Title: "test2", AssignedTo: "thinker", Status: "done"})
	if err != nil {
		t.Fatalf("Notify error: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(hitURLs) != 2 || hitURLs[0] != "agent" || hitURLs[1] != "default" {
		t.Fatalf("unexpected hit pattern: %v", hitURLs)
	}
}
