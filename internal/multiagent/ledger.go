package mission

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

// Fact represents a discovered piece of information during a mission.
type Fact struct {
	ID         string
	Content    string
	Source     string
	Confidence float64
	Timestamp  time.Time
}

// PlanItem represents a single unit of work in the orchestration plan.
type PlanItem struct {
	ID           string
	Description  string
	Status       string // "pending", "active", "done", "stalled"
	AssignedTo   string
	Dependencies []string
	Order        int
}

// AgentStatus tracks the state and performance of a registered agent.
type AgentStatus struct {
	Name        string
	Busy        bool
	TaskCount   int
	SuccessRate float64
	LastActive  time.Time
}

// Ledger maintains the shared state of facts, plan, and agents for orchestration.
type Ledger struct {
	Facts             []Fact
	Plan              []PlanItem
	Agents            map[string]*AgentStatus
	CurrentAssignment string
	StallCount        int
	mu                sync.RWMutex
}

// LedgerOrchestrator coordinates multi-agent work using a task ledger,
// dynamically reassigning work based on progress — inspired by MagenticOne.
type LedgerOrchestrator struct {
	Ledger            *Ledger
	MaxStalls         int
	ReassignThreshold time.Duration
	mu                sync.Mutex
}

// NewLedgerOrchestrator creates a new orchestrator with sensible defaults.
func NewLedgerOrchestrator() *LedgerOrchestrator {
	return &LedgerOrchestrator{
		Ledger: &Ledger{
			Facts:  make([]Fact, 0),
			Plan:   make([]PlanItem, 0),
			Agents: make(map[string]*AgentStatus),
		},
		MaxStalls:         3,
		ReassignThreshold: 2 * time.Minute,
	}
}

// AddFact records a new discovered fact into the ledger.
func (lo *LedgerOrchestrator) AddFact(content, source string, confidence float64) {
	lo.mu.Lock()
	defer lo.mu.Unlock()

	lo.Ledger.mu.Lock()
	defer lo.Ledger.mu.Unlock()

	fact := Fact{
		ID:         fmt.Sprintf("fact-%d", len(lo.Ledger.Facts)+1),
		Content:    content,
		Source:     source,
		Confidence: confidence,
		Timestamp:  time.Now(),
	}
	lo.Ledger.Facts = append(lo.Ledger.Facts, fact)
}

// AddPlanItem appends a new plan item with the given description and dependencies.
func (lo *LedgerOrchestrator) AddPlanItem(description string, deps []string) {
	lo.mu.Lock()
	defer lo.mu.Unlock()

	lo.Ledger.mu.Lock()
	defer lo.Ledger.mu.Unlock()

	item := PlanItem{
		ID:           fmt.Sprintf("plan-%d", len(lo.Ledger.Plan)+1),
		Description:  description,
		Status:       "pending",
		Dependencies: deps,
		Order:        len(lo.Ledger.Plan) + 1,
	}
	lo.Ledger.Plan = append(lo.Ledger.Plan, item)
}

// RegisterAgent adds a new agent to the orchestrator's pool.
func (lo *LedgerOrchestrator) RegisterAgent(name string) {
	lo.mu.Lock()
	defer lo.mu.Unlock()

	lo.Ledger.mu.Lock()
	defer lo.Ledger.mu.Unlock()

	lo.Ledger.Agents[name] = &AgentStatus{
		Name:        name,
		Busy:        false,
		TaskCount:   0,
		SuccessRate: 1.0,
		LastActive:  time.Now(),
	}
}

