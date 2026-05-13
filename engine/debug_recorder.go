package engine

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// DebugSession captures a complete debugging workflow from symptom to resolution.
type DebugSession struct {
	ID                string       `json:"id"`
	StartTime         time.Time    `json:"start_time"`
	EndTime           *time.Time   `json:"end_time,omitempty"`
	Symptom           string       `json:"symptom"`
	RootCause         string       `json:"root_cause,omitempty"`
	Resolution        string       `json:"resolution,omitempty"`
	Steps             []DebugStep  `json:"steps"`
	FilesInvestigated []string     `json:"files_investigated"`
	HypothesesTested  []Hypothesis `json:"hypotheses_tested"`
	Successful        bool         `json:"successful"`
}

// DebugStep records a single action taken during a debugging session.
type DebugStep struct {
	Index        int       `json:"index"`
	Action       string    `json:"action"` // "read", "grep", "test", "hypothesis", "fix_attempt"
	Target       string    `json:"target"`
	Result       string    `json:"result"`
	Timestamp    time.Time `json:"timestamp"`
	InsightGained string   `json:"insight_gained,omitempty"`
}

// Hypothesis represents a debugging hypothesis that can be tested and confirmed or rejected.
type Hypothesis struct {
	Description string `json:"description"`
	Tested      bool   `json:"tested"`
	Confirmed   bool   `json:"confirmed"`
	Evidence    string `json:"evidence,omitempty"`
}

// DebugRecorder manages debug sessions, recording steps and persisting them for future reference.
type DebugRecorder struct {
	Sessions      []*DebugSession `json:"sessions"`
	ActiveSession *DebugSession   `json:"active_session,omitempty"`
	Dir           string          `json:"dir"`
	mu            sync.Mutex
}

// NewDebugRecorder creates a new DebugRecorder that persists sessions to the given directory.
func NewDebugRecorder(dir string) *DebugRecorder {
	return &DebugRecorder{
		Sessions: make([]*DebugSession, 0),
		Dir:      dir,
	}
}

// StartSession begins a new debug session with the given symptom description.
func (dr *DebugRecorder) StartSession(symptom string) *DebugSession {
	dr.mu.Lock()
	defer dr.mu.Unlock()

	session := &DebugSession{
		ID:                fmt.Sprintf("dbg_%d", time.Now().UnixNano()),
		StartTime:         time.Now(),
		Symptom:           symptom,
		Steps:             make([]DebugStep, 0),
		FilesInvestigated: make([]string, 0),
		HypothesesTested:  make([]Hypothesis, 0),
	}

	dr.ActiveSession = session
	dr.Sessions = append(dr.Sessions, session)
	return session
}

// RecordStep adds a step to the active debug session.
func (dr *DebugRecorder) RecordStep(action, target, result, insight string) {
	dr.mu.Lock()
	defer dr.mu.Unlock()

	if dr.ActiveSession == nil {
		return
	}

	step := DebugStep{
		Index:         len(dr.ActiveSession.Steps) + 1,
		Action:        action,
		Target:        target,
		Result:        result,
		Timestamp:     time.Now(),
		InsightGained: insight,
	}

	dr.ActiveSession.Steps = append(dr.ActiveSession.Steps, step)

	// Track files investigated for read/grep actions
	if action == "read" || action == "grep" {
		found := false
		for _, f := range dr.ActiveSession.FilesInvestigated {
			if f == target {
				found = true
				break
			}
		}
		if !found {
			dr.ActiveSession.FilesInvestigated = append(dr.ActiveSession.FilesInvestigated, target)
		}
	}
}

// AddHypothesis adds a new hypothesis to the active debug session.
func (dr *DebugRecorder) AddHypothesis(description string) {
	dr.mu.Lock()
	defer dr.mu.Unlock()

	if dr.ActiveSession == nil {
		return
	}

	h := Hypothesis{
		Description: description,
		Tested:      false,
		Confirmed:   false,
	}

	dr.ActiveSession.HypothesesTested = append(dr.ActiveSession.HypothesesTested, h)

	// Also record as a step
	step := DebugStep{
		Index:     len(dr.ActiveSession.Steps) + 1,
		Action:    "hypothesis",
		Target:    description,
		Result:    "proposed",
		Timestamp: time.Now(),
	}
	dr.ActiveSession.Steps = append(dr.ActiveSession.Steps, step)
}

