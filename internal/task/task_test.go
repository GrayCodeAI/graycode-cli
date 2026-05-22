package task

import (
	"testing"
	"time"
)

func TestCreateAndGet(t *testing.T) {
	s := NewStore(nil)
	task, err := s.Create("t1", "Test Task", "desc", nil, 10)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if task.ID != "t1" || task.Name != "Test Task" {
		t.Errorf("task = %+v", task)
	}
	if task.State != StatePending {
		t.Errorf("State = %q, want pending", task.State)
	}
	if task.MaxWakeups != 10 {
		t.Errorf("MaxWakeups = %d, want 10", task.MaxWakeups)
	}

	got := s.Get("t1")
	if got != task {
		t.Error("Get should return the same task pointer")
	}
}

func TestCreateDefaultMaxWakeups(t *testing.T) {
	s := NewStore(nil)
	task, _ := s.Create("t1", "T", "d", nil, 0)
	if task.MaxWakeups != 50 {
		t.Errorf("MaxWakeups = %d, want 50 (default)", task.MaxWakeups)
	}
}

func TestCreateDuplicate(t *testing.T) {
	s := NewStore(nil)
	s.Create("t1", "T", "d", nil, 10)
	_, err := s.Create("t1", "T2", "d2", nil, 10)
	if err == nil {
		t.Error("creating duplicate task should return error")
	}
}

func TestGetNotFound(t *testing.T) {
	s := NewStore(nil)
	if s.Get("missing") != nil {
		t.Error("Get for missing task should return nil")
	}
}

func TestActivate(t *testing.T) {
	s := NewStore(nil)
	s.Create("t1", "T", "d", nil, 10)

	if err := s.Activate("t1"); err != nil {
		t.Fatalf("Activate: %v", err)
	}
	task := s.Get("t1")
	if task.State != StateActive {
		t.Errorf("State = %q, want active", task.State)
	}
	if task.WakeupCount != 1 {
		t.Errorf("WakeupCount = %d, want 1", task.WakeupCount)
	}
}

func TestActivateNotFound(t *testing.T) {
	s := NewStore(nil)
	if err := s.Activate("missing"); err == nil {
		t.Error("Activate on missing task should return error")
	}
}

func TestActivateExceedsMaxWakeups(t *testing.T) {
	s := NewStore(nil)
	s.Create("t1", "T", "d", nil, 2)

	s.Activate("t1")
	s.Activate("t1")
	err := s.Activate("t1") // 3rd wakeup exceeds max of 2
	if err == nil {
		t.Error("should error when exceeding max wakeups")
	}
	task := s.Get("t1")
	if task.State != StateFailed {
		t.Errorf("State = %q, want failed after exceeding wakeups", task.State)
	}
}

func TestAddHandoff(t *testing.T) {
	s := NewStore(nil)
	s.Create("t1", "T", "d", nil, 10)

	err := s.AddHandoff("t1", Handoff{
		FromSession: "sess-1",
		ToSession:   "sess-2",
		Summary:     "handing off",
		Context:     "some context",
	})
	if err != nil {
		t.Fatalf("AddHandoff: %v", err)
	}

	task := s.Get("t1")
	if len(task.Handoffs) != 1 {
		t.Fatalf("Handoffs len = %d, want 1", len(task.Handoffs))
	}
	if task.Handoffs[0].ID != 1 {
		t.Errorf("Handoff ID = %d, want 1", task.Handoffs[0].ID)
	}
	if task.State != StateWaiting {
		t.Errorf("State = %q, want waiting after handoff", task.State)
	}
	if task.Handoffs[0].CreatedAt.IsZero() {
		t.Error("Handoff CreatedAt should be set")
	}
}

func TestAddHandoffNotFound(t *testing.T) {
	s := NewStore(nil)
	err := s.AddHandoff("missing", Handoff{})
	if err == nil {
		t.Error("AddHandoff on missing task should return error")
	}
}

func TestCompleteStep(t *testing.T) {
	s := NewStore(nil)
	plan := []PlanStep{
		{Name: "step1", Description: "first"},
		{Name: "step2", Description: "second"},
	}
	s.Create("t1", "T", "d", plan, 10)

	if err := s.CompleteStep("t1", 0, "done"); err != nil {
		t.Fatalf("CompleteStep: %v", err)
	}
	task := s.Get("t1")
	if task.Plan[0].State != StateCompleted {
		t.Errorf("step 0 state = %q, want completed", task.Plan[0].State)
	}
	if task.Plan[0].Outcome != "done" {
		t.Errorf("step 0 outcome = %q, want done", task.Plan[0].Outcome)
	}
	if task.Plan[0].CompletedAt == nil {
		t.Error("step 0 CompletedAt should be set")
	}
}

