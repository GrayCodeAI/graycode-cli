package memory

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
)

// EnhancedMemoryManager extends MemoryManager with all the new subsystems:
// auto-capture, proactive context, confidence tracking, retrieval metrics,
// code-memory links, session diff, continuity scoring, cross-project memory,
// and shared memory for mission mode.
type EnhancedMemoryManager struct {
	*MemoryManager

	AutoCapture  *AutoCapture
	Proactive    *ProactiveContext
	Confidence   *ConfidenceTracker
	Retrieval    *RetrievalMetrics
	CodeLinks    *CodeMemoryLinker
	SessionDiff  *SessionDiffAnalyzer
	Continuity   *ContinuityTracker
	CrossProject *CrossProjectMemory
	GraphBudget  *GraphAwareBudget
	Shared       *SharedMemory // nil unless in mission mode

	sessionID string
	mu        sync.Mutex
}

// NewEnhancedMemoryManager creates a fully-integrated memory system with all subsystems.
func NewEnhancedMemoryManager(projectDir string) *EnhancedMemoryManager {
	base := NewMemoryManager(projectDir)

	em := &EnhancedMemoryManager{
		MemoryManager: base,
	}

	// Initialize all subsystems once the yaad bridge is ready
	if base.Yaad.Ready() {
		em.AutoCapture = NewAutoCapture(base.Yaad)
		em.Proactive = NewProactiveContext(base.Yaad)
		em.Confidence = NewConfidenceTracker(base.Yaad)
		em.Retrieval = NewRetrievalMetrics(projectDir)
		em.CodeLinks = NewCodeMemoryLinker(base.Yaad)
		em.SessionDiff = NewSessionDiffAnalyzer(base.Yaad, projectDir)
		em.Continuity = NewContinuityTracker(projectDir)
		em.CrossProject = NewCrossProjectMemory(base.Yaad)
		em.GraphBudget = NewGraphAwareBudget(base.Yaad, em.Proactive)
	}

	return em
}

// StartSession initializes all session-level tracking.
func (em *EnhancedMemoryManager) StartSession(sessionID string) {
	em.mu.Lock()
	defer em.mu.Unlock()

	em.sessionID = sessionID

	if em.Confidence != nil {
		em.Confidence.Reset()
	}
	if em.Proactive != nil {
		em.Proactive.Reset()
	}
	if em.SessionDiff != nil {
		em.SessionDiff.SnapshotStart()
	}
	if em.Continuity != nil {
		memoryInjected := em.Yaad.Ready()
		em.Continuity.StartSession(sessionID, memoryInjected)
	}
}

// EndSession performs end-of-session processing: diff analysis, confidence
// updates, metric persistence, and continuity scoring.
func (em *EnhancedMemoryManager) EndSession(success bool) {
	em.mu.Lock()
	defer em.mu.Unlock()

	// Confidence adjustments based on outcome
	if em.Confidence != nil {
		if success {
			em.Confidence.OnSessionSuccess()
		} else {
			em.Confidence.OnSessionFailure()
		}
	}

	// Session diff analysis → extract and store new memories
	if em.SessionDiff != nil {
		diff := em.SessionDiff.AnalyzeEnd()
		if diff != nil {
			em.SessionDiff.StoreMemoriesFromDiff(diff)
		}
	}

	// Record continuity
	if em.Continuity != nil {
		em.Continuity.EndSession(success)
	}

	// Persist retrieval metrics
	if em.Retrieval != nil {
		em.Retrieval.Save()
	}
}

