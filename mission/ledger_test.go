package mission

import (
	"strings"
	"testing"
	"time"
)

func TestNewLedgerOrchestrator(t *testing.T) {
	lo := NewLedgerOrchestrator()
	if lo == nil {
		t.Fatal("expected non-nil orchestrator")
	}
	if lo.MaxStalls != 3 {
		t.Errorf("expected MaxStalls=3, got %d", lo.MaxStalls)
	}
	if lo.ReassignThreshold != 2*time.Minute {
		t.Errorf("expected ReassignThreshold=2m, got %v", lo.ReassignThreshold)
	}
	if lo.Ledger == nil {
		t.Fatal("expected non-nil ledger")
	}
	if len(lo.Ledger.Facts) != 0 {
		t.Errorf("expected empty facts, got %d", len(lo.Ledger.Facts))
	}
	if len(lo.Ledger.Plan) != 0 {
		t.Errorf("expected empty plan, got %d", len(lo.Ledger.Plan))
	}
	if len(lo.Ledger.Agents) != 0 {
		t.Errorf("expected empty agents, got %d", len(lo.Ledger.Agents))
	}
}

func TestAddFact(t *testing.T) {
	lo := NewLedgerOrchestrator()
	lo.AddFact("Auth uses JWT with RS256", "code-analysis", 0.95)
	lo.AddFact("Rate limit is 100 req/min", "docs", 0.8)

	if len(lo.Ledger.Facts) != 2 {
		t.Fatalf("expected 2 facts, got %d", len(lo.Ledger.Facts))
	}

	f := lo.Ledger.Facts[0]
	if f.ID != "fact-1" {
		t.Errorf("expected ID=fact-1, got %s", f.ID)
	}
	if f.Content != "Auth uses JWT with RS256" {
		t.Errorf("unexpected content: %s", f.Content)
	}
	if f.Source != "code-analysis" {
		t.Errorf("unexpected source: %s", f.Source)
	}
	if f.Confidence != 0.95 {
		t.Errorf("expected confidence=0.95, got %f", f.Confidence)
	}
	if f.Timestamp.IsZero() {
		t.Error("expected non-zero timestamp")
	}
}

func TestAddPlanItem(t *testing.T) {
	lo := NewLedgerOrchestrator()
	lo.AddPlanItem("Implement token validation", nil)
	lo.AddPlanItem("Add rate limiting middleware", []string{"plan-1"})

	if len(lo.Ledger.Plan) != 2 {
		t.Fatalf("expected 2 plan items, got %d", len(lo.Ledger.Plan))
	}

	p1 := lo.Ledger.Plan[0]
	if p1.ID != "plan-1" {
		t.Errorf("expected ID=plan-1, got %s", p1.ID)
	}
	if p1.Status != "pending" {
		t.Errorf("expected status=pending, got %s", p1.Status)
	}
	if p1.Order != 1 {
		t.Errorf("expected order=1, got %d", p1.Order)
	}

	p2 := lo.Ledger.Plan[1]
	if len(p2.Dependencies) != 1 || p2.Dependencies[0] != "plan-1" {
		t.Errorf("unexpected dependencies: %v", p2.Dependencies)
	}
}

func TestLedgerRegisterAgent(t *testing.T) {
	lo := NewLedgerOrchestrator()
	lo.RegisterAgent("agent-1")
	lo.RegisterAgent("agent-2")

	if len(lo.Ledger.Agents) != 2 {
		t.Fatalf("expected 2 agents, got %d", len(lo.Ledger.Agents))
	}

	a := lo.Ledger.Agents["agent-1"]
	if a.Name != "agent-1" {
		t.Errorf("expected name=agent-1, got %s", a.Name)
	}
	if a.Busy {
		t.Error("expected agent not busy")
	}
	if a.TaskCount != 0 {
		t.Errorf("expected task count=0, got %d", a.TaskCount)
	}
	if a.SuccessRate != 1.0 {
		t.Errorf("expected success rate=1.0, got %f", a.SuccessRate)
	}
}

