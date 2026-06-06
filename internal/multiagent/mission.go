package mission

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/google/uuid"
)

// Mission represents a multi-agent orchestration run.
type Mission struct {
	ID          string    `json:"id"`
	Prompt      string    `json:"prompt"`
	Dir         string    `json:"dir"`
	Features    []Feature `json:"features"`
	Status      Status    `json:"status"`
	StartedAt   time.Time `json:"started_at"`
	CompletedAt time.Time `json:"completed_at,omitempty"`
	Config      Config    `json:"config"`
	mu          sync.Mutex
}

// Config controls mission orchestration behavior.
type Config struct {
	MaxWorkers        int           `json:"max_workers"`
	WorkerModel       string        `json:"worker_model"`
	ValidatorModel    string        `json:"validator_model"`
	RepoDir           string        `json:"repo_dir"`
	BaseBranch        string        `json:"base_branch"`
	AutonomyLevel     int           `json:"autonomy_level"`
	SkipValidation    bool          `json:"skip_validation"`
	PerWorkerTimeout  time.Duration `json:"per_worker_timeout,omitempty"`
	MaxRetriesPerFeat int           `json:"max_retries_per_feat,omitempty"`

	// Staged pipeline configuration (oh-my-claudecode-style team workflow).
	PRDModel          string `json:"prd_model,omitempty"`
	FixModel          string `json:"fix_model,omitempty"`
	MaxFixAttempts    int    `json:"max_fix_attempts,omitempty"` // default 3
	EnablePRDPhase    bool   `json:"enable_prd_phase,omitempty"`
	EnableVerifyPhase bool   `json:"enable_verify_phase,omitempty"`
}

// Status represents the mission lifecycle.
type Status string

const (
	StatusPlanning    Status = "planning"
	StatusPlanningPRD Status = "planning_prd"
	StatusRunning     Status = "running"
	StatusExecuting   Status = "executing"
	StatusVerifying   Status = "verifying"
	StatusFixing      Status = "fixing"
	StatusValidating  Status = "validating"
	StatusCompleted   Status = "completed"
	StatusFailed      Status = "failed"
	StatusPartial     Status = "partial"
)

// Feature is a discrete unit of work assigned to a worker.
type Feature struct {
	ID                 string        `json:"id"`
	Description        string        `json:"description"`
	ExpectedBehavior   string        `json:"expected_behavior"`
	Branch             string        `json:"branch"`
	WorkerSessionID    string        `json:"worker_session_id,omitempty"`
	Status             FeatureStatus `json:"status"`
	Handoff            *Handoff      `json:"handoff,omitempty"`
	StartedAt          time.Time     `json:"started_at,omitempty"`
	CompletedAt        time.Time     `json:"completed_at,omitempty"`
	PRD                string        `json:"prd,omitempty"`                 // generated product requirements
	VerificationResult string        `json:"verification_result,omitempty"` // verify-phase outcome
	FixAttempts        int           `json:"fix_attempts,omitempty"`        // number of fix passes applied
}

// FeatureStatus tracks individual feature progress.
type FeatureStatus string

const (
	FeaturePending    FeatureStatus = "pending"
	FeatureInProgress FeatureStatus = "in_progress"
	FeatureCompleted  FeatureStatus = "completed"
	FeatureFailed     FeatureStatus = "failed"
)

// Handoff is the structured report a worker produces upon completion.
type Handoff struct {
	CommitID     string   `json:"commit_id,omitempty"`
	RepoPath     string   `json:"repo_path,omitempty"`
	Summary      string   `json:"summary"`
	FilesChanged []string `json:"files_changed,omitempty"`
	TestsPassed  bool     `json:"tests_passed"`
}

// WorkerFunc is the function type that the orchestrator calls for each feature.
// It receives the feature and mission dir, and returns a handoff or error.
type WorkerFunc func(ctx context.Context, feature *Feature, missionDir string, cfg Config) (*Handoff, error)

// New creates a new mission from a prompt and configuration.
func New(prompt string, cfg Config) *Mission {
	if cfg.MaxWorkers <= 0 {
		cfg.MaxWorkers = 4
	}
	if cfg.BaseBranch == "" {
		cfg.BaseBranch = "main"
	}
	if cfg.MaxFixAttempts <= 0 {
		cfg.MaxFixAttempts = 3
	}
	return &Mission{
		ID:        uuid.New().String()[:8],
		Prompt:    prompt,
		Status:    StatusPlanning,
		StartedAt: time.Now(),
		Config:    cfg,
	}
}

