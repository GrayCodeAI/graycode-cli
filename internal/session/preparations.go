// Package session provides the Go-native port of DeepSeek Harness's
// SessionPreparations class (dsh-session-persistence/preparations.ts).
//
// SessionPreparations provides bounded sharing and exclusive reservation of
// unpublished session sources: an LRU cache of prepared/cold-read sessions
// with load-sharing, exclusive reservation for publication, and phase-based
// write admission control.
package session

import (
	"container/list"
	"sync"

	"github.com/GrayCodeAI/hawk/internal/eventlog"
)

// PreparationPhase tracks the lifecycle of one cached entry.
type PreparationPhase int

const (
	PhaseLoading PreparationPhase = iota
	PhaseReady
	PhaseCommitting
	PhaseReserved
)

// String returns a human-readable phase name.
func (p PreparationPhase) String() string {
	switch p {
	case PhaseLoading:
		return "loading"
	case PhaseReady:
		return "ready"
	case PhaseCommitting:
		return "committing"
	case PhaseReserved:
		return "reserved"
	default:
		return "unknown"
	}
}

// PreparedSource is the immutable view of a prepared session for caching.
type PreparedSource struct {
	Session *eventlog.Log
	Meta    map[string]any
	Events  []eventlog.WireEvent
}

// SessionState tracks per-session write state, ported from DSH's SessionState.
type SessionState struct {
	Meta         map[string]any
	Cursor       uint64
	Materialized bool
	Owner        *PreparedSource
}

// Reservation is one exclusively held prepared source and its committed
// persistence state. Ported from DSH's SessionPreparationReservation.
type Reservation struct {
	entry  *prepEntry
	source PreparedSource
	state  *SessionState
}

// prepEntry is one cached preparation entry in the LRU.
type prepEntry struct {
	id          string
	mu          sync.Mutex
	phase       PreparationPhase
	source      *PreparedSource
	result      chan struct{}
	loadErr     error
	reservation *Reservation
	settleCh    chan struct{}
}

// SessionPreparations is a bounded LRU cache of prepared session sources
// with exclusive reservation support. Ported from DSH's SessionPreparations
// class.
type SessionPreparations struct {
	mu       sync.Mutex
	entries  map[string]*list.Element
	lru      *list.List
	capacity int
}

// NewSessionPreparations creates a new preparation cache with the given
// bounded capacity for ready entries. Ported from DSH's SessionPreparations
// constructor.
func NewSessionPreparations(capacity int) *SessionPreparations {
	if capacity < 1 {
		capacity = 1
	}
	return &SessionPreparations{
		entries:  make(map[string]*list.Element),
		lru:      list.New(),
		capacity: capacity,
	}
}

// Has reports whether this pool currently knows about an unpublished identity.
func (p *SessionPreparations) Has(id string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	_, ok := p.entries[id]
	return ok
}

// Inspect observes one prepared source, sharing an in-flight read for the same id.
// The load function is called only if no entry exists for the id yet.
// Ported from DSH's inspect().
func (p *SessionPreparations) Inspect(id string, load func() (*PreparedSource, error)) (*PreparedSource, error) {
	entry := p.entryFor(id, load)

	// Wait for the load to complete.
	<-entry.result

	p.mu.Lock()
	if elem, ok := p.entries[id]; ok {
		_ = elem.Value.(*prepEntry)
		p.lru.MoveToBack(elem)
	}
	p.mu.Unlock()

	if entry.loadErr != nil {
		return nil, entry.loadErr
	}
	if entry.source == nil {
		return nil, errPreparationIncomplete
	}
	return entry.source, nil
}

