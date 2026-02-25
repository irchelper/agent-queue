// Package notify – SessionNotifier sends CEO alerts via OpenClaw sessions_send.
package notify

import (
	"fmt"
	"log"

	"github.com/irchelper/agent-queue/internal/model"
	"github.com/irchelper/agent-queue/internal/openclaw"
)

// CEOSessionKey is the default CEO session for alerts.
const CEOSessionKey = "agent:ceo:discord:channel:1475338424293789877"

// SessionNotifier notifies the CEO session when a task fails without a
// retry_assigned_to directive (i.e., human intervention is needed).
type SessionNotifier struct {
	client        *openclaw.Client
	ceoSessionKey string
	retryQ        *RetryQueue // CEO-critical notifications walk this queue
}

// NewSessionNotifier creates a SessionNotifier targeting the given CEO session.
// If ceoSessionKey is empty, CEOSessionKey is used.
// Call Start() / Stop() on the embedded RetryQueue via the returned notifier.
func NewSessionNotifier(client *openclaw.Client, ceoSessionKey string) *SessionNotifier {
	if ceoSessionKey == "" {
		ceoSessionKey = CEOSessionKey
	}
	return &SessionNotifier{
		client:        client,
		ceoSessionKey: ceoSessionKey,
		retryQ:        NewRetryQueue(),
	}
}

// Start launches the background retry goroutine. Must be called once at server startup.
func (s *SessionNotifier) Start() { s.retryQ.Start() }

// Stop gracefully shuts down the retry goroutine. Call during server shutdown.
func (s *SessionNotifier) Stop() { s.retryQ.Stop() }

// RetryQueueLen returns the number of pending retries (for testing/monitoring).
func (s *SessionNotifier) RetryQueueLen() int { return s.retryQ.Len() }

// OnFailed sends a CEO alert for a failed task that has no retry directive.
// Message format matches docs/ARCH.md §5 spec.
// Uses RetryQueue: up to 3 retries with 30s/60s/120s backoff.
func (s *SessionNotifier) OnFailed(task model.Task) error {
	reason := task.FailureReason
	if reason == "" {
		reason = task.Result
	}
	if reason == "" {
		reason = "（无）"
	}
	msg := fmt.Sprintf(
		"[agent-queue] ❌ 任务失败需介入：%s\nresult: %s\ntask_id: %s",
		task.Title, reason, task.ID,
	)
	label := "OnFailed:" + task.ID
	sendFn := func() error {
		if err := s.client.SendToSession(s.ceoSessionKey, msg); err != nil {
			log.Printf("[session_notifier] OnFailed → %s failed: %v", s.ceoSessionKey, err)
			return err
		}
		log.Printf("[session_notifier] OnFailed → %s: notified CEO", s.ceoSessionKey)
		return nil
	}
	s.retryQ.Enqueue(label, sendFn)
	return nil
}

// AsyncOnFailed runs OnFailed in a goroutine so the handler is never blocked.
func AsyncOnFailed(sn *SessionNotifier, task model.Task) {
	go func() {
		sn.OnFailed(task) //nolint:errcheck
	}()
}

// Dispatch sends a "有新任务请 poll" nudge to the session for the given agent.
// It resolves the sessionKey via openclaw.SessionKey.
// Returns ("", false) when the agent is unknown.
func (s *SessionNotifier) Dispatch(assignedTo string) (string, bool) {
	sessionKey, known := openclaw.SessionKey(assignedTo)
	if !known {
		log.Printf("[session_notifier] Dispatch: unknown agent %q – skipping", assignedTo)
		return "", false
	}
	msg := "[agent-queue] 你有新的待处理任务。请执行 poll 流程认领。"
	if err := s.client.SendToSession(sessionKey, msg); err != nil {
		log.Printf("[session_notifier] Dispatch → %s failed: %v", sessionKey, err)
		return sessionKey, false
	}
	log.Printf("[session_notifier] Dispatch → %s: notified", sessionKey)
	return sessionKey, true
}

// OnChainComplete sends a CEO notification when all tasks in a chain are done.
// Message format matches docs/ARCH.md §5 chain complete spec.
// Uses RetryQueue: up to 3 retries with 30s/60s/120s backoff.
func (s *SessionNotifier) OnChainComplete(chainID, chainTitle string, tasks []model.Task) error {
	if chainTitle == "" {
		chainTitle = "链路 " + chainID
	}
	total := len(tasks)
	done := 0
	for _, t := range tasks {
		if t.Status == model.StatusDone || t.Status == model.StatusCancelled {
			done++
		}
	}

	lines := fmt.Sprintf("[agent-queue] ✅ 任务链完成：%s\n完成任务数：%d/%d\n链路任务：",
		chainTitle, done, total)
	for _, t := range tasks {
		result := t.Result
		if result == "" {
			result = "（无）"
		}
		lines += fmt.Sprintf("\n  ✅ %s (%s) — %s", t.Title, t.AssignedTo, result)
	}
	lines += fmt.Sprintf("\nchain_id: %s", chainID)

	label := "OnChainComplete:" + chainID
	msg := lines
	sendFn := func() error {
		if err := s.client.SendToSession(s.ceoSessionKey, msg); err != nil {
			log.Printf("[session_notifier] OnChainComplete → %s failed: %v", s.ceoSessionKey, err)
			return err
		}
		log.Printf("[session_notifier] OnChainComplete chain=%s: notified CEO", chainID)
		return nil
	}
	s.retryQ.Enqueue(label, sendFn)
	return nil
}