func TestAssignNext(t *testing.T) {
	lo := NewLedgerOrchestrator()
	lo.RegisterAgent("agent-1")
	lo.RegisterAgent("agent-2")
	lo.AddPlanItem("Task A", nil)
	lo.AddPlanItem("Task B", nil)

	item, agent := lo.AssignNext()
	if item == nil {
		t.Fatal("expected assignment, got nil")
	}
	if item.Description != "Task A" {
		t.Errorf("expected Task A, got %s", item.Description)
	}
	if agent == "" {
		t.Fatal("expected agent name, got empty")
	}
	if item.Status != "active" {
		t.Errorf("expected status=active, got %s", item.Status)
	}
	if item.AssignedTo != agent {
		t.Errorf("expected AssignedTo=%s, got %s", agent, item.AssignedTo)
	}
}

func TestAssignNextRespectsBlockedDeps(t *testing.T) {
	lo := NewLedgerOrchestrator()
	lo.RegisterAgent("agent-1")
	lo.AddPlanItem("Task A", nil)
	lo.AddPlanItem("Task B", []string{"plan-1"})

	// Assign Task A
	item, _ := lo.AssignNext()
	if item.Description != "Task A" {
		t.Fatalf("expected Task A first, got %s", item.Description)
	}

	// Try to assign again — Task B is blocked, agent-1 is busy
	item2, agent2 := lo.AssignNext()
	if item2 != nil {
		t.Errorf("expected nil (Task B is blocked), got %s assigned to %s", item2.Description, agent2)
	}
}

func TestAssignNextNoAvailableAgent(t *testing.T) {
	lo := NewLedgerOrchestrator()
	lo.AddPlanItem("Task A", nil)

	// No agents registered
	item, agent := lo.AssignNext()
	if item != nil || agent != "" {
		t.Errorf("expected nil assignment with no agents, got %v, %s", item, agent)
	}
}

func TestReportProgressDone(t *testing.T) {
	lo := NewLedgerOrchestrator()
	lo.RegisterAgent("agent-1")
	lo.AddPlanItem("Task A", nil)

	item, agent := lo.AssignNext()
	lo.ReportProgress(agent, item.ID, "done")

	// Verify item status updated
	lo.Ledger.mu.RLock()
	defer lo.Ledger.mu.RUnlock()

	if lo.Ledger.Plan[0].Status != "done" {
		t.Errorf("expected status=done, got %s", lo.Ledger.Plan[0].Status)
	}

	a := lo.Ledger.Agents["agent-1"]
	if a.Busy {
		t.Error("expected agent not busy after done")
	}
	if a.TaskCount != 1 {
		t.Errorf("expected task count=1, got %d", a.TaskCount)
	}
}

func TestReportProgressStalled(t *testing.T) {
	lo := NewLedgerOrchestrator()
	lo.RegisterAgent("agent-1")
	lo.AddPlanItem("Task A", nil)

	item, agent := lo.AssignNext()
	lo.ReportProgress(agent, item.ID, "stalled")

	lo.Ledger.mu.RLock()
	defer lo.Ledger.mu.RUnlock()

	if lo.Ledger.Plan[0].Status != "stalled" {
		t.Errorf("expected status=stalled, got %s", lo.Ledger.Plan[0].Status)
	}

	a := lo.Ledger.Agents["agent-1"]
	if a.Busy {
		t.Error("expected agent not busy after stall")
	}
	if a.SuccessRate != 0.0 {
		t.Errorf("expected success rate=0.0 after stall, got %f", a.SuccessRate)
	}
}

func TestDetectStall(t *testing.T) {
	lo := NewLedgerOrchestrator()
	lo.ReassignThreshold = 50 * time.Millisecond
	lo.RegisterAgent("agent-1")
	lo.AddPlanItem("Task A", nil)

	lo.AssignNext()

	// Immediately check — no stall yet
	if lo.DetectStall() {
		t.Error("expected no stall immediately after assignment")
	}

	// Wait past threshold
	time.Sleep(60 * time.Millisecond)

	if !lo.DetectStall() {
		t.Error("expected stall after threshold exceeded")
	}
	if lo.Ledger.StallCount != 1 {
		t.Errorf("expected StallCount=1, got %d", lo.Ledger.StallCount)
	}
}

func TestReassign(t *testing.T) {
	lo := NewLedgerOrchestrator()
	lo.RegisterAgent("agent-1")
	lo.RegisterAgent("agent-2")
	lo.AddPlanItem("Task A", nil)

	item, firstAgent := lo.AssignNext()

	newAgent := lo.Reassign(item.ID)
	if newAgent == "" {
		t.Fatal("expected reassignment to succeed")
	}
	if newAgent == firstAgent {
		t.Errorf("expected different agent, got same: %s", newAgent)
	}

	// Check stall fact was recorded
	lo.Ledger.mu.RLock()
	defer lo.Ledger.mu.RUnlock()

	found := false
	for _, f := range lo.Ledger.Facts {
		if strings.Contains(f.Content, "stalled") && strings.Contains(f.Content, "reassigned") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected stall fact to be recorded")
	}
}