// ConfirmHypothesis marks a hypothesis as tested and confirmed with evidence.
func (dr *DebugRecorder) ConfirmHypothesis(index int, evidence string) {
	dr.mu.Lock()
	defer dr.mu.Unlock()

	if dr.ActiveSession == nil {
		return
	}

	if index < 0 || index >= len(dr.ActiveSession.HypothesesTested) {
		return
	}

	dr.ActiveSession.HypothesesTested[index].Tested = true
	dr.ActiveSession.HypothesesTested[index].Confirmed = true
	dr.ActiveSession.HypothesesTested[index].Evidence = evidence
}

// RejectHypothesis marks a hypothesis as tested and rejected with evidence.
func (dr *DebugRecorder) RejectHypothesis(index int, evidence string) {
	dr.mu.Lock()
	defer dr.mu.Unlock()

	if dr.ActiveSession == nil {
		return
	}

	if index < 0 || index >= len(dr.ActiveSession.HypothesesTested) {
		return
	}

	dr.ActiveSession.HypothesesTested[index].Tested = true
	dr.ActiveSession.HypothesesTested[index].Confirmed = false
	dr.ActiveSession.HypothesesTested[index].Evidence = evidence
}

// SetRootCause records the root cause for the active debug session.
func (dr *DebugRecorder) SetRootCause(cause string) {
	dr.mu.Lock()
	defer dr.mu.Unlock()

	if dr.ActiveSession == nil {
		return
	}

	dr.ActiveSession.RootCause = cause
}

// SetResolution records the resolution for the active debug session.
func (dr *DebugRecorder) SetResolution(resolution string) {
	dr.mu.Lock()
	defer dr.mu.Unlock()

	if dr.ActiveSession == nil {
		return
	}

	dr.ActiveSession.Resolution = resolution
}

// EndSession ends the active debug session and marks it as successful or not.
func (dr *DebugRecorder) EndSession(successful bool) {
	dr.mu.Lock()
	defer dr.mu.Unlock()

	if dr.ActiveSession == nil {
		return
	}

	now := time.Now()
	dr.ActiveSession.EndTime = &now
	dr.ActiveSession.Successful = successful
	dr.ActiveSession = nil
}

// FormatSession produces a human-readable summary of a debug session.
func (dr *DebugRecorder) FormatSession(session *DebugSession) string {
	if session == nil {
		return ""
	}

	var sb strings.Builder

	// Header
	status := "UNRESOLVED"
	if session.Successful {
		status = "RESOLVED"
	}

	duration := "in progress"
	if session.EndTime != nil {
		d := session.EndTime.Sub(session.StartTime)
		minutes := int(d.Minutes())
		seconds := int(d.Seconds()) % 60
		if minutes > 0 {
			duration = fmt.Sprintf("%dm %ds", minutes, seconds)
		} else {
			duration = fmt.Sprintf("%ds", seconds)
		}
	}

	sb.WriteString(fmt.Sprintf("Debug Session: %q\n", session.Symptom))
	sb.WriteString(fmt.Sprintf("Duration: %s | Status: %s\n", duration, status))
	sb.WriteString(strings.Repeat("─", 41) + "\n")
	sb.WriteString("\n")

	// Symptom
	sb.WriteString(fmt.Sprintf("Symptom: %s\n", session.Symptom))
	sb.WriteString("\n")

	// Investigation steps
	if len(session.Steps) > 0 {
		sb.WriteString("Investigation:\n")
		for _, step := range session.Steps {
			line := fmt.Sprintf("%d. [%s] %s", step.Index, step.Action, step.Target)

			if step.Action == "hypothesis" {
				// Find matching hypothesis
				for _, h := range session.HypothesesTested {
					if h.Description == step.Target {
						if h.Tested {
							if h.Confirmed {
								line += " → CONFIRMED"
								if h.Evidence != "" {
									line += fmt.Sprintf("\n   Evidence: %s", h.Evidence)
								}
							} else {
								line += " → REJECTED"
							}
						} else {
							line += " → UNTESTED"
						}
						break
					}
				}
			} else {
				if step.Result != "" {
					line += fmt.Sprintf(" → %s", step.Result)
				}
			}

			sb.WriteString(line + "\n")
		}
		sb.WriteString("\n")
	}

	// Root cause
	if session.RootCause != "" {
		sb.WriteString(fmt.Sprintf("Root Cause: %s\n", session.RootCause))
	}

	// Resolution
	if session.Resolution != "" {
		sb.WriteString(fmt.Sprintf("Resolution: %s\n", session.Resolution))
	}

	// Files
	if len(session.FilesInvestigated) > 0 {
		sb.WriteString("\n")
		sb.WriteString(fmt.Sprintf("Files: %s\n", strings.Join(session.FilesInvestigated, ", ")))
	}

	return sb.String()
}

