package engine

import "sync"

// EventType identifies the kind of event.
type EventType string

const (
	EventFileChanged   EventType = "file.changed"
	EventToolStarted   EventType = "tool.started"
	EventToolCompleted EventType = "tool.completed"
	EventToolFailed    EventType = "tool.failed"
	EventSessionStart  EventType = "session.start"
	EventSessionEnd    EventType = "session.end"
	EventStreamChunk   EventType = "stream.chunk"
	EventStreamDone    EventType = "stream.done"
	EventPermission    EventType = "permission.ask"
	EventError         EventType = "error"
)

// Event is a typed event in the system.
type Event struct {
	Type    EventType
	Payload interface{}
}

// EventBus is a lightweight pub/sub system for decoupling hawk components.
type EventBus struct {
	mu   sync.RWMutex
	subs map[EventType][]chan Event
}

// NewEventBus creates a new event bus.
func NewEventBus() *EventBus {
	return &EventBus{subs: make(map[EventType][]chan Event)}
}

// Subscribe returns a channel that receives events of the given type.
func (eb *EventBus) Subscribe(eventType EventType) chan Event {
	eb.mu.Lock()
	defer eb.mu.Unlock()
	ch := make(chan Event, 32)
	eb.subs[eventType] = append(eb.subs[eventType], ch)
	return ch
}

// Publish sends an event to all subscribers of that type.
func (eb *EventBus) Publish(event Event) {
	eb.mu.RLock()
	defer eb.mu.RUnlock()
	for _, ch := range eb.subs[event.Type] {
		select {
		case ch <- event:
		default: // drop if subscriber is slow
		}
	}
}

// Unsubscribe removes a channel from receiving events.
func (eb *EventBus) Unsubscribe(eventType EventType, ch chan Event) {
	eb.mu.Lock()
	defer eb.mu.Unlock()
	subs := eb.subs[eventType]
	for i, sub := range subs {
		if sub == ch {
			eb.subs[eventType] = append(subs[:i], subs[i+1:]...)
			close(ch)
			return
		}
	}
}
