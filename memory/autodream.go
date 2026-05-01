package memory

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// AutoDreamConfig controls when background memory consolidation triggers.
type AutoDreamConfig struct {
	Enabled           bool
	MinElapsedTime    time.Duration // minimum time since last dream
	MinNewSessions    int           // minimum new sessions since last dream
	ConsolidatePrompt string        // prompt for the dream agent
}

// DefaultAutoDreamConfig returns the default auto-dream settings.
func DefaultAutoDreamConfig() AutoDreamConfig {
	return AutoDreamConfig{
		Enabled:        true,
		MinElapsedTime: 24 * time.Hour,
		MinNewSessions: 5,
		ConsolidatePrompt: `Review the recent session memories and consolidate them into a coherent summary.
Focus on: recurring patterns, user preferences learned, project context that should persist,
and any corrections or feedback the user gave. Remove redundant or outdated memories.
Write the consolidated result as a clear, organized memory document.`,
	}
}

// AutoDreamState tracks the last dream execution.
type AutoDreamState struct {
	mu              sync.Mutex
	LastDreamTime   time.Time `json:"last_dream_time"`
	SessionsSince   int       `json:"sessions_since"`
	DreamCount      int       `json:"dream_count"`
	LastError       string    `json:"last_error,omitempty"`
}

// NewAutoDreamState creates a new auto-dream state.
func NewAutoDreamState() *AutoDreamState {
	return &AutoDreamState{
		LastDreamTime: time.Now(),
	}
}

// RecordSession increments the session counter.
func (s *AutoDreamState) RecordSession() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.SessionsSince++
}

// ShouldDream checks if conditions are met for a background dream.
func (s *AutoDreamState) ShouldDream(cfg AutoDreamConfig) bool {
	if !cfg.Enabled {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	elapsed := time.Since(s.LastDreamTime)
	return elapsed >= cfg.MinElapsedTime && s.SessionsSince >= cfg.MinNewSessions
}

// MarkDreamComplete records a successful dream execution.
func (s *AutoDreamState) MarkDreamComplete() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.LastDreamTime = time.Now()
	s.SessionsSince = 0
	s.DreamCount++
	s.LastError = ""
}

// MarkDreamFailed records a failed dream attempt.
func (s *AutoDreamState) MarkDreamFailed(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.LastError = err.Error()
}

// DreamResult holds the outcome of a dream operation.
type DreamResult struct {
	ConsolidatedMemory string
	MemoriesProcessed  int
	Duration           time.Duration
}

// RunDream executes background memory consolidation.
// agentFn is called with the consolidation prompt and should return the LLM response.
func RunDream(ctx context.Context, cfg AutoDreamConfig, agentFn func(ctx context.Context, prompt string) (string, error)) (*DreamResult, error) {
	start := time.Now()

	// Read existing memories
	memDir := autoDreamMemoryDir()
	entries, err := os.ReadDir(memDir)
	if err != nil {
		return nil, fmt.Errorf("reading memory directory: %w", err)
	}

	var memoryContent string
	memCount := 0
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".md" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(memDir, e.Name()))
		if err != nil {
			continue
		}
		memoryContent += fmt.Sprintf("--- %s ---\n%s\n\n", e.Name(), string(data))
		memCount++
	}

	if memCount == 0 {
		return nil, fmt.Errorf("no memories to consolidate")
	}

	prompt := cfg.ConsolidatePrompt + "\n\nCurrent memories:\n" + memoryContent
	result, err := agentFn(ctx, prompt)
	if err != nil {
		return nil, fmt.Errorf("dream agent: %w", err)
	}

	// Write consolidated memory
	consolidatedPath := filepath.Join(memDir, "consolidated.md")
	if err := os.WriteFile(consolidatedPath, []byte(result), 0o644); err != nil {
		return nil, fmt.Errorf("writing consolidated memory: %w", err)
	}

	return &DreamResult{
		ConsolidatedMemory: result,
		MemoriesProcessed:  memCount,
		Duration:           time.Since(start),
	}, nil
}

func autoDreamMemoryDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".hawk", "memory")
}
