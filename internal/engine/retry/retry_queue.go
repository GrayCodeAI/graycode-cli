package retry

import (
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"math"
	mrand "math/rand/v2"
	"math/big"
	"sort"
	"strings"
	"sync"
	"time"
)

// RetryItem represents a single operation queued for retry.
type RetryItem struct {
	ID          string
	Operation   string
	Args        map[string]interface{}
	Error       string
	Attempts    int
	MaxAttempts int
	NextRetry   time.Time
	Priority    int
	CreatedAt   time.Time
	Status      string // "pending", "retrying", "succeeded", "failed_permanent"
}

// RetryQueue manages failed operations with exponential backoff, priority ordering,
// and deduplication of identical operations.
type RetryQueue struct {
	Items       []*RetryItem
	MaxSize     int
	BackoffBase time.Duration
	BackoffMax  time.Duration
	mu          sync.RWMutex
}

// NewRetryQueue creates a RetryQueue with sensible defaults.
func NewRetryQueue() *RetryQueue {
	return &RetryQueue{
		Items:       make([]*RetryItem, 0),
		MaxSize:     100,
		BackoffBase: 1 * time.Second,
		BackoffMax:  5 * time.Minute,
	}
}

// Enqueue adds an operation to the retry queue. If an identical operation+args
// combination is already queued, it increments the attempt count instead of
// creating a duplicate entry.
func (rq *RetryQueue) Enqueue(operation string, args map[string]interface{}, err string, priority int) *RetryItem {
	rq.mu.Lock()
	defer rq.mu.Unlock()

	// Deduplicate: check if same operation+args already queued
	key := rq.deduplicationKey(operation, args)
	for _, item := range rq.Items {
		if item.Status == "pending" || item.Status == "retrying" {
			if rq.deduplicationKey(item.Operation, item.Args) == key {
				item.Attempts++
				item.Error = err
				item.NextRetry = time.Now().Add(rq.CalculateBackoff(item.Attempts))
				return item
			}
		}
	}

	// Enforce max size
	if len(rq.Items) >= rq.MaxSize {
		return nil
	}

	item := &RetryItem{
		ID:          rq.generateID(operation, args),
		Operation:   operation,
		Args:        args,
		Error:       err,
		Attempts:    1,
		MaxAttempts: 5,
		NextRetry:   time.Now().Add(rq.CalculateBackoff(1)),
		Priority:    priority,
		CreatedAt:   time.Now(),
		Status:      "pending",
	}

	rq.Items = append(rq.Items, item)
	return item
}

// Dequeue returns the highest-priority item whose NextRetry time has passed.
// Returns nil if no items are ready.
func (rq *RetryQueue) Dequeue() *RetryItem {
	rq.mu.Lock()
	defer rq.mu.Unlock()

	now := time.Now()
	var best *RetryItem

	for _, item := range rq.Items {
		if item.Status != "pending" && item.Status != "retrying" {
			continue
		}
		if item.NextRetry.After(now) {
			continue
		}
		if best == nil || item.Priority < best.Priority {
			best = item
		}
	}

	if best != nil {
		best.Status = "retrying"
	}
	return best
}

// MarkSuccess marks an item as successfully completed.
func (rq *RetryQueue) MarkSuccess(id string) {
	rq.mu.Lock()
	defer rq.mu.Unlock()

	for _, item := range rq.Items {
		if item.ID == id {
			item.Status = "succeeded"
			return
		}
	}
}

// MarkFailed records a failure for an item. If the item has reached its max
// attempts, it is marked as permanently failed. Otherwise, the next retry
// time is recalculated with exponential backoff.
func (rq *RetryQueue) MarkFailed(id string, err string) {
	rq.mu.Lock()
	defer rq.mu.Unlock()

	for _, item := range rq.Items {
		if item.ID == id {
			item.Attempts++
			item.Error = err
			if item.Attempts >= item.MaxAttempts {
				item.Status = "failed_permanent"
			} else {
				item.Status = "pending"
				item.NextRetry = time.Now().Add(rq.CalculateBackoff(item.Attempts))
			}
			return
		}
	}
}

// GetReady returns all items that are ready to be retried now.
func (rq *RetryQueue) GetReady() []*RetryItem {
	rq.mu.RLock()
	defer rq.mu.RUnlock()

	now := time.Now()
	var ready []*RetryItem

	for _, item := range rq.Items {
		if (item.Status == "pending" || item.Status == "retrying") && !item.NextRetry.After(now) {
			ready = append(ready, item)
		}
	}

	// Sort by priority (lower number = higher priority)
	sort.Slice(ready, func(i, j int) bool {
		return ready[i].Priority < ready[j].Priority
	})

	return ready
}

// GetPending returns all items that are still pending or retrying.
func (rq *RetryQueue) GetPending() []*RetryItem {
	rq.mu.RLock()
	defer rq.mu.RUnlock()

	var pending []*RetryItem
	for _, item := range rq.Items {
		if item.Status == "pending" || item.Status == "retrying" {
			pending = append(pending, item)
		}
	}

	sort.Slice(pending, func(i, j int) bool {
		return pending[i].Priority < pending[j].Priority
	})

	return pending
}

