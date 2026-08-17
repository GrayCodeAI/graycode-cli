package eventlog

import (
	"fmt"
	"sync"
	"time"
)

// Listener observes durable events in append order. It must not block: Log
// invokes it synchronously under no lock, and a slow listener delays the
// writer, so a listener that does blocking work should fan its own work out.
type Listener func(Event)

// Log is the append-only session event spine. It owns sequence assignment and
// keeps an index by Type so consumers do not scan on the hot path.
type Log struct {
	mu       sync.RWMutex
	events   []Event
	byType   map[Type][]Event
	seq      uint64
	listener Listener
}

// New creates an empty log with opt as its observer.
func New(opt Listener) *Log {
	return &Log{byType: make(map[Type][]Event), listener: opt}
}

// Append assigns the next sequence number, stores the event, and notifies the
// observer. It panics on an unknown Type, matching the "fail loud" stance: a
// record the build cannot project must never be written silently.
func (l *Log) Append(typ Type, data any) {
	if !typ.Known() {
		panic(fmt.Sprintf("eventlog: unknown event type %q", typ))
	}
	l.mu.Lock()
	l.seq++
	ev := Event{Type: typ, Seq: l.seq, At: time.Now(), Data: data}
	l.events = append(l.events, ev)
	l.byType[typ] = append(l.byType[typ], ev)
	l.mu.Unlock()
	if l.listener != nil {
		l.listener(ev)
	}
}

// Writer returns the durable events in order. The returned slice is a snapshot;
// appends after the call do not affect it.
func (l *Log) Snapshot() []Event {
	l.mu.RLock()
	defer l.mu.RUnlock()
	out := make([]Event, len(l.events))
	copy(out, l.events)
	return out
}

// OfType returns the events of typ in append order.
func (l *Log) OfType(typ Type) []Event {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return append([]Event(nil), l.byType[typ]...)
}

// Len returns the number of durable events.
func (l *Log) Len() int {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return len(l.events)
}
