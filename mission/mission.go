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
	ID          string     `json:"id"`
	Prompt      string     `json:"prompt"`
	Dir         string     `json:"dir"`
	Features    []Feature  `json:"features"`
	Status      Status     `json:"status"`
	StartedAt   time.Time  `json:"started_at"`
	CompletedAt time.Time  `json:"completed_at,omitempty"`
	Config      Config     `json:"config"`
	mu          sync.Mutex
}

// Config controls mission orchestration behavior.
type Config struct {
	MaxWorkers        int    `json:"max_workers"`
	WorkerModel       string `json:"worker_model"`
	ValidatorModel    string `json:"validator_model"`
	RepoDir           string `json:"repo_dir"`
	BaseBranch        string `json:"base_branch"`
	AutonomyLevel     int    `json:"autonomy_level"`
	SkipValidation    bool   `json:"skip_validation"`
}

// Status represents the mission lifecycle.
type Status string

const (
	StatusPlanning   Status = "planning"
	StatusRunning    Status = "running"
	StatusValidating Status = "validating"
	StatusCompleted  Status = "completed"
	StatusFailed     Status = "failed"
)

// Feature is a discrete unit of work assigned to a worker.
type Feature struct {
	ID               string        `json:"id"`
	Description      string        `json:"description"`
	ExpectedBehavior string        `json:"expected_behavior"`
	Branch           string        `json:"branch"`
	WorkerSessionID  string        `json:"worker_session_id,omitempty"`
	Status           FeatureStatus `json:"status"`
	Handoff          *Handoff      `json:"handoff,omitempty"`
	StartedAt        time.Time     `json:"started_at,omitempty"`
	CompletedAt      time.Time     `json:"completed_at,omitempty"`
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
			m.mu.Unlock()

			handoff, err := workerFn(ctx, feat, missionDir, m.Config)

			m.mu.Lock()
			if err != nil {
				feat.Status = FeatureFailed
			} else {
				feat.Status = FeatureCompleted
				feat.Handoff = handoff
			}
			feat.CompletedAt = time.Now()
			m.mu.Unlock()

			_ = m.persistState()
		}(&m.Features[i])
	}

	wg.Wait()

	allDone := true
	for _, f := range m.Features {
		if f.Status == FeatureFailed {
			allDone = false
			break
		}
	}

	m.mu.Lock()
	if allDone {
		m.Status = StatusCompleted
	} else {
		m.Status = StatusFailed
	}
	m.CompletedAt = time.Now()
	m.mu.Unlock()

	return m.persistState()
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
	return fmt.Sprintf("Mission %s: %d/%d features completed, %d failed (%s)",
		m.ID, completed, len(m.Features), failed, duration)
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
