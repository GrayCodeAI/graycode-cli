package engine

import (
	"crypto/rand"
	"encoding/hex"
	"sort"
	"sync"
	"time"
)

// PromptPriority defines scheduling urgency for enqueued prompt turns.
type PromptPriority int

const (
	// PriorityNormal is standard user turns (default FIFO).
	PriorityNormal PromptPriority = 0
	// PrioritySteering is scheduled prompts or background notifications.
	PrioritySteering PromptPriority = 10
	// PriorityInterjection is high-priority immediate interjections (/btw).
	PriorityInterjection PromptPriority = 20
)

// EnqueuedPrompt represents a prompt turn waiting in the queue.
type EnqueuedPrompt struct {
	ID         string                 `json:"id"`
	Text       string                 `json:"text"`
	Priority   PromptPriority         `json:"priority"`
	Source     string                 `json:"source"`
	EnqueuedAt time.Time              `json:"enqueued_at"`
	Metadata   map[string]interface{} `json:"metadata,omitempty"`
}

// PromptQueue provides a thread-safe priority FIFO queue for multi-source prompts.
type PromptQueue struct {
	mu     sync.Mutex
	items  []EnqueuedPrompt
	paused bool
}

// NewPromptQueue initializes an empty PromptQueue.
func NewPromptQueue() *PromptQueue {
	return &PromptQueue{
		items: make([]EnqueuedPrompt, 0),
	}
}

// Enqueue adds a prompt to the queue, sorted by priority (descending) and enqueue time (ascending).
func (pq *PromptQueue) Enqueue(p EnqueuedPrompt) string {
	pq.mu.Lock()
	defer pq.mu.Unlock()

	if p.ID == "" {
		p.ID = generatePromptID()
	}
	if p.EnqueuedAt.IsZero() {
		p.EnqueuedAt = time.Now()
	}

	pq.items = append(pq.items, p)
	pq.sortLocked()
	return p.ID
}

// EnqueueText is a helper to enqueue text with a given priority and source.
func (pq *PromptQueue) EnqueueText(text string, priority PromptPriority, source string) string {
	return pq.Enqueue(EnqueuedPrompt{
		Text:       text,
		Priority:   priority,
		Source:     source,
		EnqueuedAt: time.Now(),
	})
}

// Dequeue extracts the highest priority, oldest prompt from the queue.
// Returns false if the queue is empty or currently paused.
func (pq *PromptQueue) Dequeue() (EnqueuedPrompt, bool) {
	pq.mu.Lock()
	defer pq.mu.Unlock()

	if pq.paused || len(pq.items) == 0 {
		return EnqueuedPrompt{}, false
	}

	item := pq.items[0]
	pq.items = pq.items[1:]
	return item, true
}

// Peek views the next item without removing it.
func (pq *PromptQueue) Peek() (EnqueuedPrompt, bool) {
	pq.mu.Lock()
	defer pq.mu.Unlock()

	if len(pq.items) == 0 {
		return EnqueuedPrompt{}, false
	}
	return pq.items[0], true
}

// Len returns the current number of enqueued items.
func (pq *PromptQueue) Len() int {
	pq.mu.Lock()
	defer pq.mu.Unlock()
	return len(pq.items)
}

// IsEmpty returns true if there are no items in the queue.
func (pq *PromptQueue) IsEmpty() bool {
	return pq.Len() == 0
}

// Clear drops all pending prompts.
func (pq *PromptQueue) Clear() {
	pq.mu.Lock()
	defer pq.mu.Unlock()
	pq.items = pq.items[:0]
}

// Drain removes and returns all currently queued items in priority order.
func (pq *PromptQueue) Drain() []EnqueuedPrompt {
	pq.mu.Lock()
	defer pq.mu.Unlock()

	res := make([]EnqueuedPrompt, len(pq.items))
	copy(res, pq.items)
	pq.items = pq.items[:0]
	return res
}

// Pause prevents Dequeue from returning items until Resume is called.
func (pq *PromptQueue) Pause() {
	pq.mu.Lock()
	defer pq.mu.Unlock()
	pq.paused = true
}

// Resume allows Dequeue to continue processing items.
func (pq *PromptQueue) Resume() {
	pq.mu.Lock()
	defer pq.mu.Unlock()
	pq.paused = false
}

// IsPaused reports whether the queue is currently paused.
func (pq *PromptQueue) IsPaused() bool {
	pq.mu.Lock()
	defer pq.mu.Unlock()
	return pq.paused
}

// Remove removes a specific prompt by its ID.
func (pq *PromptQueue) Remove(id string) bool {
	pq.mu.Lock()
	defer pq.mu.Unlock()

	for i, item := range pq.items {
		if item.ID == id {
			pq.items = append(pq.items[:i], pq.items[i+1:]...)
			return true
		}
	}
	return false
}

// List returns a copy of all queued items in current execution order.
func (pq *PromptQueue) List() []EnqueuedPrompt {
	pq.mu.Lock()
	defer pq.mu.Unlock()

	res := make([]EnqueuedPrompt, len(pq.items))
	copy(res, pq.items)
	return res
}

func (pq *PromptQueue) sortLocked() {
	sort.SliceStable(pq.items, func(i, j int) bool {
		// Higher priority first
		if pq.items[i].Priority != pq.items[j].Priority {
			return pq.items[i].Priority > pq.items[j].Priority
		}
		// Earlier enqueue time first (FIFO within same priority)
		return pq.items[i].EnqueuedAt.Before(pq.items[j].EnqueuedAt)
	})
}

func generatePromptID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return "pq-" + hex.EncodeToString(b)
}
