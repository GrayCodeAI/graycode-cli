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

	// waterMu guards waterfalls, the synchronous value-returning chain that is
	// separate from the pub/sub subs above.
	waterMu    sync.RWMutex
	waterfalls map[EventType]*waterfallChain
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

// WaterfallHandler joins the synchronous, value-returning event chain. Returning
// without calling next short-circuits the remaining handlers. The Event value is the
// accumulated state, shared across the chain so later handlers see earlier edits.
// Returning (Event, nil) stops with that Event as the outcome; returning
// (ev, err) aborts with ev already observed.
type WaterfallHandler func(ev Event, next func(Event) (Event, error)) (Event, error)

// waterfallNode is one link in the ordered handler chain keyed by EventType.
type waterfallNode struct {
	fn   WaterfallHandler
	rest *waterfallNode
}

type waterfallChain struct {
	head *waterfallNode
	tail *waterfallNode
	len  int
}

func (c *waterfallChain) append(fn WaterfallHandler) func() {
	if fn == nil {
		return func() {}
	}
	node := &waterfallNode{fn: fn}
	if c.tail == nil {
		c.head = node
	} else {
		c.tail.rest = node
	}
	c.tail = node
	c.len++
	return func() { c.remove(node) }
}

func (c *waterfallChain) remove(node *waterfallNode) {
	if c.head == nil {
		return
	}
	if c.head == node {
		c.head = node.rest
		if c.head == nil {
			c.tail = nil
		}
		c.len--
		return
	}
	cur := c.head
	for cur != nil {
		if cur.rest == node {
			cur.rest = node.rest
			if node == c.tail {
				c.tail = cur
			}
			c.len--
			return
		}
		cur = cur.rest
	}
}

func (c *waterfallChain) run(ev Event) (Event, error) {
	if c == nil || c.head == nil {
		return ev, nil
	}
	return c.head.run(ev)
}

func (n *waterfallNode) run(ev Event) (Event, error) {
	return n.fn(ev, func(ev Event) (Event, error) {
		if n.rest == nil {
			return ev, nil
		}
		return n.rest.run(ev)
	})
}

// Waterfall registers a synchronous, ordered, value-returning handler for an event
// type and returns a disposer that removes exactly it. Handlers run in registration
// order, independently of the channel subscribers (Subscribe/Publish paths are
// unchanged). The first handler to return without invoking next short-circuits the
// chain — the deepseek-harness waterfall semantic, absent from the pub/sub path.
func (eb *EventBus) Waterfall(eventType EventType, fn WaterfallHandler) func() {
	eb.waterMu.Lock()
	defer eb.waterMu.Unlock()
	if eb.waterfalls == nil {
		eb.waterfalls = make(map[EventType]*waterfallChain)
	}
	chain := eb.waterfalls[eventType]
	if chain == nil {
		chain = &waterfallChain{}
		eb.waterfalls[eventType] = chain
	}
	return chain.append(fn)
}

// RunWaterfall executes the registered waterfall for eventType, returning the final
// Event (accumulated or short-circuited) and any error. It returns the input Event
// unchanged when no handlers are registered.
func (eb *EventBus) RunWaterfall(eventType EventType, ev Event) (Event, error) {
	if eb == nil {
		return ev, nil
	}
	eb.waterMu.RLock()
	chain := eb.waterfalls[eventType]
	eb.waterMu.RUnlock()
	if chain == nil {
		return ev, nil
	}
	return chain.run(ev)
}