// Plan decomposes the mission prompt into features.
// planFn is called with the prompt and should return a list of features.
type PlanFunc func(ctx context.Context, prompt string) ([]Feature, error)

func (m *Mission) Plan(ctx context.Context, planFn PlanFunc) error {
	features, err := planFn(ctx, m.Prompt)
	if err != nil {
		m.Status = StatusFailed
		return fmt.Errorf("planning failed: %w", err)
	}
	for i := range features {
		if features[i].ID == "" {
			features[i].ID = fmt.Sprintf("feat-%d", i+1)
		}
		features[i].Status = FeaturePending
		features[i].Branch = fmt.Sprintf("hawk-mission/%s/%s", m.ID, features[i].ID)
	}
	m.Features = features
	return nil
}

// Run executes all features in parallel using workerFn.
func (m *Mission) Run(ctx context.Context, workerFn WorkerFunc) error {
	m.mu.Lock()
	m.Status = StatusRunning
	m.mu.Unlock()

	missionDir, err := m.createDir()
	if err != nil {
		return err
	}
	m.Dir = missionDir

	if err := m.persistState(); err != nil {
		return err
	}

	sem := make(chan struct{}, m.Config.MaxWorkers)
	var wg sync.WaitGroup

	for i := range m.Features {
		wg.Add(1)
		go func(feat *Feature) {
			defer wg.Done()

			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				m.mu.Lock()
				feat.Status = FeatureFailed
				m.mu.Unlock()
				return
			}
			defer func() { <-sem }()

			m.mu.Lock()
			feat.Status = FeatureInProgress
			feat.StartedAt = time.Now()
			maxRetries := m.Config.MaxRetriesPerFeat
			if maxRetries <= 0 {
				maxRetries = 2
			}
			m.mu.Unlock()

			var handoff *Handoff
			var err error
		retryLoop:
			for attempt := 0; attempt <= maxRetries; attempt++ {
				workerCtx := ctx
				cancel := func() {}
				if m.Config.PerWorkerTimeout > 0 {
					workerCtx, cancel = context.WithTimeout(ctx, m.Config.PerWorkerTimeout)
				}
				handoff, err = workerFn(workerCtx, feat, missionDir, m.Config)
				cancel()
				if err == nil {
					break
				}
				if attempt < maxRetries {
					delay := time.Duration(1<<attempt) * time.Second
					select {
					case <-time.After(delay):
					case <-ctx.Done():
						err = ctx.Err()
						break retryLoop
					}
				}
			}

			m.mu.Lock()
			if err != nil {
				feat.Status = FeatureFailed
				feat.Handoff = nil
			} else {
				feat.Status = FeatureCompleted
				feat.Handoff = handoff
			}
			feat.CompletedAt = time.Now()
			m.mu.Unlock()

			if persistErr := m.persistState(); persistErr != nil {
				fmt.Fprintf(os.Stderr, "warning: failed to persist mission state: %v\n", persistErr)
			}
		}(&m.Features[i])
	}

	wg.Wait()

	completed := 0
	failed := 0
	for _, f := range m.Features {
		switch f.Status {
		case FeatureCompleted:
			completed++
		case FeatureFailed:
			failed++
		}
	}

	m.mu.Lock()
	if failed == 0 {
		m.Status = StatusCompleted
	} else if completed == 0 {
		m.Status = StatusFailed
	} else {
		m.Status = StatusPartial
	}
	m.CompletedAt = time.Now()
	m.mu.Unlock()

	return m.persistState()
}

// PRDFunc generates a product requirements document for a feature.
type PRDFunc func(ctx context.Context, feature *Feature) (prd string, err error)

// VerifyFunc validates a completed feature's implementation.
// Returns passed=true if the implementation satisfies the feature's expected behavior.
type VerifyFunc func(ctx context.Context, feature *Feature, handoff *Handoff) (passed bool, result string, err error)

// FixFunc attempts to fix a feature that failed verification.
type FixFunc func(ctx context.Context, feature *Feature, verificationResult string, handoff *Handoff) (*Handoff, error)

