package mission

import (
	"strings"
	"sync"
	"testing"
	"time"
)

func TestNewHandoffProtocol(t *testing.T) {
	hp := NewHandoffProtocol()
	if hp == nil {
		t.Fatal("NewHandoffProtocol returned nil")
	}
	if hp.Agents == nil {
		t.Error("Agents map should be initialized")
	}
	if hp.ActiveAgent != "" {
		t.Error("ActiveAgent should be empty initially")
	}
	if hp.History == nil {
		t.Error("History should be initialized")
	}
	if len(hp.History) != 0 {
		t.Error("History should be empty initially")
	}
}

func TestRegisterAgent(t *testing.T) {
	hp := NewHandoffProtocol()

	agent := AgentCapability{
		Name:          "security-reviewer",
		Expertise:     []string{"security", "auth", "vulnerability"},
		Tools:         []string{"static-analysis", "dependency-check"},
		MaxComplexity: 8,
		Model:         "claude-opus",
	}

	hp.RegisterAgent(agent)

	if len(hp.Agents) != 1 {
		t.Fatalf("expected 1 agent, got %d", len(hp.Agents))
	}

	registered := hp.Agents["security-reviewer"]
	if registered == nil {
		t.Fatal("agent not found after registration")
	}
	if registered.Name != "security-reviewer" {
		t.Errorf("expected name 'security-reviewer', got %q", registered.Name)
	}
	if registered.MaxComplexity != 8 {
		t.Errorf("expected MaxComplexity 8, got %d", registered.MaxComplexity)
	}

	// First registered agent becomes active.
	if hp.ActiveAgent != "security-reviewer" {
		t.Errorf("expected active agent 'security-reviewer', got %q", hp.ActiveAgent)
	}
}

func TestRegisterAgent_FirstBecomesActive(t *testing.T) {
	hp := NewHandoffProtocol()

	hp.RegisterAgent(AgentCapability{Name: "first", Expertise: []string{"general"}})
	hp.RegisterAgent(AgentCapability{Name: "second", Expertise: []string{"specific"}})

	if hp.ActiveAgent != "first" {
		t.Errorf("expected first registered agent to be active, got %q", hp.ActiveAgent)
	}
}