// Recall performs an enhanced recall that tracks metrics and confidence.
// Implements engine.MemoryRecaller interface.
func (em *EnhancedMemoryManager) Recall(query string, tokenBudget int) (string, error) {
	// Use graph-aware budget if available
	if em.GraphBudget != nil {
		var activeFiles []string
		if em.Proactive != nil {
			em.Proactive.mu.Lock()
			for f := range em.Proactive.activeFiles {
				activeFiles = append(activeFiles, f)
			}
			em.Proactive.mu.Unlock()
		}

		injection := em.GraphBudget.BuildInjection(query, activeFiles, tokenBudget)
		if injection != "" {
			// Track metrics
			if em.Retrieval != nil {
				resultCount := strings.Count(injection, "\n")
				tokensUsed := len(injection) / 4
				em.Retrieval.RecordRecall(query, resultCount, tokensUsed, "graph_budget")
			}
			if em.Continuity != nil {
				em.Continuity.RecordMemoryUse(1, len(injection)/4)
			}
			return injection, nil
		}
	}

	// Fall back to base recall (MemoryManager.Recall)
	result, err := em.MemoryManager.Recall(query, tokenBudget)
	if err != nil {
		return "", err
	}

	// Track metrics
	if em.Retrieval != nil {
		resultCount := 0
		if result != "" {
			resultCount = strings.Count(result, "\n") + 1
		}
		em.Retrieval.RecordRecall(query, resultCount, len(result)/4, "base")
	}
	if em.Continuity != nil && result != "" {
		em.Continuity.RecordMemoryUse(1, len(result)/4)
	}

	return result, nil
}

// Remember stores memory and routes through auto-capture pipeline.
// Implements engine.MemoryRecaller interface.
func (em *EnhancedMemoryManager) Remember(content, category string) error {
	err := em.MemoryManager.Remember(content, category)
	if err != nil {
		return err
	}

	// Also check if this should be promoted to global scope
	if em.CrossProject != nil {
		promoted := em.CrossProject.DetectGlobalPatterns([]string{content})
		if len(promoted) > 0 {
			for _, p := range promoted {
				_ = em.CrossProject.StoreGlobal(p, "preference")
			}
		}
	}

	return nil
}

// OnToolResult processes a tool result for auto-capture.
func (em *EnhancedMemoryManager) OnToolResult(toolName string, args map[string]interface{}, output string, isErr bool) {
	if em.AutoCapture != nil {
		em.AutoCapture.Ingest(toolName, args, output, isErr)
	}

	// Track file interactions for proactive context
	if em.Proactive != nil {
		if path, ok := extractPath(args); ok && path != "" {
			em.Proactive.TrackFile(path)
		}
	}

	// Link code index to memories on file writes
	if em.CodeLinks != nil {
		cn := canonicalName(toolName)
		if cn == "Write" || cn == "Edit" {
			if path, ok := extractPath(args); ok {
				go func() { _ = em.CodeLinks.LinkFileToMemories(path) }()
			}
		}
	}
}

// ProactiveContextForFile returns memories relevant to a file being worked on.
func (em *EnhancedMemoryManager) ProactiveContextForFile(path string) string {
	if em.Proactive == nil {
		return ""
	}
	return em.Proactive.ContextForFile(path)
}

// GlobalContext returns cross-project user preferences for injection.
func (em *EnhancedMemoryManager) GlobalContext(budget int) string {
	if em.CrossProject == nil {
		return ""
	}
	return em.CrossProject.InjectGlobalContext(budget)
}

// EnableMissionMode activates shared memory for parallel agent coordination.
func (em *EnhancedMemoryManager) EnableMissionMode(missionID, agentID string) {
	em.mu.Lock()
	defer em.mu.Unlock()
	em.Shared = NewSharedMemory(em.Yaad, missionID, agentID)
}

// ShareWithMission stores a memory visible to all agents in the mission.
func (em *EnhancedMemoryManager) ShareWithMission(content, nodeType string) error {
	if em.Shared == nil {
		return nil
	}
	return em.Shared.Share(content, nodeType)
}

// RecallFromMission retrieves shared memories from the mission namespace.
func (em *EnhancedMemoryManager) RecallFromMission(query string, budget int) (string, error) {
	if em.Shared == nil {
		return "", nil
	}
	return em.Shared.Recall(query, budget)
}

// FormatForPrompt builds the full memory context for prompt injection,
// combining all sources: graph budget, global, proactive, and mission.
func (em *EnhancedMemoryManager) FormatForPrompt() string {
	var sections []string

	// Base format (auto, evolving, zen)
	if s := em.MemoryManager.FormatForPrompt(); s != "" {
		sections = append(sections, s)
	}

	// Global user preferences
	if globalCtx := em.GlobalContext(300); globalCtx != "" {
		sections = append(sections, globalCtx)
	}

	// Mission shared memory
	if em.Shared != nil {
		if missionCtx, err := em.RecallFromMission("", 500); err == nil && missionCtx != "" {
			sections = append(sections, missionCtx)
		}
	}

	return strings.Join(sections, "\n\n")
}

