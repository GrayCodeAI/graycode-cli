package mission

import (
	"crypto/rand"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
)

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
}

// NewMessageBus creates and returns an initialized MessageBus.
func NewMessageBus() *MessageBus {
	return &MessageBus{
		channels:    make(map[string]chan AgentMessage),
		subscribers: make(map[string][]string),
		history:     make([]AgentMessage, 0),
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
func (mb *MessageBus) Send(msg AgentMessage) error {
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

	if msg.To != "" {
		// Deliver to specific agent
		ch, ok := mb.channels[msg.To]
		if !ok {
			return fmt.Errorf("agent %q not registered", msg.To)
		}
		select {
		case ch <- msg:
		default:
			return fmt.Errorf("channel full for agent %q", msg.To)
		}
		return nil
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
			// Skip agents with full buffers rather than blocking
		}
	}
	return nil
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
func (mb *MessageBus) WaitForResponse(messageID string, timeout time.Duration) (*AgentMessage, error) {
	deadline := time.After(timeout)
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-deadline:
			return nil, errors.New("timeout waiting for response")
		case <-ticker.C:
			mb.mu.RLock()
			for i := range mb.history {
				if mb.history[i].ResponseTo == messageID {
					msg := mb.history[i]
					mb.mu.RUnlock()
					return &msg, nil
				}
			}
			mb.mu.RUnlock()
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
