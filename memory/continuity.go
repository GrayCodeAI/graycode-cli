package memory

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// ContinuityTracker measures the effectiveness of memory across sessions.
// It tracks token savings, re-explanation avoidance, and generates a
// "memory continuity score" that shows how much memory helps.
type ContinuityTracker struct {
	mu       sync.Mutex
	sessions []ContinuitySession
	savePath string
}

// ContinuitySession records memory effectiveness for a single session.
type ContinuitySession struct {
	SessionID        string    `json:"session_id"`
	StartedAt        time.Time `json:"started_at"`
	Duration         time.Duration `json:"duration"`
	MemoryInjected   bool      `json:"memory_injected"`
	MemoriesAccessed int       `json:"memories_accessed"`
	TokensFromMemory int       `json:"tokens_from_memory"`
	TokensSaved      int       `json:"tokens_saved"`
	ReExplanations   int       `json:"re_explanations"`
	TaskSuccess      bool      `json:"task_success"`
	Score            float64   `json:"score"`
}

// ContinuityReport summarizes memory effectiveness over time.
type ContinuityReport struct {
	TotalSessions      int     `json:"total_sessions"`
	SessionsWithMemory int     `json:"sessions_with_memory"`
	AvgScore           float64 `json:"avg_score"`
	TotalTokensSaved   int     `json:"total_tokens_saved"`
	AvgReExplanations  float64 `json:"avg_re_explanations"`
	SuccessRate        float64 `json:"success_rate"`
	MemoryContribution float64 `json:"memory_contribution"` // 0-1 how much memory helped
}

// NewContinuityTracker creates a tracker that persists across sessions.
func NewContinuityTracker(projectDir string) *ContinuityTracker {
	savePath := ""
	if projectDir != "" {
		savePath = filepath.Join(projectDir, ".yaad", "continuity.json")
	}
	ct := &ContinuityTracker{
		sessions: make([]ContinuitySession, 0),
		savePath: savePath,
	}
	ct.load()
	return ct
}

// StartSession begins tracking a new session.
func (ct *ContinuityTracker) StartSession(sessionID string, memoryInjected bool) {
	ct.mu.Lock()
	defer ct.mu.Unlock()

	ct.sessions = append(ct.sessions, ContinuitySession{
		SessionID:      sessionID,
		StartedAt:      time.Now(),
		MemoryInjected: memoryInjected,
	})
}

// RecordMemoryUse records that memory was accessed during the current session.
func (ct *ContinuityTracker) RecordMemoryUse(memoriesAccessed, tokensUsed int) {
	ct.mu.Lock()
	defer ct.mu.Unlock()

	if len(ct.sessions) == 0 {
		return
	}
	current := &ct.sessions[len(ct.sessions)-1]
	current.MemoriesAccessed += memoriesAccessed
	current.TokensFromMemory += tokensUsed
}

// RecordReExplanation notes that the user had to re-explain something.
func (ct *ContinuityTracker) RecordReExplanation() {
	ct.mu.Lock()
	defer ct.mu.Unlock()

	if len(ct.sessions) == 0 {
		return
	}
	ct.sessions[len(ct.sessions)-1].ReExplanations++
}

// EndSession closes the current session and computes its score.
func (ct *ContinuityTracker) EndSession(success bool) {
	ct.mu.Lock()
	defer ct.mu.Unlock()

	if len(ct.sessions) == 0 {
		return
	}
	current := &ct.sessions[len(ct.sessions)-1]
	current.Duration = time.Since(current.StartedAt)
	current.TaskSuccess = success
	current.TokensSaved = ct.estimateTokensSaved(current)
	current.Score = ct.computeScore(current)
	ct.saveNoLock()
}

func (ct *ContinuityTracker) estimateTokensSaved(s *ContinuitySession) int {
	// Each memory access saves ~500 tokens of re-explanation
	// Each re-explanation costs ~500 tokens
	const tokensPerReExplanation = 500
	saved := s.MemoriesAccessed*tokensPerReExplanation - s.ReExplanations*tokensPerReExplanation
	if saved < 0 {
		saved = 0
	}
	return saved
}

