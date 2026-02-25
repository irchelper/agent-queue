// Package notify – RetryQueue provides in-memory retry for CEO-critical notifications.
package notify

import (
	"log"
	"sync"
	"time"
)

// retryBackoff defines the wait durations for each attempt (0-indexed).
// attempt 0 is the initial call (not a retry), attempts 1-3 are retries.
var retryBackoff = []time.Duration{
	30 * time.Second,
	60 * time.Second,
	120 * time.Second,
}

const maxRetryAttempts = 3 // after 3 retries, give up

// retryItem holds a single pending retry.
type retryItem struct {
	sendFunc  func() error  // closure captures sessionKey + message
	attempts  int           // number of retries already done (0 = none yet)
	nextRetry time.Time
	label     string // for logs: "OnFailed:task_id" / "OnChainComplete:chain_id"
}

// RetryQueue is an in-memory retry queue for CEO-critical notifications.
// It is safe for concurrent use. Only OnFailed and OnChainComplete notifications
// should be enqueued; dispatch nudges to experts are intentionally NOT retried
// (the StaleTicker provides that safety net).
type RetryQueue struct {
	mu    sync.Mutex
	items []*retryItem
	stop  chan struct{}
	wg    sync.WaitGroup
}

// NewRetryQueue creates a new RetryQueue. Call Start() to begin processing.
func NewRetryQueue() *RetryQueue {
	return &RetryQueue{stop: make(chan struct{})}
}

// Enqueue immediately calls sendFunc. If it fails, schedules up to maxRetryAttempts
// retries with exponential backoff. label is used only for log messages.
func (q *RetryQueue) Enqueue(label string, sendFunc func() error) {
	if err := sendFunc(); err == nil {
		return // success on first try
	}
	log.Printf("[retry_queue] %s: initial send failed, queuing retry (attempt 1/%d)", label, maxRetryAttempts)
	item := &retryItem{
		sendFunc:  sendFunc,
		attempts:  1,
		nextRetry: time.Now().Add(retryBackoff[0]),
		label:     label,
	}
	q.mu.Lock()
	q.items = append(q.items, item)
	q.mu.Unlock()
}

// Start launches the background retry goroutine. Must be called once.
func (q *RetryQueue) Start() {
	q.wg.Add(1)
	go q.run()
}

// Stop signals the goroutine to stop and waits for it to exit.
func (q *RetryQueue) Stop() {
	close(q.stop)
	q.wg.Wait()
}

// run is the background goroutine that checks for due retries every 10 seconds.
func (q *RetryQueue) run() {
	defer q.wg.Done()
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-q.stop:
			return
		case now := <-ticker.C:
			q.processDue(now)
		}
	}
}

// processDue iterates pending items and retries those whose nextRetry has passed.
func (q *RetryQueue) processDue(now time.Time) {
	q.mu.Lock()
	items := make([]*retryItem, len(q.items))
	copy(items, q.items)
	q.mu.Unlock()

	var remaining []*retryItem
	for _, item := range items {
		if now.Before(item.nextRetry) {
			remaining = append(remaining, item)
			continue
		}
		// Due for retry.
		err := item.sendFunc()
		if err == nil {
			log.Printf("[retry_queue] %s: retry %d succeeded", item.label, item.attempts)
			// Success: drop from queue (don't add to remaining).
			continue
		}
		item.attempts++
		if item.attempts > maxRetryAttempts {
			log.Printf("[retry_queue] %s: gave up after %d retries", item.label, maxRetryAttempts)
			// Give up: drop from queue.
			continue
		}
		// Schedule next retry.
		backoffIdx := item.attempts - 1
		if backoffIdx >= len(retryBackoff) {
			backoffIdx = len(retryBackoff) - 1
		}
		item.nextRetry = now.Add(retryBackoff[backoffIdx])
		log.Printf("[retry_queue] %s: retry %d failed, next in %v", item.label, item.attempts-1, retryBackoff[backoffIdx])
		remaining = append(remaining, item)
	}

	q.mu.Lock()
	q.items = remaining
	q.mu.Unlock()
}

// Len returns the current queue length (for testing/monitoring).
func (q *RetryQueue) Len() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.items)
}
