// Package notify provides the Notifier interface and a Discord Incoming
// Webhook implementation.
package notify

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"net/http"
	"os"
	"time"

	"github.com/irchelper/agent-queue/internal/model"
)

// Notifier sends a notification when a task changes state.
type Notifier interface {
	Notify(task model.Task) error
}

// NoOp is a no-operation notifier used when no webhook URL is configured.
type NoOp struct{}

func (NoOp) Notify(task model.Task) error {
	log.Printf("[notify] no-op: task %s (%s) done – webhook not configured", task.ID, task.Title)
	return nil
}

// DiscordNotifier sends a message to a Discord Incoming Webhook.
type DiscordNotifier struct {
	webhookURL string
	userID     string
	client     *http.Client
}

// NewFromEnv returns a DiscordNotifier if AGENT_QUEUE_DISCORD_WEBHOOK_URL is
// set, otherwise a NoOp notifier.
func NewFromEnv() Notifier {
	url := os.Getenv("AGENT_QUEUE_DISCORD_WEBHOOK_URL")
	if url == "" {
		return NoOp{}
	}
	return &DiscordNotifier{
		webhookURL: url,
		userID:     os.Getenv("AGENT_QUEUE_DISCORD_USER_ID"),
		client:     &http.Client{Timeout: 10 * time.Second},
	}
}

// Notify sends a Discord webhook message.  It retries once on failure.
// A final failure is logged but never blocks the caller.
func (d *DiscordNotifier) Notify(task model.Task) error {
	err := d.send(task)
	if err != nil {
		log.Printf("[notify] first attempt failed for task %s: %v – retrying", task.ID, err)
		err = d.send(task)
		if err != nil {
			log.Printf("[notify] retry failed for task %s: %v – giving up", task.ID, err)
		}
	}
	return err
}

func (d *DiscordNotifier) send(task model.Task) error {
	content := FormatMessage(task, d.userID, time.Now())

	body, err := json.Marshal(map[string]string{"content": content})
	if err != nil {
		return fmt.Errorf("marshal body: %w", err)
	}

	resp, err := d.client.Post(d.webhookURL, "application/json", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("http post: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("webhook returned HTTP %d", resp.StatusCode)
	}
	return nil
}

// FormatMessage builds the Discord notification content for a completed task.
// userID may be empty (no @mention in that case).
// doneAt is the timestamp used for duration calculation (pass time.Now() in production).
func FormatMessage(task model.Task, userID string, doneAt time.Time) string {
	mention := ""
	if userID != "" {
		mention = fmt.Sprintf("<@%s> ", userID)
	}

	expert := task.AssignedTo
	if expert == "" {
		expert = "未知"
	}

	result := task.Result
	if result == "" {
		result = "（无）"
	}

	duration := FormatDuration(task.StartedAt, doneAt)

	return fmt.Sprintf(
		"%s✅ 任务完成\n**任务：** %s\n**专家：** %s\n**耗时：** %s\n**结果：** %s\n`task_id: %s`",
		mention, task.Title, expert, duration, result, task.ID,
	)
}

// FormatDuration computes a human-readable duration from startedAt to doneAt.
// If startedAt is nil, returns "未知".
// Duration < 1 min → "< 1 分钟"; otherwise → "约 X 分钟" (ceiling).
func FormatDuration(startedAt *time.Time, doneAt time.Time) string {
	if startedAt == nil {
		return "未知"
	}
	elapsed := doneAt.Sub(*startedAt)
	minutes := elapsed.Minutes()
	if minutes < 1 {
		return "< 1 分钟"
	}
	return fmt.Sprintf("约 %d 分钟", int(math.Ceil(minutes)))
}

// AsyncNotify runs n.Notify in a goroutine so the caller is never blocked.
func AsyncNotify(n Notifier, task model.Task) {
	go func() {
		if err := n.Notify(task); err != nil {
			log.Printf("[notify] async notification failed for task %s: %v", task.ID, err)
		}
	}()
}
