package mission

import (
	"sync"
	"time"
)

// Family messaging adopted from Prime Agent's agent-messages: running agents
// can exchange messages directly, but reach is scoped to a family graph
// (parent / sibling / child) rather than global, with per-session pending caps
// and token-bucket rate limiting to prevent one agent flooding another.

// FamilyRole is an agent's relationship to another.
type FamilyRole string

const (
	FamilyParent  FamilyRole = "parent"
	FamilySibling FamilyRole = "sibling"
	FamilyChild   FamilyRole = "child"
)

// FamilyLinks describes one agent's known family.
type FamilyLinks struct {
	Parent   string   `json:"parent,omitempty"`
	Siblings []string `json:"siblings,omitempty"`
	Children []string `json:"children,omitempty"`
}

// FamilyMessengerConfig controls rate limiting.
type FamilyMessengerConfig struct {
	MaxPendingPerAgent int     `json:"max_pending_per_agent,omitempty"`
	RefillPerSec       float64 `json:"refill_per_sec,omitempty"` // token bucket refill
	Capacity           float64 `json:"capacity,omitempty"`
}

func (c FamilyMessengerConfig) withDefaults() FamilyMessengerConfig {
	if c.MaxPendingPerAgent <= 0 {
		c.MaxPendingPerAgent = 20
	}
	if c.RefillPerSec <= 0 {
		c.RefillPerSec = 3
	}
	if c.Capacity <= 0 {
		c.Capacity = 3
	}
	return c
}

// FamilyMessage is one inter-agent message.
type FamilyMessage struct {
	From    string     `json:"from"`
	To      string     `json:"to"`
	Role    FamilyRole `json:"role"`
	Content string     `json:"content"`
	SentAt  time.Time  `json:"sent_at"`
}

// bucket is a token bucket for one destination.
type bucket struct {
	tokens float64
	last   time.Time
}

// FamilyMessenger delivers messages within a family graph with rate limits.
type FamilyMessenger struct {
	mu      sync.Mutex
	links   map[string]FamilyLinks
	inbox   map[string][]FamilyMessage
	buckets map[string]*bucket
	cfg     FamilyMessengerConfig
}

// NewFamilyMessenger creates a messenger.
func NewFamilyMessenger(cfg FamilyMessengerConfig) *FamilyMessenger {
	return &FamilyMessenger{
		links:   map[string]FamilyLinks{},
		inbox:   map[string][]FamilyMessage{},
		buckets: map[string]*bucket{},
		cfg:     cfg.withDefaults(),
	}
}

// Register sets an agent's family links.
func (m *FamilyMessenger) Register(agentID string, links FamilyLinks) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.links[agentID] = links
}

// RoleBetween returns the family relationship from -> to, or "" if none.
func (m *FamilyMessenger) RoleBetween(from, to string) FamilyRole {
	m.mu.Lock()
	defer m.mu.Unlock()
	links := m.links[from]
	if links.Parent == to {
		return FamilyParent
	}
	for _, s := range links.Siblings {
		if s == to {
			return FamilySibling
		}
	}
	for _, c := range links.Children {
		if c == to {
			return FamilyChild
		}
	}
	return ""
}

// Allowed reports whether from may message to within the family graph.
func (m *FamilyMessenger) Allowed(from, to string) bool { return m.RoleBetween(from, to) != "" }

// Send delivers a message if relationship, rate limit, and pending cap allow.
// Returns accepted=true when delivered.
func (m *FamilyMessenger) Send(from, to, content string) (bool, FamilyRole) {
	m.mu.Lock()
	defer m.mu.Unlock()
	role := m.roleBetweenLocked(from, to)
	if role == "" {
		return false, ""
	}
	if len(m.inbox[to]) >= m.cfg.MaxPendingPerAgent {
		return false, role // pending cap hit
	}
	b := m.bucketForLocked(to)
	if b.tokens < 1 {
		return false, role // rate limited
	}
	b.tokens--
	m.inbox[to] = append(m.inbox[to], FamilyMessage{
		From: from, To: to, Role: role, Content: content, SentAt: time.Now(),
	})
	return true, role
}

// Receive drains all pending messages for an agent.
func (m *FamilyMessenger) Receive(agentID string) []FamilyMessage {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := m.inbox[agentID]
	delete(m.inbox, agentID)
	return out
}

// Pending returns the count of undelivered messages for an agent.
func (m *FamilyMessenger) Pending(agentID string) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.inbox[agentID])
}

func (m *FamilyMessenger) roleBetweenLocked(from, to string) FamilyRole {
	links := m.links[from]
	if links.Parent == to {
		return FamilyParent
	}
	for _, s := range links.Siblings {
		if s == to {
			return FamilySibling
		}
	}
	for _, c := range links.Children {
		if c == to {
			return FamilyChild
		}
	}
	return ""
}

func (m *FamilyMessenger) bucketForLocked(dest string) *bucket {
	b, ok := m.buckets[dest]
	if !ok {
		b = &bucket{tokens: m.cfg.Capacity, last: time.Now()}
		m.buckets[dest] = b
		return b
	}
	elapsed := time.Since(b.last).Seconds()
	refill := elapsed * m.cfg.RefillPerSec
	b.tokens += refill
	if b.tokens > m.cfg.Capacity {
		b.tokens = m.cfg.Capacity
	}
	b.last = time.Now()
	return b
}