// GeneratePRD runs the PRD phase: generates a requirements doc for each feature.
func (m *Mission) GeneratePRD(ctx context.Context, prdFn PRDFunc) error {
	m.mu.Lock()
	m.Status = StatusPlanningPRD
	m.mu.Unlock()

	for i := range m.Features {
		feat := &m.Features[i]
		prd, err := prdFn(ctx, feat)
		if err != nil {
			return fmt.Errorf("PRD generation failed for %s: %w", feat.ID, err)
		}
		m.mu.Lock()
		feat.PRD = prd
		m.mu.Unlock()
	}
	return m.persistState()
}

// Verify runs the verify phase: validates each completed feature.
// Features that fail verification are marked FeatureFailed and their result recorded.
func (m *Mission) Verify(ctx context.Context, verifyFn VerifyFunc) error {
	m.mu.Lock()
	m.Status = StatusVerifying
	m.mu.Unlock()

	for i := range m.Features {
		feat := &m.Features[i]
		if feat.Status != FeatureCompleted {
			continue
		}
		passed, result, err := verifyFn(ctx, feat, feat.Handoff)
		if err != nil {
			return fmt.Errorf("verification failed for %s: %w", feat.ID, err)
		}
		m.mu.Lock()
		feat.VerificationResult = result
		if !passed {
			feat.Status = FeatureFailed
		}
		m.mu.Unlock()
	}
	return m.persistState()
}

// Fix runs the fix loop: for each failed feature with a recorded verification
// result, attempts up to MaxFixAttempts fixes, re-marking the feature completed on success.
func (m *Mission) Fix(ctx context.Context, fixFn FixFunc) error {
	m.mu.Lock()
	m.Status = StatusFixing
	maxAttempts := m.Config.MaxFixAttempts
	if maxAttempts <= 0 {
		maxAttempts = 3
	}
	m.mu.Unlock()

	for i := range m.Features {
		feat := &m.Features[i]
		if feat.Status != FeatureFailed || feat.VerificationResult == "" {
			continue
		}
		for feat.FixAttempts < maxAttempts {
			handoff, err := fixFn(ctx, feat, feat.VerificationResult, feat.Handoff)
			m.mu.Lock()
			feat.FixAttempts++
			if err == nil && handoff != nil {
				feat.Handoff = handoff
				feat.Status = FeatureCompleted
				feat.VerificationResult = ""
				m.mu.Unlock()
				break
			}
			m.mu.Unlock()
			if err != nil {
				// Retry until attempts exhausted.
				continue
			}
		}
	}
	return m.persistState()
}

// StagedOption configures RunStaged behavior.
type StagedOption func(*stagedConfig)

type stagedConfig struct {
	prdFn    PRDFunc
	verifyFn VerifyFunc
	fixFn    FixFunc
}

// WithPRD supplies the PRD generation function for the staged pipeline.
func WithPRD(fn PRDFunc) StagedOption { return func(c *stagedConfig) { c.prdFn = fn } }

// WithVerify supplies the verification function for the staged pipeline.
func WithVerify(fn VerifyFunc) StagedOption { return func(c *stagedConfig) { c.verifyFn = fn } }

// WithFix supplies the fix function for the staged pipeline.
func WithFix(fn FixFunc) StagedOption { return func(c *stagedConfig) { c.fixFn = fn } }

// RunStaged orchestrates the full team pipeline:
//
//	team-plan (already done via Plan) -> team-prd -> team-exec -> team-verify -> team-fix loop
//
// PRD and verify phases run only when enabled in Config. The fix loop runs when
// a fix function is provided and the verify phase is enabled.
func (m *Mission) RunStaged(ctx context.Context, workerFn WorkerFunc, opts ...StagedOption) error {
	var sc stagedConfig
	for _, opt := range opts {
		opt(&sc)
	}

	// Phase: team-prd
	if m.Config.EnablePRDPhase && sc.prdFn != nil {
		if err := m.GeneratePRD(ctx, sc.prdFn); err != nil {
			return err
		}
	}

	// Phase: team-exec (existing parallel execution)
	if err := m.Run(ctx, workerFn); err != nil {
		return err
	}

	// Phase: team-verify + team-fix
	if m.Config.EnableVerifyPhase && sc.verifyFn != nil {
		if err := m.Verify(ctx, sc.verifyFn); err != nil {
			return err
		}
		if sc.fixFn != nil {
			if err := m.Fix(ctx, sc.fixFn); err != nil {
				return err
			}
		}
		// Recompute final status after verify/fix.
		m.recomputeStatus()
	}

	return m.persistState()
}

