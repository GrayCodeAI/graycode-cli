package mission

import (
	"crypto/rand"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// ResourceLock represents an exclusive lock on a shared resource (e.g. a file).
// Prevents conflicting operations when multiple agents work in parallel.
type ResourceLock struct {
	Resource   string    `json:"resource"`
	Owner      string    `json:"owner"`
	AcquiredAt time.Time `json:"acquired_at"`
	ExpiresAt  time.Time `json:"expires_at"`
	Priority   int       `json:"priority"`
}

// AgentMessage represents a message exchanged between agents during a mission.
type AgentMessage struct {
	ID               string    `json:"id"`
	From             string    `json:"from"`
	To               string    `json:"to,omitempty"`
	Topic            string    `json:"topic"`
	Content          string    `json:"content"`
	Priority         int       `json:"priority"`
	Timestamp        time.Time `json:"timestamp"`
	RequiresResponse bool      `json:"requires_response,omitempty"`
	ResponseTo       string    `json:"response_to,omitempty"`
}

// MessageBus coordinates inter-agent communication during mission execution.
type MessageBus struct {
	channels    map[string]chan AgentMessage
	subscribers map[string][]string // topic -> agent IDs
	history     []AgentMessage
	mu          sync.RWMutex

	locks  map[string]*ResourceLock // resource -> current lock
	lockMu sync.Mutex               // separate mutex for lock operations

	// droppedCount counts messages that could not be delivered because
	// the target agent's channel was full. Incremented atomically so it
	// can be read via Stats() without acquiring mu. Surfaced via WARN logs
	// (sampled to avoid spam — see logDroppedMessage).
	droppedCount atomic.Int64

	// responseWaiters tracks in-flight WaitForResponse calls, keyed by
	// the messageID they are waiting on. Send() closes the waiter's
	// done channel when a matching response is appended to history,
	// replacing the old 10ms busy-poll.
	//
	// Protected by mu (the same lock that protects history — the two
	// are always updated together under the same critical section).
	responseWaiters map[string][]*responseWaiter

	// lockWaiters tracks in-flight WaitForLock calls, keyed by resource.
	// ReleaseLock() closes the waiter's done channel, replacing the old
	// 20ms busy-poll.
	//
	// Protected by lockMu (the same lock that protects locks).
	lockWaiters map[string][]*lockWaiter
}

// responseWaiter is a single in-flight WaitForResponse call.
type responseWaiter struct {
	done chan struct{} // closed by Send when a matching response arrives
	msg  *AgentMessage // populated by Send before closing done
}

// lockWaiter is a single in-flight WaitForLock call.
type lockWaiter struct {
	done chan struct{} // closed by ReleaseLock when the lock is released
}

// BusStats is a snapshot of MessageBus runtime counters.
type BusStats struct {
	// Dropped is the cumulative number of messages that could not be
	// delivered to a registered agent because its channel was full.
	// Includes both broadcast and direct-send drops.
	Dropped int64
	// Agents is the number of currently registered agents.
	Agents int
	// Locks is the number of currently held resource locks.
	Locks int
	// HistorySz is the number of messages retained in history.
	HistorySz int
}

// Stats returns a snapshot of MessageBus counters. Safe for concurrent use.
func (mb *MessageBus) Stats() BusStats {
	mb.mu.RLock()
	defer mb.mu.RUnlock()
	return BusStats{
		Dropped:   mb.droppedCount.Load(),
		Agents:    len(mb.channels),
		Locks:     len(mb.locks),
		HistorySz: len(mb.history),
	}
}

// dropLogEveryN controls the sampling rate for dropped-message
// WARN logs. The first drop and then every Nth drop are logged;
// the rest are silent (just bump the counter). The default (100)
// balances observability against log volume when an agent is stuck
// or the bus is under sustained pressure. Lower it for more
// visibility; raise it for quieter logs.
const dropLogEveryN = 100

// logDroppedMessage records a dropped-message event. Sampling: logs the
// first drop and then every Nth drop (see dropLogEveryN), to avoid
// log spam when an agent is stuck or the bus is under sustained
// pressure (see M9 in the code review).
//
// IMPORTANT: callers MUST hold mb.mu (write lock) when invoking this
// method. The sampling decision uses the atomic droppedCount to avoid
// holding mb.mu for the slog.Warn call (which can serialize on the
// slog handler's internal mutex if the handler is slow — see M8).
// The actual slog.Warn runs synchronously; for high-throughput drop
// scenarios consider a buffered channel + background drainer.
func (mb *MessageBus) logDroppedMessage(from, to, topic string) {
	n := mb.droppedCount.Load()
	if n != 1 && n%dropLogEveryN != 0 {
		return
	}
	slog.Warn(
		"message bus: dropped message (channel full)",
		"from", from,
		"to", to,
		"topic", topic,
		"dropped_total", n,
	)
}

// NewMessageBus creates and returns an initialized MessageBus.
func NewMessageBus() *MessageBus {
	return &MessageBus{
		channels:        make(map[string]chan AgentMessage),
		subscribers:     make(map[string][]string),
		history:         make([]AgentMessage, 0),
		locks:           make(map[string]*ResourceLock),
		responseWaiters: make(map[string][]*responseWaiter),
		lockWaiters:     make(map[string][]*lockWaiter),
	}
}

// Register creates a channel for the given agent to receive messages.
// Returns a read-only channel the agent can listen on.
func (mb *MessageBus) Register(agentID string) <-chan AgentMessage {
	mb.mu.Lock()
	defer mb.mu.Unlock()

	ch := make(chan AgentMessage, 64)
	mb.channels[agentID] = ch
	return ch
}

// Unregister removes an agent from the message bus and closes its channel.
func (mb *MessageBus) Unregister(agentID string) {
	mb.mu.Lock()
	defer mb.mu.Unlock()

	if ch, ok := mb.channels[agentID]; ok {
		close(ch)
		delete(mb.channels, agentID)
	}

	// Remove from all topic subscriptions
	for topic, agents := range mb.subscribers {
		filtered := make([]string, 0, len(agents))
		for _, a := range agents {
			if a != agentID {
				filtered = append(filtered, a)
			}
		}
		mb.subscribers[topic] = filtered
	}
}

// Send delivers a message to a specific agent or broadcasts to all.
// If msg.To is set, delivers to that specific agent.
// If msg.To is empty, broadcasts to all registered agents (except sender).
//
// The dropped-message WARN log is performed after mb.mu is released
// (see M8): a slow slog handler should not serialize bus operations.
// We snapshot the (from, to, topic) of each drop into a local slice
// while still holding the lock, then iterate the slice after the
// inner function returns and the lock has been released.
func (mb *MessageBus) Send(msg AgentMessage) error {
	// sentErr is set inside the locked section for two cases:
	//   - msg.To names an agent that isn't registered
	//   - msg.To names a registered agent whose channel is full
	// The first case is a hard error; the second is "channel full
	// for agent %q" (the original error return) and is also propagated
	// to the caller. Both were returned under the lock in the old
	// implementation; the new implementation collects the error and
	// returns it after the lock is released.
	var sentErr error
	var drops []dropEvent

	func() {
		mb.mu.Lock()
		defer mb.mu.Unlock()

		if msg.ID == "" {
			msg.ID = generateID()
		}
		if msg.Timestamp.IsZero() {
			msg.Timestamp = time.Now()
		}
		if msg.Priority == 0 {
			msg.Priority = 3
		}

		mb.history = append(mb.history, msg)

		// Signal any in-flight WaitForResponse callers. Must happen while
		// still holding mb.mu so the waiter's defer-cleanup can't race
		// with us (the waiter's defer also acquires mb.mu to remove itself).
		if msg.ResponseTo != "" {
			if waiters := mb.responseWaiters[msg.ResponseTo]; len(waiters) > 0 {
				for _, w := range waiters {
					w.msg = &msg
					close(w.done)
				}
				delete(mb.responseWaiters, msg.ResponseTo)
			}
		}

		if msg.To != "" {
			ch, ok := mb.channels[msg.To]
			if !ok {
				sentErr = fmt.Errorf("agent %q not registered", msg.To)
				return
			}
			select {
			case ch <- msg:
			default:
				mb.droppedCount.Add(1)
				sentErr = fmt.Errorf("channel full for agent %q", msg.To)
				drops = append(drops, dropEvent{from: msg.From, to: msg.To, topic: msg.Topic})
			}
			return
		}

		// Broadcast to all agents except sender
		for agentID, ch := range mb.channels {
			if agentID == msg.From {
				continue
			}
			// If topic-based, only send to subscribers of that topic
			if msg.Topic != "" && len(mb.subscribers[msg.Topic]) > 0 {
				if !contains(mb.subscribers[msg.Topic], agentID) {
					continue
				}
			}
			select {
			case ch <- msg:
			default:
				mb.droppedCount.Add(1)
				drops = append(drops, dropEvent{from: msg.From, to: agentID, topic: msg.Topic})
			}
		}
	}()

	// Log drops outside the lock so a slow slog handler (file I/O,
	// network sink) doesn't serialize bus operations (M8 fix).
	for _, d := range drops {
		mb.logDroppedMessage(d.from, d.to, d.topic)
	}

	return sentErr
}

// dropEvent is a snapshot of a single drop, captured under mb.mu and
// logged outside the lock (M8). The fields are plain copies; no
// pointers into bus state.
type dropEvent struct {
	from  string
	to    string
	topic string
}

// Subscribe registers an agent to receive messages for a given topic.
func (mb *MessageBus) Subscribe(agentID, topic string) {
	mb.mu.Lock()
	defer mb.mu.Unlock()

	// Avoid duplicate subscriptions
	for _, a := range mb.subscribers[topic] {
		if a == agentID {
			return
		}
	}
	mb.subscribers[topic] = append(mb.subscribers[topic], agentID)
}

// Broadcast sends a message from one agent to all others.
// Broadcasts never fail: agents with full buffers are silently skipped.
func (mb *MessageBus) Broadcast(from, topic, content string) {
	msg := AgentMessage{
		ID:        generateID(),
		From:      from,
		Topic:     topic,
		Content:   content,
		Priority:  3,
		Timestamp: time.Now(),
	}
	// Safe to ignore: broadcast (no To) never returns errors.
	_ = mb.Send(msg)
}

// RequestHelp broadcasts a help request and returns the message ID for tracking responses.
func (mb *MessageBus) RequestHelp(from, description string) string {
	msg := AgentMessage{
		ID:               generateID(),
		From:             from,
		Topic:            "request",
		Content:          description,
		Priority:         1,
		Timestamp:        time.Now(),
		RequiresResponse: true,
	}
	// Safe to ignore: broadcast (no To) never returns errors.
	_ = mb.Send(msg)
	return msg.ID
}

// ReportConflict notifies all agents about a file conflict so they can coordinate.
func (mb *MessageBus) ReportConflict(from string, files []string, description string) {
	content := fmt.Sprintf("conflict on files [%s]: %s", strings.Join(files, ", "), description)
	msg := AgentMessage{
		ID:        generateID(),
		From:      from,
		Topic:     "conflict",
		Content:   content,
		Priority:  1,
		Timestamp: time.Now(),
	}
	// Safe to ignore: broadcast (no To) never returns errors.
	_ = mb.Send(msg)
}

// ReportDiscovery shares a useful finding with all agents.
func (mb *MessageBus) ReportDiscovery(from, discovery string) {
	msg := AgentMessage{
		ID:        generateID(),
		From:      from,
		Topic:     "discovery",
		Content:   discovery,
		Priority:  3,
		Timestamp: time.Now(),
	}
	// Safe to ignore: broadcast (no To) never returns errors.
	_ = mb.Send(msg)
}

// ReportProgress sends a progress update for coordination.
func (mb *MessageBus) ReportProgress(from string, pct float64, status string) {
	content := fmt.Sprintf("%.0f%%: %s", pct, status)
	msg := AgentMessage{
		ID:        generateID(),
		From:      from,
		Topic:     "progress",
		Content:   content,
		Priority:  5,
		Timestamp: time.Now(),
	}
	// Safe to ignore: broadcast (no To) never returns errors.
	_ = mb.Send(msg)
}

// GetHistory returns messages filtered by topic (or all if topic is empty),
// limited to the most recent `limit` entries.
func (mb *MessageBus) GetHistory(topic string, limit int) []AgentMessage {
	mb.mu.RLock()
	defer mb.mu.RUnlock()

	var filtered []AgentMessage
	for i := len(mb.history) - 1; i >= 0; i-- {
		if topic == "" || mb.history[i].Topic == topic {
			filtered = append(filtered, mb.history[i])
			if limit > 0 && len(filtered) >= limit {
				break
			}
		}
	}

	// Reverse to chronological order
	for i, j := 0, len(filtered)-1; i < j; i, j = i+1, j-1 {
		filtered[i], filtered[j] = filtered[j], filtered[i]
	}
	return filtered
}

// WaitForResponse blocks until a response to the given messageID arrives or timeout elapses.
//
// Implementation: push-based via a per-call done channel. The caller
// registers as a waiter; Send() closes the done channel when a matching
// response is appended to history. This replaces the previous 10ms
// busy-poll (which missed sub-tick responses and burned CPU under load).
//
// Race: a response may have been appended to history just before the
// waiter registers. The fast path below checks history first, so a
// late-arriving WaitForResponse still finds the answer.
func (mb *MessageBus) WaitForResponse(messageID string, timeout time.Duration) (*AgentMessage, error) {
	// Fast path: check history for an already-recorded response.
	if msg := mb.findResponse(messageID); msg != nil {
		return msg, nil
	}

	// Slow path: register as waiter, wait for done or timeout.
	w := &responseWaiter{done: make(chan struct{})}

	mb.mu.Lock()
	// Double-check history under the write lock to close the race with Send.
	if msg := mb.findResponseLocked(messageID); msg != nil {
		mb.mu.Unlock()
		return msg, nil
	}
	mb.responseWaiters[messageID] = append(mb.responseWaiters[messageID], w)
	mb.mu.Unlock()

	defer mb.removeResponseWaiter(messageID, w)

	select {
	case <-w.done:
		if w.msg == nil {
			return nil, errors.New("response waiter signaled without message")
		}
		return w.msg, nil
	case <-time.After(timeout):
		return nil, errors.New("timeout waiting for response")
	}
}

// findResponse scans history for a response to messageID. Read-locked.
func (mb *MessageBus) findResponse(messageID string) *AgentMessage {
	mb.mu.RLock()
	defer mb.mu.RUnlock()
	return findResponseInSlice(mb.history, messageID)
}

// findResponseLocked is the write-lock variant (caller holds mb.mu).
func (mb *MessageBus) findResponseLocked(messageID string) *AgentMessage {
	return findResponseInSlice(mb.history, messageID)
}

func findResponseInSlice(history []AgentMessage, messageID string) *AgentMessage {
	for i := range history {
		if history[i].ResponseTo == messageID {
			msg := history[i]
			return &msg
		}
	}
	return nil
}

// removeResponseWaiter is the defer-cleanup for WaitForResponse.
// Removes the waiter from the per-messageID list so a later response
// (or a never-arriving response that timed out) doesn't leave a
// dangling entry. Safe to call after the waiter's done has already
// been closed — the entry has already been removed by Send.
func (mb *MessageBus) removeResponseWaiter(messageID string, w *responseWaiter) {
	mb.mu.Lock()
	defer mb.mu.Unlock()
	waiters := mb.responseWaiters[messageID]
	for i, ww := range waiters {
		if ww == w {
			mb.responseWaiters[messageID] = append(waiters[:i], waiters[i+1:]...)
			return
		}
	}
}

// BuildContextFromMessages formats recent relevant messages for injection into an agent's context.
// It returns a human-readable summary of team communication limited to approximately maxTokens characters.
func (mb *MessageBus) BuildContextFromMessages(agentID string, maxTokens int) string {
	mb.mu.RLock()
	defer mb.mu.RUnlock()

	var lines []string
	lines = append(lines, "## Team Communication")

	// Walk history in reverse (newest first) and collect relevant messages
	for i := len(mb.history) - 1; i >= 0; i-- {
		msg := mb.history[i]
		// Skip messages from the requesting agent itself
		if msg.From == agentID {
			continue
		}
		// Skip messages targeted to other agents
		if msg.To != "" && msg.To != agentID {
			continue
		}

		var line string
		switch msg.Topic {
		case "discovery":
			line = fmt.Sprintf("[%s] discovered: %q", msg.From, msg.Content)
		case "conflict":
			line = fmt.Sprintf("[%s] conflict: %s", msg.From, msg.Content)
		case "progress":
			line = fmt.Sprintf("[%s] progress: %s", msg.From, msg.Content)
		case "request":
			line = fmt.Sprintf("[%s] needs help: %s", msg.From, msg.Content)
		case "complete":
			line = fmt.Sprintf("[%s] completed: %s", msg.From, msg.Content)
		default:
			line = fmt.Sprintf("[%s] %s: %s", msg.From, msg.Topic, msg.Content)
		}

		lines = append(lines, line)
	}

	result := strings.Join(lines, "\n")
	if maxTokens > 0 && len(result) > maxTokens {
		result = result[:maxTokens]
		// Trim to last complete line
		if idx := strings.LastIndex(result, "\n"); idx > 0 {
			result = result[:idx]
		}
	}
	return result
}

// AcquireLock attempts to acquire an exclusive lock on a resource.
// Returns nil on success, or an error if the resource is held by another agent.
// A lock held by the same owner is refreshed (re-entrant). Expired locks are reclaimed.
func (mb *MessageBus) AcquireLock(resource, owner string, ttl time.Duration) error {
	mb.lockMu.Lock()
	defer mb.lockMu.Unlock()

	if existing, ok := mb.locks[resource]; ok {
		if time.Now().Before(existing.ExpiresAt) && existing.Owner != owner {
			return fmt.Errorf("resource %q locked by %q until %s", resource, existing.Owner, existing.ExpiresAt.Format(time.RFC3339))
		}
	}

	mb.locks[resource] = &ResourceLock{
		Resource:   resource,
		Owner:      owner,
		AcquiredAt: time.Now(),
		ExpiresAt:  time.Now().Add(ttl),
		Priority:   3,
	}
	return nil
}

// ReleaseLock releases a lock on a resource, verifying ownership.
func (mb *MessageBus) ReleaseLock(resource, owner string) error {
	mb.lockMu.Lock()
	defer mb.lockMu.Unlock()

	existing, ok := mb.locks[resource]
	if !ok {
		return nil // already released; idempotent
	}
	if existing.Owner != owner {
		return fmt.Errorf("resource %q is owned by %q, not %q", resource, existing.Owner, owner)
	}
	delete(mb.locks, resource)

	// Wake up any in-flight WaitForLock callers. Each waiter's done
	// channel is closed exactly once; the subsequent AcquireLock
	// race-loser will loop and re-register (or be cleaned up by
	// its defer if it has already timed out).
	if waiters := mb.lockWaiters[resource]; len(waiters) > 0 {
		for _, w := range waiters {
			close(w.done)
		}
		delete(mb.lockWaiters, resource)
	}

	return nil
}

// IsLocked reports whether a resource is currently locked by any agent.
// Expired locks are treated as unlocked.
func (mb *MessageBus) IsLocked(resource string) bool {
	mb.lockMu.Lock()
	defer mb.lockMu.Unlock()

	existing, ok := mb.locks[resource]
	if !ok {
		return false
	}
	return time.Now().Before(existing.ExpiresAt)
}

// WaitForLock blocks until a resource lock can be acquired or the timeout elapses.
//
// Implementation: push-based via a per-call done channel. The caller
// registers as a waiter; ReleaseLock() closes the done channel when
// the lock is freed. This replaces the previous 20ms busy-poll.
//
// Once ReleaseLock closes our done channel, the channel stays closed
// for the lifetime of the waiter. The previous implementation re-entered
// the select after each signal, which (a) busy-spinned calling
// AcquireLock on the same closed-channel signal under contention and
// (b) raced the timer.C case: both `<-w.done` and `<-timer.C` are
// ready, and select picks randomly — so the timeout could be missed
// or the function could loop indefinitely waiting on the second signal
// from a ReleaseLock that never re-fires. (See M7 in the code review.)
//
// Fix: use a one-shot select. On the first signal (w.done or timer),
// try AcquireLock exactly once. If it succeeds, return nil; if the
// timer fired, return a timeout error. There is no second loop, so
// no busy-spin and no select race.
func (mb *MessageBus) WaitForLock(resource, owner string, timeout time.Duration) error {
	// Fast path: try immediately; the lock can be acquired without
	// waiting if no one else holds it.
	if err := mb.AcquireLock(resource, owner, timeout); err == nil {
		return nil
	}

	w := &lockWaiter{done: make(chan struct{})}

	mb.lockMu.Lock()
	mb.lockWaiters[resource] = append(mb.lockWaiters[resource], w)
	mb.lockMu.Unlock()

	defer mb.removeLockWaiter(resource, w)

	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case <-w.done:
		// Lock was released by another agent. Try to acquire it.
		// If we lose the race to a sibling waiter, return an error
		// rather than re-registering and looping (M7 fix: no busy-spin).
		if err := mb.AcquireLock(resource, owner, timeout); err != nil {
			return err
		}
		return nil
	case <-timer.C:
		return fmt.Errorf("timeout waiting for lock on %q", resource)
	}
}