func TestCompleteStepInvalidIndex(t *testing.T) {
	s := NewStore(nil)
	s.Create("t1", "T", "d", []PlanStep{{Name: "s1"}}, 10)

	if err := s.CompleteStep("t1", -1, "x"); err == nil {
		t.Error("negative index should error")
	}
	if err := s.CompleteStep("t1", 5, "x"); err == nil {
		t.Error("out-of-range index should error")
	}
}

func TestCompleteStepNotFound(t *testing.T) {
	s := NewStore(nil)
	if err := s.CompleteStep("missing", 0, "x"); err == nil {
		t.Error("CompleteStep on missing task should error")
	}
}

func TestComplete(t *testing.T) {
	s := NewStore(nil)
	s.Create("t1", "T", "d", nil, 10)

	if err := s.Complete("t1"); err != nil {
		t.Fatalf("Complete: %v", err)
	}
	task := s.Get("t1")
	if task.State != StateCompleted {
		t.Errorf("State = %q, want completed", task.State)
	}
	if task.CompletedAt == nil {
		t.Error("CompletedAt should be set")
	}
}

func TestCompleteWithJudgePass(t *testing.T) {
	judge := func(task *Task) (bool, string) {
		return true, "looks good"
	}
	s := NewStore(judge)
	s.Create("t1", "T", "d", nil, 10)

	if err := s.Complete("t1"); err != nil {
		t.Fatalf("Complete: %v", err)
	}
	task := s.Get("t1")
	if !task.JudgeVerified {
		t.Error("JudgeVerified should be true")
	}
	if task.JudgeResult != "looks good" {
		t.Errorf("JudgeResult = %q", task.JudgeResult)
	}
	if task.State != StateCompleted {
		t.Errorf("State = %q, want completed", task.State)
	}
}

func TestCompleteWithJudgeReject(t *testing.T) {
	judge := func(task *Task) (bool, string) {
		return false, "not good enough"
	}
	s := NewStore(judge)
	s.Create("t1", "T", "d", nil, 10)

	err := s.Complete("t1")
	if err == nil {
		t.Fatal("Complete should error when judge rejects")
	}
	task := s.Get("t1")
	if task.State != StateFailed {
		t.Errorf("State = %q, want failed after judge rejection", task.State)
	}
	if !task.JudgeVerified {
		t.Error("JudgeVerified should be true")
	}
}

func TestCompleteNotFound(t *testing.T) {
	s := NewStore(nil)
	if err := s.Complete("missing"); err == nil {
		t.Error("Complete on missing task should error")
	}
}

func TestList(t *testing.T) {
	s := NewStore(nil)
	s.Create("t1", "T1", "d1", nil, 10)
	s.Create("t2", "T2", "d2", nil, 10)

	tasks := s.List()
	if len(tasks) != 2 {
		t.Errorf("List returned %d tasks, want 2", len(tasks))
	}
}

func TestListActive(t *testing.T) {
	s := NewStore(nil)
	s.Create("t1", "T1", "d1", nil, 10)
	s.Create("t2", "T2", "d2", nil, 10)
	s.Create("t3", "T3", "d3", nil, 10)

	s.Activate("t1")                          // active
	s.AddHandoff("t2", Handoff{Summary: "h"}) // waiting
	// t3 remains pending

	active := s.ListActive()
	if len(active) != 2 {
		t.Errorf("ListActive returned %d, want 2 (active + waiting)", len(active))
	}
}

func TestConcurrentAccess(t *testing.T) {
	s := NewStore(nil)
	s.Create("t1", "T", "d", nil, 100)

	done := make(chan struct{})
	go func() {
		for i := 0; i < 50; i++ {
			s.Activate("t1")
		}
		close(done)
	}()

	for i := 0; i < 50; i++ {
		s.Get("t1")
	}
	<-done
}

func TestHandoffAutoIncrementID(t *testing.T) {
	s := NewStore(nil)
	s.Create("t1", "T", "d", nil, 10)

	for i := 1; i <= 3; i++ {
		s.AddHandoff("t1", Handoff{Summary: "h"})
		task := s.Get("t1")
		if task.Handoffs[i-1].ID != i {
			t.Errorf("handoff %d ID = %d, want %d", i, task.Handoffs[i-1].ID, i)
		}
	}
}

func TestCreatedAtSet(t *testing.T) {
	before := time.Now()
	s := NewStore(nil)
	task, _ := s.Create("t1", "T", "d", nil, 10)
	after := time.Now()

	if task.CreatedAt.Before(before) || task.CreatedAt.After(after) {
		t.Errorf("CreatedAt %v not between %v and %v", task.CreatedAt, before, after)
	}
}