func TestHandoff_Success(t *testing.T) {
	hp := NewHandoffProtocol()
	hp.RegisterAgent(AgentCapability{Name: "default", Expertise: []string{"general"}})
	hp.RegisterAgent(AgentCapability{Name: "security-reviewer", Expertise: []string{"security"}})

	msg := HandoffMessage{
		FromAgent: "default",
		ToAgent:   "security-reviewer",
		Reason:    "Security audit needed for auth module",
		Context:   "Auth module has new endpoints",
		Task:      "audit-auth",
		Priority:  5,
		State:     map[string]interface{}{"module": "auth"},
	}

	err := hp.Handoff(msg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if hp.ActiveAgent != "security-reviewer" {
		t.Errorf("expected active agent 'security-reviewer', got %q", hp.ActiveAgent)
	}

	if len(hp.History) != 1 {
		t.Fatalf("expected 1 history entry, got %d", len(hp.History))
	}

	recorded := hp.History[0]
	if recorded.FromAgent != "default" {
		t.Errorf("expected FromAgent 'default', got %q", recorded.FromAgent)
	}
	if recorded.ToAgent != "security-reviewer" {
		t.Errorf("expected ToAgent 'security-reviewer', got %q", recorded.ToAgent)
	}
	if recorded.Timestamp.IsZero() {
		t.Error("timestamp should be set")
	}
}

func TestHandoff_TargetNotRegistered(t *testing.T) {
	hp := NewHandoffProtocol()
	hp.RegisterAgent(AgentCapability{Name: "default", Expertise: []string{"general"}})

	msg := HandoffMessage{
		FromAgent: "default",
		ToAgent:   "nonexistent",
		Reason:    "test",
		Task:      "test-task",
	}

	err := hp.Handoff(msg)
	if err == nil {
		t.Fatal("expected error for nonexistent target agent")
	}
	if !strings.Contains(err.Error(), "nonexistent") {
		t.Errorf("error should mention agent name, got: %v", err)
	}

	// Active agent should not change on failure.
	if hp.ActiveAgent != "default" {
		t.Errorf("active agent should remain 'default', got %q", hp.ActiveAgent)
	}
}

func TestHandoff_SetsTimestamp(t *testing.T) {
	hp := NewHandoffProtocol()
	hp.RegisterAgent(AgentCapability{Name: "a", Expertise: []string{"x"}})
	hp.RegisterAgent(AgentCapability{Name: "b", Expertise: []string{"y"}})

	before := time.Now()
	err := hp.Handoff(HandoffMessage{
		FromAgent: "a",
		ToAgent:   "b",
		Reason:    "test",
		Task:      "task",
	})
	after := time.Now()

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	ts := hp.History[0].Timestamp
	if ts.Before(before) || ts.After(after) {
		t.Error("timestamp should be set to approximately now")
	}
}

func TestSelectBestAgent(t *testing.T) {
	hp := NewHandoffProtocol()
	hp.RegisterAgent(AgentCapability{
		Name:          "default",
		Expertise:     []string{"general", "coding"},
		MaxComplexity: 5,
	})
	hp.RegisterAgent(AgentCapability{
		Name:          "security-reviewer",
		Expertise:     []string{"security", "auth", "vulnerability"},
		MaxComplexity: 8,
	})
	hp.RegisterAgent(AgentCapability{
		Name:          "test-writer",
		Expertise:     []string{"test", "testing", "coverage"},
		MaxComplexity: 6,
	})

	tests := []struct {
		task     string
		expected string
	}{
		{"Review security of the auth module", "security-reviewer"},
		{"Write tests for the new feature", "test-writer"},
		{"Fix the security vulnerability in testing", "security-reviewer"}, // security has more matches
	}

	for _, tt := range tests {
		got := hp.SelectBestAgent(tt.task)
		if got != tt.expected {
			t.Errorf("SelectBestAgent(%q) = %q, want %q", tt.task, got, tt.expected)
		}
	}
}

func TestSelectBestAgent_NoMatch(t *testing.T) {
	hp := NewHandoffProtocol()
	hp.RegisterAgent(AgentCapability{
		Name:      "default",
		Expertise: []string{"coding"},
	})

	// Task has no matching keywords - should return active agent.
	got := hp.SelectBestAgent("completely unrelated topic xyz")
	if got != "default" {
		t.Errorf("expected fallback to active agent 'default', got %q", got)
	}
}

func TestBuildHandoffContext(t *testing.T) {
	hp := NewHandoffProtocol()
	hp.RegisterAgent(AgentCapability{Name: "default", Expertise: []string{"general"}})
	hp.RegisterAgent(AgentCapability{Name: "reviewer", Expertise: []string{"review"}})

	msg := hp.BuildHandoffContext("default", "reviewer", "Review the PR")

	if msg == nil {
		t.Fatal("BuildHandoffContext returned nil")
	}
	if msg.FromAgent != "default" {
		t.Errorf("expected FromAgent 'default', got %q", msg.FromAgent)
	}
	if msg.ToAgent != "reviewer" {
		t.Errorf("expected ToAgent 'reviewer', got %q", msg.ToAgent)
	}
	if msg.Task != "Review the PR" {
		t.Errorf("expected Task 'Review the PR', got %q", msg.Task)
	}
	if msg.State == nil {
		t.Fatal("State should not be nil")
	}
	if msg.State["from_agent"] != "default" {
		t.Errorf("state from_agent mismatch")
	}
	if msg.State["to_agent"] != "reviewer" {
		t.Errorf("state to_agent mismatch")
	}
	if msg.Timestamp.IsZero() {
		t.Error("timestamp should be set")
	}
	if !strings.Contains(msg.Context, "default") || !strings.Contains(msg.Context, "reviewer") {
		t.Error("context should mention both agents")
	}
}

func TestBuildHandoffContext_WithHistory(t *testing.T) {
	hp := NewHandoffProtocol()
	hp.RegisterAgent(AgentCapability{Name: "a", Expertise: []string{"x"}})
	hp.RegisterAgent(AgentCapability{Name: "b", Expertise: []string{"y"}})

	// Add a history entry.
	hp.Handoff(HandoffMessage{
		FromAgent: "a",
		ToAgent:   "b",
		Reason:    "previous reason",
		Context:   "previous context",
		Task:      "old task",
	})

	msg := hp.BuildHandoffContext("b", "a", "new task")
	if msg.State["last_reason"] != "previous reason" {
		t.Errorf("expected last_reason in state, got %v", msg.State["last_reason"])
	}
	if msg.State["last_context"] != "previous context" {
		t.Errorf("expected last_context in state, got %v", msg.State["last_context"])
	}
}

func TestEscalateToHuman(t *testing.T) {
	hp := NewHandoffProtocol()
	hp.RegisterAgent(AgentCapability{Name: "default", Expertise: []string{"general"}})

	msg := hp.EscalateToHuman("Cannot resolve merge conflict automatically")

	if msg == nil {
		t.Fatal("EscalateToHuman returned nil")
	}
	if msg.FromAgent != "default" {
		t.Errorf("expected FromAgent 'default', got %q", msg.FromAgent)
	}
	if msg.ToAgent != "human" {
		t.Errorf("expected ToAgent 'human', got %q", msg.ToAgent)
	}
	if msg.Reason != "Cannot resolve merge conflict automatically" {
		t.Errorf("unexpected reason: %q", msg.Reason)
	}
	if msg.Priority != 10 {
		t.Errorf("expected Priority 10, got %d", msg.Priority)
	}
	if msg.State["escalation"] != true {
		t.Error("state should have escalation=true")
	}
	if msg.Task != "human-review" {
		t.Errorf("expected task 'human-review', got %q", msg.Task)
	}
	if msg.Timestamp.IsZero() {
		t.Error("timestamp should be set")
	}
}

func TestFormatHandoffHistory_Empty(t *testing.T) {
	hp := NewHandoffProtocol()

	result := hp.FormatHandoffHistory()
	if !strings.Contains(result, "Handoff History:") {
		t.Error("should contain header")
	}
	if !strings.Contains(result, "no handoffs recorded") {
		t.Error("should indicate no handoffs")
	}
}

func TestFormatHandoffHistory_WithEntries(t *testing.T) {
	hp := NewHandoffProtocol()
	hp.RegisterAgent(AgentCapability{Name: "default", Expertise: []string{"general"}})
	hp.RegisterAgent(AgentCapability{Name: "security-reviewer", Expertise: []string{"security"}})
	hp.RegisterAgent(AgentCapability{Name: "test-writer", Expertise: []string{"test"}})

	hp.Handoff(HandoffMessage{
		FromAgent: "default",
		ToAgent:   "security-reviewer",
		Reason:    "Security audit needed for auth module",
		Task:      "audit",
	})
	hp.Handoff(HandoffMessage{
		FromAgent: "security-reviewer",
		ToAgent:   "default",
		Reason:    "Audit complete, 2 issues found",
		Task:      "report",
	})
	hp.Handoff(HandoffMessage{
		FromAgent: "default",
		ToAgent:   "test-writer",
		Reason:    "Write tests for the fixes",
		Task:      "test",
	})

	result := hp.FormatHandoffHistory()

	if !strings.Contains(result, "Handoff History:") {
		t.Error("should contain header")
	}
	if !strings.Contains(result, "1. default → security-reviewer:") {
		t.Errorf("missing first entry in: %s", result)
	}
	if !strings.Contains(result, "2. security-reviewer → default:") {
		t.Errorf("missing second entry in: %s", result)
	}
	if !strings.Contains(result, "3. default → test-writer:") {
		t.Errorf("missing third entry in: %s", result)
	}
	if !strings.Contains(result, "Security audit needed for auth module") {
		t.Errorf("missing reason in: %s", result)
	}
}

func TestCanHandle(t *testing.T) {
	hp := NewHandoffProtocol()
	hp.RegisterAgent(AgentCapability{
		Name:      "security-reviewer",
		Expertise: []string{"security", "auth", "vulnerability"},
	})
	hp.RegisterAgent(AgentCapability{
		Name:      "test-writer",
		Expertise: []string{"test", "coverage"},
	})

	tests := []struct {
		agent    string
		task     string
		expected bool
	}{
		{"security-reviewer", "Check security of the login page", true},
		{"security-reviewer", "Write unit tests", false},
		{"test-writer", "Improve test coverage", true},
		{"test-writer", "Deploy to production", false},
		{"nonexistent", "any task", false},
	}

	for _, tt := range tests {
		got := hp.CanHandle(tt.agent, tt.task)
		if got != tt.expected {
			t.Errorf("CanHandle(%q, %q) = %v, want %v", tt.agent, tt.task, got, tt.expected)
		}
	}
}

func TestCanHandle_CaseInsensitive(t *testing.T) {
	hp := NewHandoffProtocol()
	hp.RegisterAgent(AgentCapability{
		Name:      "reviewer",
		Expertise: []string{"Security"},
	})

	if !hp.CanHandle("reviewer", "check SECURITY issues") {
		t.Error("CanHandle should be case insensitive")
	}
}

func TestGetActiveAgent(t *testing.T) {
	hp := NewHandoffProtocol()

	// No agents registered.
	if hp.GetActiveAgent() != nil {
		t.Error("should return nil when no agents registered")
	}

	hp.RegisterAgent(AgentCapability{
		Name:          "default",
		Expertise:     []string{"general"},
		MaxComplexity: 5,
		Model:         "claude-sonnet",
	})

	active := hp.GetActiveAgent()
	if active == nil {
		t.Fatal("should return active agent")
	}
	if active.Name != "default" {
		t.Errorf("expected 'default', got %q", active.Name)
	}
	if active.Model != "claude-sonnet" {
		t.Errorf("expected model 'claude-sonnet', got %q", active.Model)
	}
}

func TestGetActiveAgent_AfterHandoff(t *testing.T) {
	hp := NewHandoffProtocol()
	hp.RegisterAgent(AgentCapability{Name: "a", Expertise: []string{"x"}})
	hp.RegisterAgent(AgentCapability{Name: "b", Expertise: []string{"y"}, Model: "opus"})

	hp.Handoff(HandoffMessage{FromAgent: "a", ToAgent: "b", Reason: "test", Task: "t"})

	active := hp.GetActiveAgent()
	if active == nil {
		t.Fatal("should return active agent after handoff")
	}
	if active.Name != "b" {
		t.Errorf("expected 'b', got %q", active.Name)
	}
}

func TestConcurrentHandoffs(t *testing.T) {
	hp := NewHandoffProtocol()
	hp.RegisterAgent(AgentCapability{Name: "a", Expertise: []string{"x"}})
	hp.RegisterAgent(AgentCapability{Name: "b", Expertise: []string{"y"}})

	var wg sync.WaitGroup
	iterations := 100

	wg.Add(iterations * 2)
	for i := 0; i < iterations; i++ {
		go func() {
			defer wg.Done()
			hp.Handoff(HandoffMessage{FromAgent: "a", ToAgent: "b", Reason: "test", Task: "t"})
		}()
		go func() {
			defer wg.Done()
			hp.Handoff(HandoffMessage{FromAgent: "b", ToAgent: "a", Reason: "test", Task: "t"})
		}()
	}

	wg.Wait()

	if len(hp.History) != iterations*2 {
		t.Errorf("expected %d history entries, got %d", iterations*2, len(hp.History))
	}
}

func TestConcurrentReads(t *testing.T) {
	hp := NewHandoffProtocol()
	hp.RegisterAgent(AgentCapability{Name: "a", Expertise: []string{"security", "auth"}})
	hp.RegisterAgent(AgentCapability{Name: "b", Expertise: []string{"test", "coverage"}})

	var wg sync.WaitGroup
	wg.Add(4)

	go func() {
		defer wg.Done()
		hp.SelectBestAgent("security review")
	}()
	go func() {
		defer wg.Done()
		hp.CanHandle("a", "security check")
	}()
	go func() {
		defer wg.Done()
		hp.GetActiveAgent()
	}()
	go func() {
		defer wg.Done()
		hp.FormatHandoffHistory()
	}()

	wg.Wait()
}
