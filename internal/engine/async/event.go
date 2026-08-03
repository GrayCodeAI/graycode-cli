package async

import (
	"sync"
	"time"
)

// OpType identifies the kind of operation submitted to the engine.
type OpType string

const (
	OpUserTurn  OpType = "user_turn"
	OpCancel    OpType = "cancel"
	OpResume    OpType = "resume"
	OpInterrupt OpType = "interrupt"
)

// Submission represents a user request sent to the async engine.
type Submission struct {
	ID        string
	Op        OpType
	Payload   string
	Timestamp time.Time
	ReplyTo   chan<- *Event // optional: caller receives the terminal event (EventDone/EventError) directly
}

// EventType identifies the kind of event emitted by the engine.
type EventType string

const (
	EventToken      EventType = "token"
	EventToolCall   EventType = "tool_call"
	EventToolResult EventType = "tool_result"
	EventDone       EventType = "done"
	EventError      EventType = "error"
	EventThinking   EventType = "thinking"
	EventUsage      EventType = "usage"
	EventInfo       EventType = "info"
)

// Event is a single event from the async engine.
type Event struct {
	ID        string
	Type      EventType
	Content   string
	ToolName  string
	ToolID    string
	Usage     *Usage
	Timestamp time.Time
	SessionID string
	// SourceType preserves the raw engine stream event type for unmapped
	// events (e.g. "compact_start") so diagnostics stay accurate.
	SourceType string
}

// Usage tracks token usage.
type Usage struct {
	PromptTokens     int
	CompletionTokens int
	CacheReadTokens  int
	CacheWriteTokens int
	TotalTokens      int
	Provider         string
	Model            string
}

// EventQueue is a thread-safe, replayable queue of events.
type EventQueue struct {
	mu      sync.RWMutex
	events  []*Event
	subs    []chan *Event
	subMu   sync.Mutex
	maxSize int
}

// NewEventQueue creates an event queue with the given max capacity.
func NewEventQueue(maxSize int) *EventQueue {
	if maxSize <= 0 {
		maxSize = 10000
	}
	return &EventQueue{
		events:  make([]*Event, 0, maxSize),
		maxSize: maxSize,
	}
}

// Push adds an event and broadcasts it to all subscribers.
func (eq *EventQueue) Push(evt *Event) {
	eq.mu.Lock()
	eq.events = append(eq.events, evt)
	if len(eq.events) > eq.maxSize {
		eq.events = eq.events[len(eq.events)-eq.maxSize:]
	}
	eq.mu.Unlock()

	eq.subMu.Lock()
	for _, sub := range eq.subs {
		select {
		case sub <- evt:
		default:
			// Drop if subscriber is slow
		}
	}
	eq.subMu.Unlock()
}

// Subscribe returns a channel that receives new events.
func (eq *EventQueue) Subscribe() <-chan *Event {
	ch := make(chan *Event, 256)
	eq.subMu.Lock()
	eq.subs = append(eq.subs, ch)
	eq.subMu.Unlock()
	return ch
}

// Unsubscribe removes a subscriber channel. It is safe to call from the
// consumer goroutine; a channel that is not subscribed is a no-op.
func (eq *EventQueue) Unsubscribe(ch <-chan *Event) {
	eq.subMu.Lock()
	defer eq.subMu.Unlock()
	for i, sub := range eq.subs {
		if sub == ch {
			eq.subs = append(eq.subs[:i], eq.subs[i+1:]...)
			return
		}
	}
}

// Replay returns all stored events for replay.
func (eq *EventQueue) Replay() []*Event {
	eq.mu.RLock()
	defer eq.mu.RUnlock()
	out := make([]*Event, len(eq.events))
	copy(out, eq.events)
	return out
}

// SubmissionQueue is a thread-safe queue for user submissions.
type SubmissionQueue struct {
	mu          sync.Mutex
	submissions []*Submission
	notify      chan struct{}
}

// NewSubmissionQueue creates a new submission queue.
func NewSubmissionQueue() *SubmissionQueue {
	return &SubmissionQueue{
		submissions: make([]*Submission, 0, 64),
		notify:      make(chan struct{}, 1),
	}
}

// Submit adds a submission and notifies the engine.
func (sq *SubmissionQueue) Submit(s *Submission) {
	sq.mu.Lock()
	sq.submissions = append(sq.submissions, s)
	sq.mu.Unlock()

	select {
	case sq.notify <- struct{}{}:
	default:
	}
}

// pop returns the oldest pending submission without blocking. Returns ok=false
// when the queue is empty. The engine drains the queue after each notify.
func (sq *SubmissionQueue) pop() (s *Submission, ok bool) {
	sq.mu.Lock()
	defer sq.mu.Unlock()
	if len(sq.submissions) == 0 {
		return nil, false
	}
	s = sq.submissions[0]
	sq.submissions = sq.submissions[1:]
	return s, true
}

// Len returns the number of pending submissions.
func (sq *SubmissionQueue) Len() int {
	sq.mu.Lock()
	defer sq.mu.Unlock()
	return len(sq.submissions)
}