// Reserve reserves one ready source after committing its pending durable repair.
// The commit function performs the durable repair + cursor-state commit and
// returns the committed state, or nil if the source was invalidated.
// Ported from DSH's reserve().
func (p *SessionPreparations) Reserve(
	id string,
	load func() (*PreparedSource, error),
	commit func(source PreparedSource) (*SessionState, error),
) (*Reservation, error) {
	entry := p.entryFor(id, load)

	// Wait for the load to complete.
	<-entry.result

	entry.mu.Lock()
	// Check for load errors.
	if entry.loadErr != nil {
		entry.mu.Unlock()
		return nil, entry.loadErr
	}
	// Wait for entry to become ready (transition from loading).
	for entry.phase != PhaseReady {
		entry.mu.Unlock()
		// If there's a settle channel, wait on it.
		entry.mu.Lock()
		if entry.phase == PhaseReady {
			break
		}
		if entry.settleCh == nil {
			// Should not happen — loading phase should complete before
			// reserve can reach here (result channel is closed).
			entry.mu.Unlock()
			return nil, errPreparationIncomplete
		}
		settleCh := entry.settleCh
		entry.mu.Unlock()
		<-settleCh
		entry.mu.Lock()
	}

	if entry.source == nil {
		entry.mu.Unlock()
		return nil, errPreparationIncomplete
	}

	// Transition to committing phase.
	entry.phase = PhaseCommitting
	entry.settleCh = make(chan struct{})
	entry.mu.Unlock()

	source := *entry.source
	state, err := commit(source)
	if err != nil {
		// Remove the entry on commit failure.
		p.settleAndRemove(id, entry)
		return nil, err
	}
	if state == nil {
		p.settleAndRemove(id, entry)
		return nil, nil // invalidated
	}

	// Transition to reserved phase.
	entry.mu.Lock()
	entry.source = &source
	entry.phase = PhaseReserved
	ch := entry.settleCh
	entry.settleCh = nil
	entry.mu.Unlock()
	if ch != nil {
		close(ch)
	}

	reservation := &Reservation{
		entry:  entry,
		source: source,
		state:  state,
	}
	entry.reservation = reservation

	p.mu.Lock()
	if elem, ok := p.entries[id]; ok {
		p.lru.MoveToBack(elem)
	}
	p.mu.Unlock()

	return reservation, nil
}

// ReservationFor returns the exact reservation for a given session,
// rejecting aliases. Ported from DSH's reservationFor().
func (p *SessionPreparations) ReservationFor(session *eventlog.Log) *Reservation {
	p.mu.Lock()
	defer p.mu.Unlock()

	for _, elem := range p.entries {
		entry := elem.Value.(*prepEntry)
		entry.mu.Lock()
		if entry.phase == PhaseReserved && entry.source != nil && entry.source.Session == session {
			r := entry.reservation
			entry.mu.Unlock()
			if r != nil {
				return r
			}
		}
		entry.mu.Unlock()
	}
	return nil
}

// Attach consumes a reservation after its exact Session has attached.
// Ported from DSH's attach().
func (p *SessionPreparations) Attach(reservation *Reservation) {
	p.mu.Lock()
	defer p.mu.Unlock()

	entry := reservation.entry
	if elem, ok := p.entries[entry.id]; ok && elem.Value.(*prepEntry) == entry {
		p.lru.Remove(elem)
		delete(p.entries, entry.id)
	}
}

// Discard consumes a reservation whose caller only needs the committed
// inspection (does not attach a live session). Ported from DSH's discard().
func (p *SessionPreparations) Discard(reservation *Reservation) {
	p.mu.Lock()
	defer p.mu.Unlock()

	entry := reservation.entry
	if elem, ok := p.entries[entry.id]; ok && elem.Value.(*prepEntry) == entry && entry.reservation == reservation {
		p.lru.Remove(elem)
		delete(p.entries, entry.id)
	}
}

// Release returns a reusable unpublished reservation to the ready LRU.
// If reusable is false, the entry is removed. Ported from DSH's release().
func (p *SessionPreparations) Release(reservation *Reservation, reusable bool) {
	p.mu.Lock()
	defer p.mu.Unlock()

	entry := reservation.entry
	if elem, ok := p.entries[entry.id]; ok && elem.Value.(*prepEntry) == entry {
		e := elem.Value.(*prepEntry)
		e.mu.Lock()
		if e.reservation == reservation && e.phase == PhaseReserved {
			if !reusable {
				p.lru.Remove(elem)
				delete(p.entries, entry.id)
				e.mu.Unlock()
				return
			}
			e.reservation = nil
			e.phase = PhaseReady
			e.mu.Unlock()
			p.lru.MoveToBack(elem)
			p.evictLocked()
			return
		}
		e.mu.Unlock()
	}
}

// Invalidate discards a prepared view after the durable log changes.
// Ported from DSH's invalidate().
func (p *SessionPreparations) Invalidate(id string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if elem, ok := p.entries[id]; ok {
		p.lru.Remove(elem)
		delete(p.entries, id)
	}
}

// DiscardReady discards an exact stale ready source without disturbing an
// exclusive owner. Returns whether the source was discarded, retained by a
// reservation, or is absent. Ported from DSH's discardReady().
func (p *SessionPreparations) DiscardReady(id string, expected *PreparedSource) string {
	p.mu.Lock()
	defer p.mu.Unlock()

	elem, ok := p.entries[id]
	if !ok {
		return "missing"
	}
	entry := elem.Value.(*prepEntry)
	entry.mu.Lock()
	if entry.source != expected || entry.phase != PhaseReady {
		entry.mu.Unlock()
		return "retained"
	}
	entry.mu.Unlock()
	p.lru.Remove(elem)
	delete(p.entries, id)
	return "discarded"
}