// removeLockWaiter is the defer-cleanup for WaitForLock. Symmetric
// with removeResponseWaiter.
func (mb *MessageBus) removeLockWaiter(resource string, w *lockWaiter) {
	mb.lockMu.Lock()
	defer mb.lockMu.Unlock()
	waiters := mb.lockWaiters[resource]
	for i, ww := range waiters {
		if ww == w {
			mb.lockWaiters[resource] = append(waiters[:i], waiters[i+1:]...)
			return
		}
	}
}

// CleanupExpiredLocks removes all locks whose expiry time has passed.
func (mb *MessageBus) CleanupExpiredLocks() {
	mb.lockMu.Lock()
	defer mb.lockMu.Unlock()

	now := time.Now()
	for resource, lock := range mb.locks {
		if now.After(lock.ExpiresAt) {
			delete(mb.locks, resource)
		}
	}
}

// TryLockFiles attempts to acquire locks on a set of files for an agent.
// Returns the list of files that could NOT be locked (held by others).
// On any contention, it also reports a conflict via the message bus.
func (mb *MessageBus) TryLockFiles(from string, files []string, ttl time.Duration) []string {
	var failed []string
	for _, f := range files {
		if err := mb.AcquireLock(f, from, ttl); err != nil {
			failed = append(failed, f)
		}
	}
	if len(failed) > 0 {
		mb.ReportConflict(from, failed, "could not acquire exclusive locks")
	}
	return failed
}

// generateID creates a random hex ID suitable for message identification.
func generateID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return fmt.Sprintf("%x", b)
}

// contains checks if a slice contains a given string.
func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}
