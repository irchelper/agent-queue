package notify

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/irchelper/agent-queue/internal/model"
)

// OutboundWebhookEvent is the payload posted to the external webhook URL.
type OutboundWebhookEvent struct {
	Event      string `json:"event"`       // "task.done" | "task.failed" | "task.cancelled"
	TaskID     string `json:"task_id"`
	Title      string `json:"title"`
	AssignedTo string `json:"assigned_to"`
	Result     string `json:"result,omitempty"`
	ChainID    string `json:"chain_id,omitempty"`
	Timestamp  string `json:"timestamp"`
}

// OutboundWebhookNotifier posts task lifecycle events to an external URL.
// It signs each request with HMAC-SHA256 if a secret is configured.
// All operations are best-effort: failures are logged, not propagated.
type OutboundWebhookNotifier struct {
	url    string
	secret string
	client *http.Client
}

// NewOutboundWebhookNotifier creates a notifier. url must be non-empty.
func NewOutboundWebhookNotifier(webhookURL, secret string) *OutboundWebhookNotifier {
	return &OutboundWebhookNotifier{
		url:    webhookURL,
		secret: secret,
		client: &http.Client{Timeout: 5 * time.Second},
	}
}

// Notify implements notify.Notifier by posting a webhook for done/failed/cancelled tasks.
func (n *OutboundWebhookNotifier) Notify(task model.Task) error {
	event := taskEvent(task.Status)
	if event == "" {
		return nil // only notify on terminal/notable transitions
	}

	payload := OutboundWebhookEvent{
		Event:      event,
		TaskID:     task.ID,
		Title:      task.Title,
		AssignedTo: task.AssignedTo,
		Result:     task.Result,
		ChainID:    task.ChainID,
		Timestamp:  time.Now().UTC().Format(time.RFC3339),
	}
	go n.send(payload) // best-effort, non-blocking
	return nil
}

// Send posts the event to the configured webhook URL.
func (n *OutboundWebhookNotifier) send(payload OutboundWebhookEvent) {
	body, err := json.Marshal(payload)
	if err != nil {
		log.Printf("[outbound-webhook] marshal: %v", err)
		return
	}

	req, err := http.NewRequest(http.MethodPost, n.url, bytes.NewReader(body))
	if err != nil {
		log.Printf("[outbound-webhook] build request: %v", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "agent-queue/1.0")

	if n.secret != "" {
		sig := sign(body, n.secret)
		req.Header.Set("X-Signature", "sha256="+sig)
	}

	resp, err := n.client.Do(req)
	if err != nil {
		log.Printf("[outbound-webhook] POST %s: %v", n.url, err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		log.Printf("[outbound-webhook] non-2xx response %d from %s", resp.StatusCode, n.url)
	}
}

// sign computes HMAC-SHA256(body, secret) and returns the hex digest.
func sign(body []byte, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}

// taskEvent maps a task status to an event name, returning "" to skip.
func taskEvent(status model.Status) string {
	switch status {
	case model.StatusDone:
		return "task.done"
	case model.StatusFailed:
		return "task.failed"
	case model.StatusCancelled:
		return "task.cancelled"
	default:
		return ""
	}
}

// SignatureValid verifies an HMAC-SHA256 signature from an inbound webhook.
// Use this in test helpers or future inbound webhook receivers.
func SignatureValid(body []byte, secret, received string) bool {
	expected := "sha256=" + sign(body, secret)
	return hmac.Equal([]byte(expected), []byte(received))
}

// MultiNotifier fans out to multiple Notifier implementations.
type MultiNotifier struct {
	notifiers []Notifier
}

// NewMultiNotifier wraps multiple notifiers into one.
func NewMultiNotifier(ns ...Notifier) *MultiNotifier {
	return &MultiNotifier{notifiers: ns}
}

// Notify calls all child notifiers; returns the first non-nil error.
func (m *MultiNotifier) Notify(task model.Task) error {
	var firstErr error
	for _, n := range m.notifiers {
		if err := n.Notify(task); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// Verify OutboundWebhookNotifier implements Notifier.
var _ Notifier = (*OutboundWebhookNotifier)(nil)
var _ Notifier = (*MultiNotifier)(nil)

// ensure sign is used (avoid unused warning in tests).
var _ = fmt.Sprintf
