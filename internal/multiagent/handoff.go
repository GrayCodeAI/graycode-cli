package mission

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

// HandoffMessage represents a control transfer message between agents.
type HandoffMessage struct {
	FromAgent string
	ToAgent   string
	Reason    string
	Context   string
	Task      string
	Priority  int
	State     map[string]interface{}
	Timestamp time.Time
}

// AgentCapability describes a specialized agent's capabilities.
type AgentCapability struct {
	Name          string
	Expertise     []string
	Tools         []string
	MaxComplexity int
	Model         string
}

// HandoffProtocol manages typed message-based control transfer between specialized agents.
type HandoffProtocol struct {
	Agents      map[string]*AgentCapability
	ActiveAgent string
	History     []HandoffMessage
	mu          sync.RWMutex
}

// NewHandoffProtocol creates a new HandoffProtocol with initialized fields.
func NewHandoffProtocol() *HandoffProtocol {
	return &HandoffProtocol{
		Agents:      make(map[string]*AgentCapability),
		ActiveAgent: "",
		History:     make([]HandoffMessage, 0),
	}
}

// RegisterAgent adds an agent capability to the protocol.
func (hp *HandoffProtocol) RegisterAgent(cap AgentCapability) {
	hp.mu.Lock()
	defer hp.mu.Unlock()

	hp.Agents[cap.Name] = &cap
	// If this is the first agent registered, set it as active.
	if hp.ActiveAgent == "" {
		hp.ActiveAgent = cap.Name
	}
}

// Handoff transfers control from one agent to another.
// It validates the target agent exists, transfers context and task,
// records the handoff in history, and sets the new active agent.
func (hp *HandoffProtocol) Handoff(msg HandoffMessage) error {
	hp.mu.Lock()
	defer hp.mu.Unlock()

	// Validate target agent exists.
	if _, exists := hp.Agents[msg.ToAgent]; !exists {
		return fmt.Errorf("handoff failed: agent %q not registered", msg.ToAgent)
	}

	// Set timestamp if not already provided.
	if msg.Timestamp.IsZero() {
		msg.Timestamp = time.Now()
	}

	// Record in history.
	hp.History = append(hp.History, msg)

	// Set new active agent.
	hp.ActiveAgent = msg.ToAgent

	return nil
}

// SelectBestAgent picks the best agent for a given task by matching task
// keywords against agent expertise and considering complexity.
func (hp *HandoffProtocol) SelectBestAgent(task string) string {
	hp.mu.RLock()
	defer hp.mu.RUnlock()

	taskLower := strings.ToLower(task)
	bestAgent := ""
	bestScore := 0

	for name, cap := range hp.Agents {
		score := 0
		for _, expertise := range cap.Expertise {
			if strings.Contains(taskLower, strings.ToLower(expertise)) {
				score++
			}
		}
		// Prefer agents with higher max complexity (more capable).
		if score > bestScore || (score == bestScore && cap.MaxComplexity > hp.agentComplexity(bestAgent)) {
			bestScore = score
			bestAgent = name
		}
	}

	// If no match found, return the active agent as fallback.
	if bestAgent == "" {
		return hp.ActiveAgent
	}

	return bestAgent
}

// agentComplexity returns the MaxComplexity for a given agent name.
// Must be called with at least a read lock held.
func (hp *HandoffProtocol) agentComplexity(name string) int {
	if cap, exists := hp.Agents[name]; exists {
		return cap.MaxComplexity
	}
	return 0
}

// BuildHandoffContext automatically builds a HandoffMessage with relevant state
// for transferring control from one agent to another.
func (hp *HandoffProtocol) BuildHandoffContext(from, to string, task string) *HandoffMessage {
	hp.mu.RLock()
	defer hp.mu.RUnlock()

	state := make(map[string]interface{})
	state["from_agent"] = from
	state["to_agent"] = to
	state["task"] = task
	state["history_length"] = len(hp.History)

	// Include the last handoff context if available.
	if len(hp.History) > 0 {
		last := hp.History[len(hp.History)-1]
		state["last_reason"] = last.Reason
		state["last_context"] = last.Context
	}

	context := fmt.Sprintf("Handoff from %s to %s for task: %s", from, to, task)

	return &HandoffMessage{
		FromAgent: from,
		ToAgent:   to,
		Reason:    fmt.Sprintf("Task delegation: %s", task),
		Context:   context,
		Task:      task,
		Priority:  1,
		State:     state,
		Timestamp: time.Now(),
	}
}

// EscalateToHuman creates a special handoff message that signals human
// intervention is needed.
func (hp *HandoffProtocol) EscalateToHuman(reason string) *HandoffMessage {
	hp.mu.RLock()
	activeAgent := hp.ActiveAgent
	hp.mu.RUnlock()

	state := make(map[string]interface{})
	state["escalation"] = true
	state["reason"] = reason

	return &HandoffMessage{
		FromAgent: activeAgent,
		ToAgent:   "human",
		Reason:    reason,
		Context:   fmt.Sprintf("Agent %s requires human intervention: %s", activeAgent, reason),
		Task:      "human-review",
		Priority:  10,
		State:     state,
		Timestamp: time.Now(),
	}
}

// FormatHandoffHistory returns a formatted string representation of the
// handoff history.
func (hp *HandoffProtocol) FormatHandoffHistory() string {
	hp.mu.RLock()
	defer hp.mu.RUnlock()

	if len(hp.History) == 0 {
		return "Handoff History:\n  (no handoffs recorded)"
	}

	var sb strings.Builder
	sb.WriteString("Handoff History:\n")
	for i, msg := range hp.History {
		sb.WriteString(fmt.Sprintf("  %d. %s → %s: %q\n", i+1, msg.FromAgent, msg.ToAgent, msg.Reason))
	}

	return strings.TrimRight(sb.String(), "\n")
}

// CanHandle checks whether a given agent can handle a task by matching
// task keywords against the agent's expertise.
func (hp *HandoffProtocol) CanHandle(agentName, task string) bool {
	hp.mu.RLock()
	defer hp.mu.RUnlock()

	cap, exists := hp.Agents[agentName]
	if !exists {
		return false
	}

	taskLower := strings.ToLower(task)
	for _, expertise := range cap.Expertise {
		if strings.Contains(taskLower, strings.ToLower(expertise)) {
			return true
		}
	}

	return false
}

// GetActiveAgent returns the currently active agent's capability, or nil if
// no agent is active.
func (hp *HandoffProtocol) GetActiveAgent() *AgentCapability {
	hp.mu.RLock()
	defer hp.mu.RUnlock()

	if hp.ActiveAgent == "" {
		return nil
	}

	return hp.Agents[hp.ActiveAgent]
}