func (ct *ContinuityTracker) computeScore(s *ContinuitySession) float64 {
	// Score: 0-100 based on:
	// - Memory used effectively (40%)
	// - No re-explanations needed (30%)
	// - Task succeeded (30%)
	score := 0.0

	// Memory usage (0-40)
	if s.MemoriesAccessed > 0 {
		memScore := float64(s.MemoriesAccessed) * 10
		if memScore > 40 {
			memScore = 40
		}
		score += memScore
	}

	// Re-explanation avoidance (0-30)
	if s.ReExplanations == 0 {
		score += 30
	} else {
		penalty := float64(s.ReExplanations) * 10
		reScore := 30 - penalty
		if reScore < 0 {
			reScore = 0
		}
		score += reScore
	}

	// Task success (0-30)
	if s.TaskSuccess {
		score += 30
	}

	return score
}

// Report generates an aggregate report across all tracked sessions.
func (ct *ContinuityTracker) Report() ContinuityReport {
	ct.mu.Lock()
	defer ct.mu.Unlock()

	report := ContinuityReport{
		TotalSessions: len(ct.sessions),
	}
	if len(ct.sessions) == 0 {
		return report
	}

	var totalScore float64
	var totalReExplain int
	var successCount int

	for _, s := range ct.sessions {
		totalScore += s.Score
		totalReExplain += s.ReExplanations
		report.TotalTokensSaved += s.TokensSaved
		if s.MemoryInjected {
			report.SessionsWithMemory++
		}
		if s.TaskSuccess {
			successCount++
		}
	}

	report.AvgScore = totalScore / float64(len(ct.sessions))
	report.AvgReExplanations = float64(totalReExplain) / float64(len(ct.sessions))
	report.SuccessRate = float64(successCount) / float64(len(ct.sessions))

	// Memory contribution: compare success rate of memory sessions vs non-memory
	memSuccess := 0
	memTotal := 0
	noMemSuccess := 0
	noMemTotal := 0
	for _, s := range ct.sessions {
		if s.MemoryInjected {
			memTotal++
			if s.TaskSuccess {
				memSuccess++
			}
		} else {
			noMemTotal++
			if s.TaskSuccess {
				noMemSuccess++
			}
		}
	}
	if memTotal > 0 && noMemTotal > 0 {
		memRate := float64(memSuccess) / float64(memTotal)
		noMemRate := float64(noMemSuccess) / float64(noMemTotal)
		report.MemoryContribution = memRate - noMemRate
		if report.MemoryContribution < 0 {
			report.MemoryContribution = 0
		}
	}

	return report
}

// FormatSummary returns a human-readable session summary.
func (ct *ContinuityTracker) FormatSummary() string {
	r := ct.Report()
	if r.TotalSessions == 0 {
		return ""
	}
	return fmt.Sprintf(
		"Memory continuity: %.0f/100 avg score, %d tokens saved, %.1f re-explanations/session",
		r.AvgScore, r.TotalTokensSaved, r.AvgReExplanations,
	)
}

// Save persists the tracker state.
func (ct *ContinuityTracker) Save() {
	ct.mu.Lock()
	defer ct.mu.Unlock()
	ct.saveNoLock()
}

func (ct *ContinuityTracker) saveNoLock() {
	if ct.savePath == "" {
		return
	}
	dir := filepath.Dir(ct.savePath)
	_ = os.MkdirAll(dir, 0o755)

	// Keep last 100 sessions
	if len(ct.sessions) > 100 {
		ct.sessions = ct.sessions[len(ct.sessions)-100:]
	}

	data, err := json.Marshal(ct.sessions)
	if err != nil {
		return
	}
	_ = os.WriteFile(ct.savePath, data, 0o644)
}

func (ct *ContinuityTracker) load() {
	if ct.savePath == "" {
		return
	}
	data, err := os.ReadFile(ct.savePath)
	if err != nil {
		return
	}
	_ = json.Unmarshal(data, &ct.sessions)
}
