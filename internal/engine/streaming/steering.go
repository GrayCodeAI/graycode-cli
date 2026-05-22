package streaming

import "sync"

// SteeringQueue allows external callers (TUI, daemon) to inject messages
// into the agent loop between tool execution batches. This enables users
// to guide the agent without interrupting ongoing work.
type SteeringQueue struct {
	mu       sync.Mutex
	messages []SteeringMessage
	notify   chan struct{} // signals that a message was enqueued
}

// SteeringMessage represents a user guidance message injected into the agent loop.
type SteeringMessage struct {
	Content  string // user guidance text
	Priority int    // 0 = normal, 1 = high (inject immediately)
}

// NewSteeringQueue creates a new SteeringQueue with an initialized notify channel.
func NewSteeringQueue() *SteeringQueue {
	return &SteeringQueue{
		notify: make(chan struct{}, 1),
	}
}

// Enqueue adds a steering message. Thread-safe — called from TUI goroutine.
func (sq *SteeringQueue) Enqueue(msg SteeringMessage) {
	sq.mu.Lock()
	defer sq.mu.Unlock()

	if msg.Priority == 1 {
		// High-priority messages go to the front of the queue.
		sq.messages = append([]SteeringMessage{msg}, sq.messages...)
	} else {
		sq.messages = append(sq.messages, msg)
	}

	// Non-blocking signal on the notify channel.
	select {
	case sq.notify <- struct{}{}:
	default:
	}
}

// Drain returns all pending steering messages and clears the queue.
// Called from agent loop goroutine between tool batches.
func (sq *SteeringQueue) Drain() []SteeringMessage {
	sq.mu.Lock()
	defer sq.mu.Unlock()

	if len(sq.messages) == 0 {
		return nil
	}

	msgs := sq.messages
	sq.messages = nil
	return msgs
}

// HasPending returns true if there are queued messages.
func (sq *SteeringQueue) HasPending() bool {
	sq.mu.Lock()
	defer sq.mu.Unlock()
	return len(sq.messages) > 0
}

// Clear discards all pending messages.
func (sq *SteeringQueue) Clear() {
	sq.mu.Lock()
	defer sq.mu.Unlock()
	sq.messages = nil
}

// Notify returns the channel that is signaled when a message is enqueued.
// Consumers can select on this to wake up when steering input arrives.
func (sq *SteeringQueue) Notify() <-chan struct{} {
	return sq.notify
}