// recomputeStatus recalculates the mission status from feature statuses.
func (m *Mission) recomputeStatus() {
	completed := 0
	failed := 0
	for _, f := range m.Features {
		switch f.Status {
		case FeatureCompleted:
			completed++
		case FeatureFailed:
			failed++
		}
	}
	m.mu.Lock()
	if failed == 0 {
		m.Status = StatusCompleted
	} else if completed == 0 {
		m.Status = StatusFailed
	} else {
		m.Status = StatusPartial
	}
	m.CompletedAt = time.Now()
	m.mu.Unlock()
}

// Summary returns a human-readable summary of the mission.
func (m *Mission) Summary() string {
	completed := 0
	failed := 0
	for _, f := range m.Features {
		switch f.Status {
		case FeatureCompleted:
			completed++
		case FeatureFailed:
			failed++
		}
	}
	duration := m.CompletedAt.Sub(m.StartedAt).Round(time.Second)
	if m.CompletedAt.IsZero() {
		duration = time.Since(m.StartedAt).Round(time.Second)
	}
	status := "completed"
	if failed > 0 && completed > 0 {
		status = "partial"
	} else if failed > 0 {
		status = "failed"
	}
	return fmt.Sprintf("Mission %s [%s]: %d/%d features completed, %d failed (%s)",
		m.ID, status, completed, len(m.Features), failed, duration)
}

func (m *Mission) createDir() (string, error) {
	dir := filepath.Join(os.TempDir(), "hawk-missions", m.ID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return dir, nil
}

func (m *Mission) persistState() error {
	if m.Dir == "" {
		return nil
	}
	m.mu.Lock()
	data, err := json.MarshalIndent(m, "", "  ")
	m.mu.Unlock()
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(m.Dir, "mission.json"), data, 0o644)
}

// ─────────────────────────────────────────────────────────────────────────────
// Durable workflow (LangGraph-style) — named step boundaries, state persistence
// across failures, explicit human-in-the-loop checkpoint gates, and resume from
// the last completed step on restart.
//
// A Workflow is a linear sequence of named steps. After every step completes,
// the workflow state is persisted to disk (workflow.json) alongside the mission
// JSON store. If the process crashes mid-run, Resume picks up from the first
// step that has not reached StepCompleted. Steps may be marked as human-in-the-
// loop gates: when reached, the workflow halts with ErrAwaitingApproval until an
// operator calls Approve (or Reject), making the gate decision itself durable.
// ─────────────────────────────────────────────────────────────────────────────

// StepStatus tracks the lifecycle of a single workflow step.
type StepStatus string

const (
	StepPending        StepStatus = "pending"
	StepRunning        StepStatus = "running"
	StepCompleted      StepStatus = "completed"
	StepFailed         StepStatus = "failed"
	StepAwaitingApprov StepStatus = "awaiting_approval"
	StepRejected       StepStatus = "rejected"
)

// ErrAwaitingApproval is returned by Workflow.Run when execution halts at a
// human-in-the-loop checkpoint gate. The workflow state is durable at this
// point: a later Resume after Approve/Reject continues from the gate.
var ErrAwaitingApproval = errors.New("workflow halted: awaiting human approval at checkpoint gate")

// WorkflowStep is a durable, named unit of work in a Workflow.
type WorkflowStep struct {
	Name        string     `json:"name"`
	Status      StepStatus `json:"status"`
	HumanGate   bool       `json:"human_gate,omitempty"` // requires Approve before running
	Approved    bool       `json:"approved,omitempty"`
	StartedAt   time.Time  `json:"started_at,omitempty"`
	CompletedAt time.Time  `json:"completed_at,omitempty"`
	Attempts    int        `json:"attempts,omitempty"`
	Error       string     `json:"error,omitempty"`
	// Output is an opaque, JSON-serializable result the step produced. It is
	// persisted so resumed runs can read prior step output without re-running.
	Output json.RawMessage `json:"output,omitempty"`
}