func TestReassignNoAlternativeAgent(t *testing.T) {
	lo := NewLedgerOrchestrator()
	lo.RegisterAgent("agent-1")
	lo.AddPlanItem("Task A", nil)

	item, _ := lo.AssignNext()

	newAgent := lo.Reassign(item.ID)
	if newAgent != "" {
		t.Errorf("expected no reassignment with single agent, got %s", newAgent)
	}
}

func TestReassignInvalidItem(t *testing.T) {
	lo := NewLedgerOrchestrator()
	lo.RegisterAgent("agent-1")

	result := lo.Reassign("nonexistent")
	if result != "" {
		t.Errorf("expected empty string for invalid item, got %s", result)
	}
}

func TestUpdatePlanMerge(t *testing.T) {
	lo := NewLedgerOrchestrator()
	lo.AddPlanItem("Task A", nil)
	lo.AddPlanItem("Task B", nil)

	// Update existing item and add new one
	lo.UpdatePlan([]PlanItem{
		{ID: "plan-1", Description: "Task A (revised)", Status: "done"},
		{Description: "Task C"},
	})

	lo.Ledger.mu.RLock()
	defer lo.Ledger.mu.RUnlock()

	if len(lo.Ledger.Plan) != 3 {
		t.Fatalf("expected 3 plan items, got %d", len(lo.Ledger.Plan))
	}
	if lo.Ledger.Plan[0].Description != "Task A (revised)" {
		t.Errorf("expected revised description, got %s", lo.Ledger.Plan[0].Description)
	}
	if lo.Ledger.Plan[0].Status != "done" {
		t.Errorf("expected done status, got %s", lo.Ledger.Plan[0].Status)
	}
	if lo.Ledger.Plan[2].Description != "Task C" {
		t.Errorf("expected Task C, got %s", lo.Ledger.Plan[2].Description)
	}
	if lo.Ledger.Plan[2].Status != "pending" {
		t.Errorf("expected pending status for new item, got %s", lo.Ledger.Plan[2].Status)
	}
}

func TestUpdatePlanPreservesAssignment(t *testing.T) {
	lo := NewLedgerOrchestrator()
	lo.RegisterAgent("agent-1")
	lo.AddPlanItem("Task A", nil)
	lo.AssignNext()

	// Update description without changing assignment
	lo.UpdatePlan([]PlanItem{
		{ID: "plan-1", Description: "Task A (updated)"},
	})

	lo.Ledger.mu.RLock()
	defer lo.Ledger.mu.RUnlock()

	if lo.Ledger.Plan[0].AssignedTo != "agent-1" {
		t.Errorf("expected assignment preserved, got %s", lo.Ledger.Plan[0].AssignedTo)
	}
	if lo.Ledger.Plan[0].Status != "active" {
		t.Errorf("expected status preserved, got %s", lo.Ledger.Plan[0].Status)
	}
}

func TestFormatLedger(t *testing.T) {
	lo := NewLedgerOrchestrator()
	lo.AddFact("Auth uses JWT with RS256", "code-analysis", 0.95)
	lo.AddFact("Rate limit is 100 req/min", "docs", 0.8)
	lo.RegisterAgent("agent-1")
	lo.RegisterAgent("agent-2")
	lo.AddPlanItem("Implement token validation", nil)
	lo.AddPlanItem("Add rate limiting middleware", nil)
	lo.AddPlanItem("Write integration tests", []string{"plan-1", "plan-2"})

	// Complete first task
	item, agent := lo.AssignNext()
	lo.ReportProgress(agent, item.ID, "done")

	// Assign second task
	lo.AssignNext()

	output := lo.FormatLedger()

	if !strings.Contains(output, "Ledger State:") {
		t.Error("expected header in output")
	}
	if !strings.Contains(output, "═══════════════════════════") {
		t.Error("expected separator in output")
	}
	if !strings.Contains(output, "Facts (2):") {
		t.Error("expected facts count in output")
	}
	if !strings.Contains(output, "Auth uses JWT with RS256") {
		t.Error("expected fact content in output")
	}
	if !strings.Contains(output, "confidence: 0.95") {
		t.Error("expected confidence in output")
	}
	if !strings.Contains(output, "Plan (3 items):") {
		t.Error("expected plan count in output")
	}
	if !strings.Contains(output, "[done]") {
		t.Error("expected done status in output")
	}
	if !strings.Contains(output, "[active]") {
		t.Error("expected active status in output")
	}
	if !strings.Contains(output, "[pending]") {
		t.Error("expected pending status in output")
	}
	if !strings.Contains(output, "Agents:") {
		t.Error("expected agents section in output")
	}
}