// StatusSummary returns a concise status line for the memory system.
func (em *EnhancedMemoryManager) StatusSummary() string {
	var parts []string

	if em.Retrieval != nil {
		if s := em.Retrieval.FormatSummary(); s != "" {
			parts = append(parts, s)
		}
	}
	if em.Continuity != nil {
		if s := em.Continuity.FormatSummary(); s != "" {
			parts = append(parts, s)
		}
	}
	if em.AutoCapture != nil {
		m := em.AutoCapture.Metrics()
		if m.Captured > 0 {
			parts = append(parts, fmt.Sprintf("Auto-captured: %d", m.Captured))
		}
	}

	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, " | ")
}

// Close shuts down all subsystems gracefully.
func (em *EnhancedMemoryManager) Close() {
	if em.AutoCapture != nil {
		em.AutoCapture.Stop()
	}
	if em.Retrieval != nil {
		em.Retrieval.Save()
	}
	if em.Continuity != nil {
		em.Continuity.Save()
	}
	em.Yaad.Close()
}

// HealthCheck verifies the memory system is working correctly.
func (em *EnhancedMemoryManager) HealthCheck() map[string]interface{} {
	health := map[string]interface{}{
		"yaad_ready": em.Yaad.Ready(),
		"timestamp":  time.Now().Format(time.RFC3339),
	}

	if em.Retrieval != nil {
		health["hit_rate"] = em.Retrieval.HitRate()
		health["total_recalls"] = em.Retrieval.TotalRecalls()
	}
	if em.AutoCapture != nil {
		m := em.AutoCapture.Metrics()
		health["auto_captured"] = m.Captured
		health["auto_skipped"] = m.Skipped
	}
	if em.Continuity != nil {
		r := em.Continuity.Report()
		health["continuity_score"] = r.AvgScore
		health["tokens_saved"] = r.TotalTokensSaved
	}
	if em.Confidence != nil {
		health["memories_accessed"] = em.Confidence.AccessedCount()
	}

	return health
}

// DiagnosticReport generates a detailed diagnostic for troubleshooting.
func (em *EnhancedMemoryManager) DiagnosticReport(_ context.Context) string {
	var sb strings.Builder
	sb.WriteString("=== Memory System Diagnostic ===\n\n")

	// Core status
	sb.WriteString(fmt.Sprintf("Yaad Bridge: %v\n", em.Yaad.Ready()))

	// Retrieval metrics
	if em.Retrieval != nil {
		r := em.Retrieval.Report()
		sb.WriteString("\nRetrieval:\n")
		sb.WriteString(fmt.Sprintf("  Total recalls: %d\n", r.TotalRecalls))
		sb.WriteString(fmt.Sprintf("  Hit rate: %.1f%%\n", r.HitRate*100))
		sb.WriteString(fmt.Sprintf("  Avg results: %.1f\n", r.AvgResultCount))
		sb.WriteString(fmt.Sprintf("  Tokens saved: %d\n", r.TotalTokensSaved))
	}

	// Auto-capture metrics
	if em.AutoCapture != nil {
		m := em.AutoCapture.Metrics()
		sb.WriteString("\nAuto-Capture:\n")
		sb.WriteString(fmt.Sprintf("  Captured: %d\n", m.Captured))
		sb.WriteString(fmt.Sprintf("  Skipped: %d\n", m.Skipped))
		sb.WriteString(fmt.Sprintf("  Conventions: %d\n", m.ConventionsOut))
		sb.WriteString(fmt.Sprintf("  Decisions: %d\n", m.DecisionsOut))
		sb.WriteString(fmt.Sprintf("  Bugs: %d\n", m.BugsOut))
	}

	// Continuity
	if em.Continuity != nil {
		r := em.Continuity.Report()
		sb.WriteString("\nContinuity:\n")
		sb.WriteString(fmt.Sprintf("  Sessions tracked: %d\n", r.TotalSessions))
		sb.WriteString(fmt.Sprintf("  Avg score: %.0f/100\n", r.AvgScore))
		sb.WriteString(fmt.Sprintf("  Memory contribution: %.1f%%\n", r.MemoryContribution*100))
	}

	return sb.String()
}