// StepFunc executes a single workflow step. The returned bytes (may be nil) are
// persisted as the step's durable Output.
type StepFunc func(ctx context.Context, state *WorkflowState) (output json.RawMessage, err error)

// WorkflowState is the durable, serializable state of a Workflow. The bag of
// shared values (Values) lets steps pass data forward across restarts.
type WorkflowState struct {
	ID        string            `json:"id"`
	Name      string            `json:"name"`
	Steps     []WorkflowStep    `json:"steps"`
	Values    map[string]string `json:"values,omitempty"`
	CreatedAt time.Time         `json:"created_at"`
	UpdatedAt time.Time         `json:"updated_at"`
	Dir       string            `json:"-"`
}

// Workflow is a durable, resumable sequence of named steps.
type Workflow struct {
	state *WorkflowState
	funcs map[string]StepFunc
	mu    sync.Mutex
}

// NewWorkflow creates a durable workflow with the given name and ordered step
// definitions. Each definition contributes a named step and its executor. dir
// is where workflow.json is persisted; if empty, persistence is skipped.
type StepDef struct {
	Name      string
	Fn        StepFunc
	HumanGate bool
}

// NewWorkflow constructs a Workflow from ordered step definitions.
func NewWorkflow(name, dir string, defs ...StepDef) *Workflow {
	steps := make([]WorkflowStep, 0, len(defs))
	funcs := make(map[string]StepFunc, len(defs))
	for _, d := range defs {
		steps = append(steps, WorkflowStep{
			Name:      d.Name,
			Status:    StepPending,
			HumanGate: d.HumanGate,
		})
		funcs[d.Name] = d.Fn
	}
	return &Workflow{
		state: &WorkflowState{
			ID:        uuid.New().String()[:8],
			Name:      name,
			Steps:     steps,
			Values:    map[string]string{},
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
			Dir:       dir,
		},
		funcs: funcs,
	}
}

// workflowStatePath returns the on-disk location of a workflow's durable state.
func workflowStatePath(dir string) string {
	return filepath.Join(dir, "workflow.json")
}

// LoadWorkflow reads a previously persisted workflow from dir and rebinds the
// supplied step executors by name. Steps whose names are not present in defs are
// left without an executor (Resume will error if it needs to run them).
func LoadWorkflow(dir string, defs ...StepDef) (*Workflow, error) {
	data, err := os.ReadFile(workflowStatePath(dir))
	if err != nil {
		return nil, err
	}
	var st WorkflowState
	if err := json.Unmarshal(data, &st); err != nil {
		return nil, fmt.Errorf("parse workflow state: %w", err)
	}
	st.Dir = dir
	if st.Values == nil {
		st.Values = map[string]string{}
	}
	funcs := make(map[string]StepFunc, len(defs))
	for _, d := range defs {
		funcs[d.Name] = d.Fn
		// Reconcile HumanGate flag from the definition (executors aren't persisted).
		for i := range st.Steps {
			if st.Steps[i].Name == d.Name {
				st.Steps[i].HumanGate = d.HumanGate
			}
		}
	}
	return &Workflow{state: &st, funcs: funcs}, nil
}

// State returns a snapshot pointer to the workflow's durable state.
func (w *Workflow) State() *WorkflowState { return w.state }

// SetValue stores a shared value visible to subsequent steps and across restarts.
func (w *Workflow) SetValue(key, val string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.state.Values == nil {
		w.state.Values = map[string]string{}
	}
	w.state.Values[key] = val
}

// persist writes the workflow state to disk atomically (temp + rename). It is a
// no-op when Dir is empty. Callers must NOT hold w.mu (it locks internally).
func (w *Workflow) persist() error {
	w.mu.Lock()
	if w.state.Dir == "" {
		w.mu.Unlock()
		return nil
	}
	w.state.UpdatedAt = time.Now()
	dir := w.state.Dir
	data, err := json.MarshalIndent(w.state, "", "  ")
	w.mu.Unlock()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	target := workflowStatePath(dir)
	tmp := target + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, target)
}

