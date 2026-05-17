package task

import (
	"fmt"
	"sync"
	"time"
)

type TaskState string

const (
	StatePending   TaskState = "pending"
	StateActive    TaskState = "active"
	StateWaiting   TaskState = "waiting"
	StateCompleted TaskState = "completed"
	StateFailed    TaskState = "failed"
	StateAbandoned TaskState = "abandoned"
)

type PlanStep struct {
	Name        string     `json:"name"`
	Description string     `json:"description"`
	State       TaskState  `json:"state"`
	Outcome     string     `json:"outcome,omitempty"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
}

type Handoff struct {
	ID          int       `json:"id"`
	FromSession string    `json:"from_session"`
	ToSession   string    `json:"to_session,omitempty"`
	Summary     string    `json:"summary"`
	Context     string    `json:"context"`
	CreatedAt   time.Time `json:"created_at"`
}

type Task struct {
	ID            string     `json:"id"`
	Name          string     `json:"name"`
	Description   string     `json:"description"`
	State         TaskState  `json:"state"`
	Plan          []PlanStep `json:"plan"`
	Handoffs      []Handoff  `json:"handoffs"`
	CreatedAt     time.Time  `json:"created_at"`
	CompletedAt   *time.Time `json:"completed_at,omitempty"`
	WakeupCount   int        `json:"wakeup_count"`
	MaxWakeups    int        `json:"max_wakeups"`
	JudgeVerified bool       `json:"judge_verified"`
	JudgeResult   string     `json:"judge_result,omitempty"`
	CheckpointRef string     `json:"checkpoint_ref,omitempty"`
}

type JudgeFunc func(task *Task) (passed bool, reason string)

type Store struct {
	mu    sync.RWMutex
	tasks map[string]*Task
	judge JudgeFunc
}

func NewStore(judge JudgeFunc) *Store {
	return &Store{
		tasks: make(map[string]*Task),
		judge: judge,
	}
}

func (s *Store) Create(id, name, description string, plan []PlanStep, maxWakeups int) (*Task, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.tasks[id]; exists {
		return nil, fmt.Errorf("task %s already exists", id)
	}

	if maxWakeups <= 0 {
		maxWakeups = 50
	}

	task := &Task{
		ID:          id,
		Name:        name,
		Description: description,
		State:       StatePending,
		Plan:        plan,
		Handoffs:    make([]Handoff, 0),
		CreatedAt:   time.Now(),
		MaxWakeups:  maxWakeups,
	}
	s.tasks[id] = task
	return task, nil
}

func (s *Store) Get(id string) *Task {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.tasks[id]
}

func (s *Store) Activate(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	task, ok := s.tasks[id]
	if !ok {
		return fmt.Errorf("task %s not found", id)
	}
	task.State = StateActive
	task.WakeupCount++

	if task.WakeupCount > task.MaxWakeups {
		task.State = StateFailed
		return fmt.Errorf("task %s exceeded max wakeups (%d)", id, task.MaxWakeups)
	}
	return nil
}

func (s *Store) AddHandoff(taskID string, handoff Handoff) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	task, ok := s.tasks[taskID]
	if !ok {
		return fmt.Errorf("task %s not found", taskID)
	}
	handoff.ID = len(task.Handoffs) + 1
	handoff.CreatedAt = time.Now()
	task.Handoffs = append(task.Handoffs, handoff)
	task.State = StateWaiting
	return nil
}

func (s *Store) CompleteStep(taskID string, stepIdx int, outcome string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	task, ok := s.tasks[taskID]
	if !ok {
		return fmt.Errorf("task %s not found", taskID)
	}
	if stepIdx < 0 || stepIdx >= len(task.Plan) {
		return fmt.Errorf("invalid step index %d", stepIdx)
	}

	now := time.Now()
	task.Plan[stepIdx].State = StateCompleted
	task.Plan[stepIdx].Outcome = outcome
	task.Plan[stepIdx].CompletedAt = &now
	return nil
}

func (s *Store) Complete(taskID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	task, ok := s.tasks[taskID]
	if !ok {
		return fmt.Errorf("task %s not found", taskID)
	}

	// Run judge if configured
	if s.judge != nil {
		passed, reason := s.judge(task)
		task.JudgeVerified = true
		task.JudgeResult = reason
		if !passed {
			task.State = StateFailed
			return fmt.Errorf("judge rejected: %s", reason)
		}
	}

	now := time.Now()
	task.State = StateCompleted
	task.CompletedAt = &now
	return nil
}

func (s *Store) List() []*Task {
	s.mu.RLock()
	defer s.mu.RUnlock()

	tasks := make([]*Task, 0, len(s.tasks))
	for _, t := range s.tasks {
		tasks = append(tasks, t)
	}
	return tasks
}

func (s *Store) ListActive() []*Task {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var active []*Task
	for _, t := range s.tasks {
		if t.State == StateActive || t.State == StateWaiting {
			active = append(active, t)
		}
	}
	return active
}
