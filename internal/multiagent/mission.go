package mission

import (
	"context"
	"encoding/json"
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
