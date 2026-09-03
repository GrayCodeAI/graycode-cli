package schedule

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/GrayCodeAI/graycode-cli/internal/eventlog"
)

// DeliverFn delivers a due scheduled reminder into the active conversation.
type DeliverFn func(item Item) error

// Manager coordinates in-conversation schedule timers and eventlog persistence.
type Manager struct {
	mu        sync.Mutex
	journal   *eventlog.Log
	deliverFn DeliverFn
	timers    map[string]*time.Timer
	closed    bool
}

// NewManager creates an unattached schedule Manager.
func NewManager() *Manager {
	return &Manager{
		timers: make(map[string]*time.Timer),
	}
}

// Attach attaches the manager to a session journal and delivery sink.
// It folds existing events, delivers overdue items immediately (cold session catchup),
// and arms timers for pending future items.
func (m *Manager) Attach(journal *eventlog.Log, deliverFn DeliverFn) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Cancel existing timers
	for _, t := range m.timers {
		t.Stop()
	}
	m.timers = make(map[string]*time.Timer)

	m.journal = journal
	m.deliverFn = deliverFn
	m.closed = false

	if journal == nil {
		return
	}

	// Fold active items
	active := Fold(journal.Snapshot())
	now := time.Now()

	for _, item := range active {
		if !item.DueAt.After(now) {
			// Overdue work from cold session resume
			m.triggerDueLocked(item, now)
		} else {
			// Arm future timer
			m.armTimerLocked(item)
		}
	}
}

// Create registers a new scheduled reminder on the session log and arms its timer.
func (m *Manager) Create(prompt string, dueAt time.Time, interval string, recurring bool) (*Item, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed {
		return nil, fmt.Errorf("schedule manager is closed")
	}

	id := generateScheduleID()
	fact := eventlog.ScheduleCreateFact{
		ID:        id,
		Prompt:    prompt,
		DueAt:     dueAt,
		Interval:  interval,
		Recurring: recurring,
	}

	if m.journal != nil {
		m.journal.AppendScheduleCreate(fact)
	}

	item := Item{
		ID:        id,
		Prompt:    prompt,
		DueAt:     dueAt,
		Interval:  interval,
		Recurring: recurring,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	now := time.Now()
	if !dueAt.After(now) {
		m.triggerDueLocked(item, now)
	} else {
		m.armTimerLocked(item)
	}

	return &item, nil
}

// List returns active, non-deleted schedules sorted by DueAt ascending.
func (m *Manager) List() []Item {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.journal == nil {
		return nil
	}

	activeMap := Fold(m.journal.Snapshot())
	items := make([]Item, 0, len(activeMap))
	for _, item := range activeMap {
		items = append(items, item)
	}

	sort.Slice(items, func(i, j int) bool {
		return items[i].DueAt.Before(items[j].DueAt)
	})
	return items
}

// Delete cancels an in-conversation schedule and logs a schedule.delete event.
func (m *Manager) Delete(id, reason string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if timer, ok := m.timers[id]; ok {
		timer.Stop()
		delete(m.timers, id)
	}

	if m.journal != nil {
		m.journal.AppendScheduleDelete(id, reason)
	}
	return nil
}

// Close disposes all running timers.
func (m *Manager) Close() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closed = true
	for _, t := range m.timers {
		t.Stop()
	}
	m.timers = make(map[string]*time.Timer)
}

func (m *Manager) armTimerLocked(item Item) {
	if m.closed {
		return
	}
	dur := time.Until(item.DueAt)
	if dur < 0 {
		dur = 0
	}

	timer := time.AfterFunc(dur, func() {
		m.mu.Lock()
		defer m.mu.Unlock()
		if m.closed {
			return
		}
		// Confirm item is still active
		if m.journal != nil {
			active := Fold(m.journal.Snapshot())
			current, exists := active[item.ID]
			if exists && !current.Deleted {
				m.triggerDueLocked(current, time.Now())
			}
		}
	})
	m.timers[item.ID] = timer
}

func (m *Manager) triggerDueLocked(item Item, triggeredAt time.Time) {
	if m.closed {
		return
	}

	// 1. Deliver in-conversation
	if m.deliverFn != nil {
		_ = m.deliverFn(item)
	}

	// 2. Compute next due time if recurring
	var nextDueAt *time.Time
	if item.Recurring {
		next := calculateNextDue(triggeredAt, item.Interval)
		nextDueAt = &next
	}

	// 3. Log schedule.due event
	if m.journal != nil {
		m.journal.AppendScheduleDue(eventlog.ScheduleDueFact{
			ID:          item.ID,
			DeliveredAt: triggeredAt,
			NextDueAt:   nextDueAt,
		})
	}

	delete(m.timers, item.ID)

	// 4. Rearm timer if recurring
	if item.Recurring && nextDueAt != nil {
		updatedItem := item
		updatedItem.DueAt = *nextDueAt
		m.armTimerLocked(updatedItem)
	}
}

func calculateNextDue(from time.Time, interval string) time.Time {
	if interval == "" {
		return from.Add(5 * time.Minute)
	}
	// Parse simple durations like "5m", "1h", "30s"
	if dur, err := time.ParseDuration(interval); err == nil && dur > 0 {
		return from.Add(dur)
	}
	return from.Add(5 * time.Minute)
}

func generateScheduleID() string {
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	return fmt.Sprintf("sched-%x", hex.EncodeToString(b))
}