// AssignNext finds the next unblocked pending plan item and assigns it to the
// best available agent. Returns nil, "" if no assignment can be made.
func (lo *LedgerOrchestrator) AssignNext() (*PlanItem, string) {
	lo.mu.Lock()
	defer lo.mu.Unlock()

	lo.Ledger.mu.Lock()
	defer lo.Ledger.mu.Unlock()

	// Find next unblocked pending item
	var target *PlanItem
	for i := range lo.Ledger.Plan {
		item := &lo.Ledger.Plan[i]
		if item.Status != "pending" {
			continue
		}
		if lo.isBlocked(item) {
			continue
		}
		target = item
		break
	}
	if target == nil {
		return nil, ""
	}

	// Select best agent: prefer not busy, lowest task count, highest success rate
	var bestAgent *AgentStatus
	for _, agent := range lo.Ledger.Agents {
		if agent.Busy {
			continue
		}
		if bestAgent == nil {
			bestAgent = agent
			continue
		}
		if agent.TaskCount < bestAgent.TaskCount {
			bestAgent = agent
		} else if agent.TaskCount == bestAgent.TaskCount && agent.SuccessRate > bestAgent.SuccessRate {
			bestAgent = agent
		}
	}
	if bestAgent == nil {
		return nil, ""
	}

	// Assign the item
	target.Status = "active"
	target.AssignedTo = bestAgent.Name
	bestAgent.Busy = true
	bestAgent.LastActive = time.Now()
	lo.Ledger.CurrentAssignment = fmt.Sprintf("%s -> %s", target.ID, bestAgent.Name)

	return target, bestAgent.Name
}

// ReportProgress updates the status of a plan item and the agent's stats.
func (lo *LedgerOrchestrator) ReportProgress(agentName string, itemID string, status string) {
	lo.mu.Lock()
	defer lo.mu.Unlock()

	lo.Ledger.mu.Lock()
	defer lo.Ledger.mu.Unlock()

	for i := range lo.Ledger.Plan {
		item := &lo.Ledger.Plan[i]
		if item.ID != itemID {
			continue
		}
		item.Status = status

		agent, ok := lo.Ledger.Agents[agentName]
		if !ok {
			break
		}

		agent.LastActive = time.Now()

		if status == "done" {
			agent.Busy = false
			agent.TaskCount++
			// Maintain running success rate
			total := float64(agent.TaskCount)
			agent.SuccessRate = ((agent.SuccessRate * (total - 1)) + 1.0) / total
		} else if status == "stalled" {
			agent.Busy = false
			agent.TaskCount++
			total := float64(agent.TaskCount)
			agent.SuccessRate = ((agent.SuccessRate * (total - 1)) + 0.0) / total
		}
		break
	}
}

// DetectStall checks if any active item has exceeded the ReassignThreshold
// without progress. Returns true if a stall is detected.
func (lo *LedgerOrchestrator) DetectStall() bool {
	lo.mu.Lock()
	defer lo.mu.Unlock()

	lo.Ledger.mu.Lock()
	defer lo.Ledger.mu.Unlock()

	now := time.Now()
	for _, item := range lo.Ledger.Plan {
		if item.Status != "active" {
			continue
		}
		agent, ok := lo.Ledger.Agents[item.AssignedTo]
		if !ok {
			continue
		}
		if now.Sub(agent.LastActive) > lo.ReassignThreshold {
			lo.Ledger.StallCount++
			return true
		}
	}
	return false
}

// Reassign moves a stalled item to a different agent and records a fact.
// Returns the name of the new agent, or "" if no reassignment is possible.
func (lo *LedgerOrchestrator) Reassign(itemID string) string {
	lo.mu.Lock()
	defer lo.mu.Unlock()

	lo.Ledger.mu.Lock()
	defer lo.Ledger.mu.Unlock()

	var target *PlanItem
	for i := range lo.Ledger.Plan {
		if lo.Ledger.Plan[i].ID == itemID {
			target = &lo.Ledger.Plan[i]
			break
		}
	}
	if target == nil {
		return ""
	}

	previousAgent := target.AssignedTo

	// Mark previous agent as not busy
	if prev, ok := lo.Ledger.Agents[previousAgent]; ok {
		prev.Busy = false
	}

	// Find a different available agent
	var bestAgent *AgentStatus
	for _, agent := range lo.Ledger.Agents {
		if agent.Name == previousAgent {
			continue
		}
		if agent.Busy {
			continue
		}
		if bestAgent == nil {
			bestAgent = agent
			continue
		}
		if agent.TaskCount < bestAgent.TaskCount {
			bestAgent = agent
		} else if agent.TaskCount == bestAgent.TaskCount && agent.SuccessRate > bestAgent.SuccessRate {
			bestAgent = agent
		}
	}
	if bestAgent == nil {
		return ""
	}

	// Record stall fact
	stallFact := Fact{
		ID:         fmt.Sprintf("fact-%d", len(lo.Ledger.Facts)+1),
		Content:    fmt.Sprintf("Task %q stalled on %s, reassigned to %s", target.Description, previousAgent, bestAgent.Name),
		Source:     "orchestrator",
		Confidence: 1.0,
		Timestamp:  time.Now(),
	}
	lo.Ledger.Facts = append(lo.Ledger.Facts, stallFact)

	// Reassign
	target.Status = "active"
	target.AssignedTo = bestAgent.Name
	bestAgent.Busy = true
	bestAgent.LastActive = time.Now()
	lo.Ledger.CurrentAssignment = fmt.Sprintf("%s -> %s", target.ID, bestAgent.Name)

	return bestAgent.Name
}

