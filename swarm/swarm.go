package swarm

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// AgentState represents the state of a teammate agent.
type AgentState string

const (
	AgentStateIdle       AgentState = "idle"
	AgentStateWorking    AgentState = "working"
	AgentStateWaiting    AgentState = "waiting"
	AgentStateShutdown   AgentState = "shutdown"
)

// Teammate represents an agent participating in a team.
type Teammate struct {
	ID        string     `json:"id"`
	Name      string     `json:"name"`
	TeamName  string     `json:"teamName"`
	AgentType string     `json:"agentType"`
	State     AgentState `json:"state"`
	TaskID    string     `json:"taskId,omitempty"`
	Color     string     `json:"color,omitempty"`
	JoinedAt  time.Time  `json:"joinedAt"`
}

// Coordinator manages a swarm of collaborating agents.
type Coordinator struct {
	mu         sync.RWMutex
	teammates  map[string]*Teammate
	teamName   string
	leaderName string
	msgChan    chan InternalMessage
}

// InternalMessage is a message between agents in a swarm.
type InternalMessage struct {
	From    string    `json:"from"`
	To      string    `json:"to"`
	Type    string    `json:"type"`
	Content string    `json:"content"`
	SentAt  time.Time `json:"sentAt"`
}

// NewCoordinator creates a new swarm coordinator.
func NewCoordinator(teamName, leaderName string) *Coordinator {
	return &Coordinator{
		teammates:  make(map[string]*Teammate),
		teamName:   teamName,
		leaderName: leaderName,
		msgChan:    make(chan InternalMessage, 100),
	}
}

func (c *Coordinator) TeamName() string  { return c.teamName }
func (c *Coordinator) LeaderName() string { return c.leaderName }

func (c *Coordinator) AddTeammate(name, agentType, color string) *Teammate {
	c.mu.Lock()
	defer c.mu.Unlock()
	t := &Teammate{
		ID:        fmt.Sprintf("%s_%s", c.teamName, name),
		Name:      name,
		TeamName:  c.teamName,
		AgentType: agentType,
		State:     AgentStateIdle,
		Color:     color,
		JoinedAt:  time.Now(),
	}
	c.teammates[name] = t
	return t
}

func (c *Coordinator) RemoveTeammate(name string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if t, ok := c.teammates[name]; ok {
		t.State = AgentStateShutdown
	}
	delete(c.teammates, name)
}

func (c *Coordinator) GetTeammate(name string) (*Teammate, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	t, ok := c.teammates[name]
	return t, ok
}

func (c *Coordinator) ListTeammates() []*Teammate {
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make([]*Teammate, 0, len(c.teammates))
	for _, t := range c.teammates {
		out = append(out, t)
	}
	return out
}

func (c *Coordinator) SetState(name string, state AgentState) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if t, ok := c.teammates[name]; ok {
		t.State = state
	}
}

func (c *Coordinator) AssignTask(name, taskID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if t, ok := c.teammates[name]; ok {
		t.TaskID = taskID
		t.State = AgentStateWorking
	}
}

func (c *Coordinator) SendMessage(msg InternalMessage) {
	msg.SentAt = time.Now()
	select {
	case c.msgChan <- msg:
	default:
		// Channel full, drop message
	}
}

func (c *Coordinator) ReceiveMessages(ctx context.Context, forName string) []InternalMessage {
	var msgs []InternalMessage
	for {
		select {
		case msg := <-c.msgChan:
			if msg.To == forName || msg.To == "*" {
				msgs = append(msgs, msg)
			} else {
				// Put it back for other consumers
				c.msgChan <- msg
				return msgs
			}
		case <-ctx.Done():
			return msgs
		default:
			return msgs
		}
	}
}

func (c *Coordinator) Shutdown() {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, t := range c.teammates {
		t.State = AgentStateShutdown
	}
	close(c.msgChan)
}

// SaveState persists the coordinator state to disk.
func (c *Coordinator) SaveState() error {
	c.mu.RLock()
	defer c.mu.RUnlock()

	home, _ := os.UserHomeDir()
	dir := filepath.Join(home, ".hawk", "teams", c.teamName)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	state := map[string]any{
		"teamName":   c.teamName,
		"leader":     c.leaderName,
		"teammates":  c.teammates,
		"savedAt":    time.Now().UTC().Format(time.RFC3339),
	}
	data, _ := json.MarshalIndent(state, "", "  ")
	return os.WriteFile(filepath.Join(dir, "state.json"), data, 0o644)
}

// Global coordinator registry.
var coordinators = struct {
	sync.RWMutex
	m map[string]*Coordinator
}{m: make(map[string]*Coordinator)}

// GetOrCreateCoordinator gets or creates a coordinator for the given team.
func GetOrCreateCoordinator(teamName, leaderName string) *Coordinator {
	coordinators.Lock()
	defer coordinators.Unlock()
	if c, ok := coordinators.m[teamName]; ok {
		return c
	}
	c := NewCoordinator(teamName, leaderName)
	coordinators.m[teamName] = c
	return c
}

// GetCoordinator returns an existing coordinator or nil.
func GetCoordinator(teamName string) *Coordinator {
	coordinators.RLock()
	defer coordinators.RUnlock()
	return coordinators.m[teamName]
}

// RemoveCoordinator removes and shuts down a coordinator.
func RemoveCoordinator(teamName string) {
	coordinators.Lock()
	defer coordinators.Unlock()
	if c, ok := coordinators.m[teamName]; ok {
		c.Shutdown()
		delete(coordinators.m, teamName)
	}
}