func TestAssignNextSelectsBestAgent(t *testing.T) {
	lo := NewLedgerOrchestrator()
	lo.RegisterAgent("agent-1")
	lo.RegisterAgent("agent-2")

	// Give agent-1 some completed tasks to increase task count
	lo.Ledger.mu.Lock()
	lo.Ledger.Agents["agent-1"].TaskCount = 5
	lo.Ledger.Agents["agent-1"].SuccessRate = 0.8
	lo.Ledger.Agents["agent-2"].TaskCount = 2
	lo.Ledger.Agents["agent-2"].SuccessRate = 0.9
	lo.Ledger.mu.Unlock()

	lo.AddPlanItem("Task A", nil)

	_, agent := lo.AssignNext()
	// agent-2 should be selected: lower task count
	if agent != "agent-2" {
		t.Errorf("expected agent-2 (lower task count), got %s", agent)
	}
}

func TestFullWorkflow(t *testing.T) {
	lo := NewLedgerOrchestrator()
	lo.ReassignThreshold = 50 * time.Millisecond

	// Setup
	lo.RegisterAgent("coder")
	lo.RegisterAgent("reviewer")
	lo.RegisterAgent("tester")

	lo.AddFact("Project uses Go 1.22", "go.mod", 1.0)
	lo.AddFact("Need 80% test coverage", "requirements", 0.9)

	lo.AddPlanItem("Implement feature X", nil)
	lo.AddPlanItem("Review implementation", []string{"plan-1"})
	lo.AddPlanItem("Write tests", []string{"plan-1"})

	// Assign and complete first task
	item1, agent1 := lo.AssignNext()
	if item1 == nil {
		t.Fatal("expected assignment for Task 1")
	}
	lo.ReportProgress(agent1, item1.ID, "done")

	// Now plan-2 and plan-3 should be unblocked
	item2, agent2 := lo.AssignNext()
	if item2 == nil {
		t.Fatal("expected assignment for unblocked task")
	}

	item3, agent3 := lo.AssignNext()
	if item3 == nil {
		t.Fatal("expected assignment for second unblocked task")
	}

	// Complete both
	lo.ReportProgress(agent2, item2.ID, "done")
	lo.ReportProgress(agent3, item3.ID, "done")

	// All should be done
	lo.Ledger.mu.RLock()
	allDone := true
	for _, p := range lo.Ledger.Plan {
		if p.Status != "done" {
			allDone = false
			break
		}
	}
	lo.Ledger.mu.RUnlock()

	if !allDone {
		t.Error("expected all plan items to be done")
	}

	// Verify format output includes everything
	output := lo.FormatLedger()
	if !strings.Contains(output, "Facts (2):") {
		t.Error("expected 2 facts in format output")
	}
	if !strings.Contains(output, "Plan (3 items):") {
		t.Error("expected 3 plan items in format output")
	}
}

func TestConcurrentAccess(t *testing.T) {
	lo := NewLedgerOrchestrator()
	lo.RegisterAgent("agent-1")
	lo.RegisterAgent("agent-2")
	lo.RegisterAgent("agent-3")

	for i := 0; i < 10; i++ {
		lo.AddPlanItem("Task", nil)
	}

	done := make(chan struct{})

	// Concurrent assigns
	go func() {
		for i := 0; i < 5; i++ {
			lo.AssignNext()
		}
		done <- struct{}{}
	}()

	// Concurrent fact adds
	go func() {
		for i := 0; i < 5; i++ {
			lo.AddFact("concurrent fact", "test", 0.5)
		}
		done <- struct{}{}
	}()

	// Concurrent format
	go func() {
		for i := 0; i < 5; i++ {
			lo.FormatLedger()
		}
		done <- struct{}{}
	}()

	// Wait for all goroutines
	for i := 0; i < 3; i++ {
		<-done
	}
}