// SearchSessions finds past sessions with symptoms similar to the given symptom.
func (dr *DebugRecorder) SearchSessions(symptom string) []*DebugSession {
	dr.mu.Lock()
	defer dr.mu.Unlock()

	symptomLower := strings.ToLower(symptom)
	words := strings.Fields(symptomLower)

	var results []*DebugSession

	for _, session := range dr.Sessions {
		sessionSymptomLower := strings.ToLower(session.Symptom)

		// Check for direct substring match
		if strings.Contains(sessionSymptomLower, symptomLower) {
			results = append(results, session)
			continue
		}

		// Check word overlap
		matchCount := 0
		for _, word := range words {
			if len(word) < 3 {
				continue
			}
			if strings.Contains(sessionSymptomLower, word) {
				matchCount++
			}
		}

		// Also check root cause and resolution
		rootCauseLower := strings.ToLower(session.RootCause)
		resolutionLower := strings.ToLower(session.Resolution)
		for _, word := range words {
			if len(word) < 3 {
				continue
			}
			if strings.Contains(rootCauseLower, word) || strings.Contains(resolutionLower, word) {
				matchCount++
			}
		}

		// Require at least 1 word match in significant words
		significantWords := 0
		for _, w := range words {
			if len(w) >= 3 {
				significantWords++
			}
		}
		if significantWords > 0 && matchCount >= 1 {
			results = append(results, session)
		}
	}

	return results
}

// BuildDebugContext formats relevant past sessions as context for the agent.
func (dr *DebugRecorder) BuildDebugContext(symptom string) string {
	matches := dr.SearchSessions(symptom)
	if len(matches) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("=== Relevant Past Debug Sessions ===\n\n")

	// Limit to most recent 5 matches
	limit := 5
	if len(matches) < limit {
		limit = len(matches)
	}

	for i := 0; i < limit; i++ {
		session := matches[len(matches)-1-i] // most recent first
		sb.WriteString(dr.FormatSession(session))
		sb.WriteString("\n---\n\n")
	}

	return sb.String()
}

// Save persists all sessions to disk as JSON.
func (dr *DebugRecorder) Save() error {
	dr.mu.Lock()
	defer dr.mu.Unlock()

	if dr.Dir == "" {
		return fmt.Errorf("debug recorder: no directory configured")
	}

	if err := os.MkdirAll(dr.Dir, 0o755); err != nil {
		return fmt.Errorf("debug recorder: create dir: %w", err)
	}

	data, err := json.MarshalIndent(dr.Sessions, "", "  ")
	if err != nil {
		return fmt.Errorf("debug recorder: marshal: %w", err)
	}

	path := filepath.Join(dr.Dir, "debug_sessions.json")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("debug recorder: write: %w", err)
	}

	return nil
}

// Load reads previously persisted sessions from disk.
func (dr *DebugRecorder) Load() error {
	dr.mu.Lock()
	defer dr.mu.Unlock()

	if dr.Dir == "" {
		return fmt.Errorf("debug recorder: no directory configured")
	}

	path := filepath.Join(dr.Dir, "debug_sessions.json")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // No sessions file yet, not an error
		}
		return fmt.Errorf("debug recorder: read: %w", err)
	}

	var sessions []*DebugSession
	if err := json.Unmarshal(data, &sessions); err != nil {
		return fmt.Errorf("debug recorder: unmarshal: %w", err)
	}

	dr.Sessions = sessions
	return nil
}