// nextRunnable returns the index of the first step that still needs to run, or
// -1 if all steps are completed. A failed or rejected step is considered
// runnable again (resume retries it).
func (w *Workflow) nextRunnable() int {
	for i := range w.state.Steps {
		switch w.state.Steps[i].Status {
		case StepCompleted:
			continue
		default:
			return i
		}
	}
	return -1
}

// Run executes steps from the first non-completed step to the end. It persists
// after every step transition so a crash leaves a resumable state. When it
// reaches an un-approved human gate it persists StepAwaitingApprov and returns
// ErrAwaitingApproval. Resume is an alias for Run after Approve.
func (w *Workflow) Run(ctx context.Context) error {
	for {
		w.mu.Lock()
		idx := w.nextRunnable()
		w.mu.Unlock()
		if idx < 0 {
			return w.persist() // all completed
		}

		w.mu.Lock()
		step := &w.state.Steps[idx]

		// Human-in-the-loop gate: halt durably until approved.
		if step.HumanGate && !step.Approved {
			step.Status = StepAwaitingApprov
			w.mu.Unlock()
			if err := w.persist(); err != nil {
				return err
			}
			return ErrAwaitingApproval
		}
		if step.Status == StepRejected {
			w.mu.Unlock()
			return fmt.Errorf("workflow halted: step %q was rejected", step.Name)
		}

		fn := w.funcs[step.Name]
		step.Status = StepRunning
		step.StartedAt = time.Now()
		step.Attempts++
		state := w.state
		w.mu.Unlock()

		if err := w.persist(); err != nil {
			return err
		}

		if fn == nil {
			w.mu.Lock()
			step.Status = StepFailed
			step.Error = "no executor bound for step"
			w.mu.Unlock()
			_ = w.persist()
			return fmt.Errorf("workflow step %q has no executor", step.Name)
		}

		if err := ctx.Err(); err != nil {
			w.mu.Lock()
			step.Status = StepFailed
			step.Error = err.Error()
			w.mu.Unlock()
			_ = w.persist()
			return err
		}

		output, err := fn(ctx, state)

		w.mu.Lock()
		if err != nil {
			step.Status = StepFailed
			step.Error = err.Error()
			w.mu.Unlock()
			_ = w.persist()
			return fmt.Errorf("workflow step %q failed: %w", step.Name, err)
		}
		step.Status = StepCompleted
		step.Error = ""
		step.Output = output
		step.CompletedAt = time.Now()
		w.mu.Unlock()

		if err := w.persist(); err != nil {
			return err
		}
	}
}

// Resume continues a workflow from the last completed step. It is identical to
// Run; the name documents intent at call sites after a restart.
func (w *Workflow) Resume(ctx context.Context) error { return w.Run(ctx) }

// Approve records human approval for the named gate step, making the decision
// durable, so a subsequent Resume proceeds past the gate.
func (w *Workflow) Approve(name string) error {
	w.mu.Lock()
	for i := range w.state.Steps {
		if w.state.Steps[i].Name == name {
			if !w.state.Steps[i].HumanGate {
				w.mu.Unlock()
				return fmt.Errorf("step %q is not a human-in-the-loop gate", name)
			}
			w.state.Steps[i].Approved = true
			w.state.Steps[i].Status = StepPending
			w.mu.Unlock()
			return w.persist()
		}
	}
	w.mu.Unlock()
	return fmt.Errorf("workflow step %q not found", name)
}

// Reject records a human rejection for the named gate step. A rejected gate
// halts the workflow on the next Run/Resume.
func (w *Workflow) Reject(name string) error {
	w.mu.Lock()
	for i := range w.state.Steps {
		if w.state.Steps[i].Name == name {
			w.state.Steps[i].Approved = false
			w.state.Steps[i].Status = StepRejected
			w.mu.Unlock()
			return w.persist()
		}
	}
	w.mu.Unlock()
	return fmt.Errorf("workflow step %q not found", name)
}

// LastCompletedStep returns the name of the most recently completed step, or ""
// if none have completed yet. Useful for reporting resume points.
func (w *Workflow) LastCompletedStep() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	last := ""
	for i := range w.state.Steps {
		if w.state.Steps[i].Status == StepCompleted {
			last = w.state.Steps[i].Name
		}
	}
	return last
}

// Done reports whether every step has reached StepCompleted.
func (w *Workflow) Done() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.nextRunnable() < 0
}
