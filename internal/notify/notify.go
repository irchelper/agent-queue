// Package notify provides the Notifier interface and a Discord Incoming
// Webhook implementation.
package notify

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
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
	mention := ""
	if d.userID != "" {
		mention = fmt.Sprintf("<@%s> ", d.userID)
	}
	content := fmt.Sprintf("%s✅ 任务完成：%s (task_id: %s)", mention, task.Title, task.ID)

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

// AsyncNotify runs n.Notify in a goroutine so the caller is never blocked.
func AsyncNotify(n Notifier, task model.Task) {
	go func() {
		if err := n.Notify(task); err != nil {
			log.Printf("[notify] async notification failed for task %s: %v", task.ID, err)
		}
	}()
}