// AssertWritable rejects writes while a session is reserved or committing.
// Ported from DSH's assertWritable().
func (p *SessionPreparations) AssertWritable(id string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if elem, ok := p.entries[id]; ok {
		entry := elem.Value.(*prepEntry)
		entry.mu.Lock()
		phase := entry.phase
		entry.mu.Unlock()
		if phase == PhaseCommitting || phase == PhaseReserved {
			panic("cannot append session \"" + id + "\": its persisted preparation is reserved")
		}
	}
}

// TakeReady removes a completed entry for an already-serialized append adoption.
// Returns the prepared source, or nil when no ready entry exists.
// Ported from DSH's takeReady().
func (p *SessionPreparations) TakeReady(id string) *PreparedSource {
	p.mu.Lock()
	defer p.mu.Unlock()

	if elem, ok := p.entries[id]; ok {
		entry := elem.Value.(*prepEntry)
		entry.mu.Lock()
		if entry.phase == PhaseReady && entry.source != nil {
			src := entry.source
			entry.mu.Unlock()
			p.lru.Remove(elem)
			delete(p.entries, id)
			return src
		}
		entry.mu.Unlock()
	}
	return nil
}

// Len returns the number of cached entries (for testing).
func (p *SessionPreparations) Len() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.entries)
}

// --- internal helpers ---

func (p *SessionPreparations) entryFor(id string, load func() (*PreparedSource, error)) *prepEntry {
	p.mu.Lock()
	defer p.mu.Unlock()

	if elem, ok := p.entries[id]; ok {
		return elem.Value.(*prepEntry)
	}

	// Create new entry.
	entry := &prepEntry{
		id:     id,
		phase:  PhaseLoading,
		result: make(chan struct{}),
	}
	elem := p.lru.PushBack(entry)
	p.entries[id] = elem

	// Start the load asynchronously.
	go func() {
		source, err := load()
		p.mu.Lock()
		if p.entries[id] != elem {
			p.mu.Unlock()
			return
		}
		entry.mu.Lock()
		if err != nil {
			entry.loadErr = err
			close(entry.result)
			entry.mu.Unlock()
			p.lru.Remove(elem)
			delete(p.entries, id)
			p.mu.Unlock()
			return
		}
		entry.source = source
		entry.phase = PhaseReady
		close(entry.result)
		entry.mu.Unlock()
		p.lru.MoveToBack(elem)
		p.evictLocked()
		p.mu.Unlock()
	}()

	return entry
}

func (p *SessionPreparations) removeEntry(id string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if elem, ok := p.entries[id]; ok {
		p.lru.Remove(elem)
		delete(p.entries, id)
	}
}

// settleAndRemove closes the settle channel and removes the entry from the LRU.
func (p *SessionPreparations) settleAndRemove(id string, entry *prepEntry) {
	entry.mu.Lock()
	ch := entry.settleCh
	entry.settleCh = nil
	entry.mu.Unlock()
	if ch != nil {
		close(ch)
	}
	p.removeEntry(id)
}

func (e *prepEntry) touchLRU() {
	// No-op: LRU touch is handled by the parent SessionPreparations
	// via MoveToBack when it accesses the list element.
}

func (p *SessionPreparations) evictLocked() {
	readyCount := 0
	for _, elem := range p.entries {
		entry := elem.Value.(*prepEntry)
		entry.mu.Lock()
		if entry.phase == PhaseReady {
			readyCount++
		}
		entry.mu.Unlock()
	}
	if readyCount <= p.capacity {
		return
	}
	// Evict the least-recently-used ready entry.
	for elem := p.lru.Front(); elem != nil; elem = elem.Next() {
		entry := elem.Value.(*prepEntry)
		entry.mu.Lock()
		isReady := entry.phase == PhaseReady
		entry.mu.Unlock()
		if isReady {
			p.lru.Remove(elem)
			delete(p.entries, entry.id)
			readyCount--
			if readyCount <= p.capacity {
				return
			}
		}
	}
}

var errPreparationIncomplete = newError("preparation incomplete")

type simpleError struct{ msg string }

func (e *simpleError) Error() string { return e.msg }

func newError(msg string) error { return &simpleError{msg: msg} }