// CalculateBackoff computes exponential backoff with jitter for the given
// attempt count. The result is capped at BackoffMax.
func (rq *RetryQueue) CalculateBackoff(attempts int) time.Duration {
	// base * 2^attempts
	backoff := float64(rq.BackoffBase) * math.Pow(2, float64(attempts))

	// Cap at max
	if backoff > float64(rq.BackoffMax) {
		backoff = float64(rq.BackoffMax)
	}

	// Add jitter: up to 25% of the backoff duration
	jitter := float64(time.Duration(cryptoRandInt64(int64(backoff / 4))))
	result := time.Duration(backoff + jitter)

	return result
}

// cryptoRandInt64 returns a cryptographically random int64 in [0, n).
func cryptoRandInt64(n int64) int64 {
	if n <= 0 {
		return 0
	}
	bigN := big.NewInt(n)
	result, err := rand.Int(rand.Reader, bigN)
	if err != nil {
		return 0
	}
	return result.Int64()
}

// Prune removes succeeded and permanently failed items that are older than 1 hour.
func (rq *RetryQueue) Prune() {
	rq.mu.Lock()
	defer rq.mu.Unlock()

	cutoff := time.Now().Add(-1 * time.Hour)
	var kept []*RetryItem

	for _, item := range rq.Items {
		if (item.Status == "succeeded" || item.Status == "failed_permanent") && item.CreatedAt.Before(cutoff) {
			continue
		}
		kept = append(kept, item)
	}

	rq.Items = kept
}

// FormatQueue returns a human-readable representation of the retry queue.
func (rq *RetryQueue) FormatQueue() string {
	rq.mu.RLock()
	defer rq.mu.RUnlock()

	pending := 0
	for _, item := range rq.Items {
		if item.Status == "pending" || item.Status == "retrying" {
			pending++
		}
	}

	if pending == 0 {
		return "Retry Queue (empty)"
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Retry Queue (%d pending):\n", pending))
	sb.WriteString("─────────────────────────\n")

	// Collect displayable items sorted by priority
	var display []*RetryItem
	for _, item := range rq.Items {
		if item.Status == "pending" || item.Status == "retrying" || item.Status == "failed_permanent" {
			display = append(display, item)
		}
	}

	sort.Slice(display, func(i, j int) bool {
		return display[i].Priority < display[j].Priority
	})

	for i, item := range display {
		opDesc := rq.formatOperation(item)

		if item.Status == "failed_permanent" {
			sb.WriteString(fmt.Sprintf("%d. [P%d] %s (%d/%d attempts, PERMANENT FAILURE)\n",
				i+1, item.Priority, opDesc, item.Attempts, item.MaxAttempts))
		} else {
			retryIn := time.Until(item.NextRetry)
			if retryIn < 0 {
				retryIn = 0
			}
			sb.WriteString(fmt.Sprintf("%d. [P%d] %s (%d/%d attempts, retry in %s)\n",
				i+1, item.Priority, opDesc, item.Attempts, item.MaxAttempts,
				rq.formatDuration(retryIn)))
		}
		sb.WriteString(fmt.Sprintf("   Error: %q\n", item.Error))
	}

	return sb.String()
}

// Size returns the total number of items in the queue.
func (rq *RetryQueue) Size() int {
	rq.mu.RLock()
	defer rq.mu.RUnlock()
	return len(rq.Items)
}

// Clear removes all items from the queue.
func (rq *RetryQueue) Clear() {
	rq.mu.Lock()
	defer rq.mu.Unlock()
	rq.Items = make([]*RetryItem, 0)
}

// deduplicationKey generates a consistent key for operation+args combinations.
func (rq *RetryQueue) deduplicationKey(operation string, args map[string]interface{}) string {
	h := sha256.New()
	h.Write([]byte(operation))

	// Sort args keys for consistency
	keys := make([]string, 0, len(args))
	for k := range args {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, k := range keys {
		h.Write([]byte(k))
		h.Write([]byte(fmt.Sprintf("%v", args[k])))
	}

	return fmt.Sprintf("%x", h.Sum(nil))[:16]
}

// generateID creates a unique ID for a retry item.
func (rq *RetryQueue) generateID(operation string, args map[string]interface{}) string {
	h := sha256.New()
	h.Write([]byte(operation))
	h.Write([]byte(fmt.Sprintf("%v", args)))
	h.Write([]byte(time.Now().String()))
	h.Write([]byte(fmt.Sprintf("%d", mrand.Int64())))
	return fmt.Sprintf("retry_%x", h.Sum(nil))[:16]
}

// formatOperation creates a human-readable description of the operation.
func (rq *RetryQueue) formatOperation(item *RetryItem) string {
	switch item.Operation {
	case "Edit":
		if file, ok := item.Args["file"]; ok {
			return fmt.Sprintf("Edit %v", file)
		}
		return "Edit"
	case "Bash":
		if cmd, ok := item.Args["command"]; ok {
			cmdStr := fmt.Sprintf("%v", cmd)
			if len(cmdStr) > 20 {
				cmdStr = cmdStr[:20] + "..."
			}
			return fmt.Sprintf("Bash %q", cmdStr)
		}
		return "Bash"
	default:
		return item.Operation
	}
}

// formatDuration renders a duration in a compact human-readable form.
func (rq *RetryQueue) formatDuration(d time.Duration) string {
	if d < time.Second {
		return "0s"
	}
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm%ds", int(d.Minutes()), int(d.Seconds())%60)
	}
	return fmt.Sprintf("%dh%dm", int(d.Hours()), int(d.Minutes())%60)
}