// UpdatePlan replaces or merges plan items dynamically. Items with matching IDs
// are updated; new items are appended.
func (lo *LedgerOrchestrator) UpdatePlan(newItems []PlanItem) {
	lo.mu.Lock()
	defer lo.mu.Unlock()

	lo.Ledger.mu.Lock()
	defer lo.Ledger.mu.Unlock()

	existing := make(map[string]int)
	for i, item := range lo.Ledger.Plan {
		existing[item.ID] = i
	}

	for _, newItem := range newItems {
		if idx, ok := existing[newItem.ID]; ok {
			// Preserve assignment/status if not explicitly set
			if newItem.Status == "" {
				newItem.Status = lo.Ledger.Plan[idx].Status
			}
			if newItem.AssignedTo == "" {
				newItem.AssignedTo = lo.Ledger.Plan[idx].AssignedTo
			}
			lo.Ledger.Plan[idx] = newItem
		} else {
			if newItem.Status == "" {
				newItem.Status = "pending"
			}
			if newItem.ID == "" {
				newItem.ID = fmt.Sprintf("plan-%d", len(lo.Ledger.Plan)+1)
			}
			if newItem.Order == 0 {
				newItem.Order = len(lo.Ledger.Plan) + 1
			}
			lo.Ledger.Plan = append(lo.Ledger.Plan, newItem)
		}
	}
}

// FormatLedger returns a human-readable summary of the ledger state.
func (lo *LedgerOrchestrator) FormatLedger() string {
	lo.Ledger.mu.RLock()
	defer lo.Ledger.mu.RUnlock()

	var sb strings.Builder

	sb.WriteString("Ledger State:\n")
	sb.WriteString("═══════════════════════════\n")

	// Facts section
	sb.WriteString(fmt.Sprintf("Facts (%d):\n", len(lo.Ledger.Facts)))
	for _, fact := range lo.Ledger.Facts {
		sb.WriteString(fmt.Sprintf("  • %s (confidence: %.2g)\n", fact.Content, fact.Confidence))
	}

	sb.WriteString("\n")

	// Plan section
	sb.WriteString(fmt.Sprintf("Plan (%d items):\n", len(lo.Ledger.Plan)))
	for i, item := range lo.Ledger.Plan {
		line := fmt.Sprintf("  %d. [%s] %s", i+1, item.Status, item.Description)
		if item.AssignedTo != "" {
			line += fmt.Sprintf(" (%s)", item.AssignedTo)
		}
		sb.WriteString(line + "\n")
	}

	sb.WriteString("\n")

	// Agents section
	sb.WriteString("Agents:\n")
	for _, agent := range lo.Ledger.Agents {
		var statusStr string
		if agent.Busy {
			statusStr = fmt.Sprintf("busy (1 active, %.0f%% success)", agent.SuccessRate*100)
		} else {
			statusStr = fmt.Sprintf("idle (%d tasks done, %.0f%% success)", agent.TaskCount, agent.SuccessRate*100)
		}
		sb.WriteString(fmt.Sprintf("  %s: %s\n", agent.Name, statusStr))
	}

	return sb.String()
}

// isBlocked checks whether a plan item's dependencies are all done.
// Must be called with Ledger.mu held.
func (lo *LedgerOrchestrator) isBlocked(item *PlanItem) bool {
	if len(item.Dependencies) == 0 {
		return false
	}
	doneSet := make(map[string]bool)
	for _, p := range lo.Ledger.Plan {
		if p.Status == "done" {
			doneSet[p.ID] = true
		}
	}
	for _, dep := range item.Dependencies {
		if !doneSet[dep] {
			return true
		}
	}
	return false
}
