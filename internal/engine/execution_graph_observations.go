package engine

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	eyrieengine "github.com/GrayCodeAI/eyrie/engine"
	graphcontracts "github.com/GrayCodeAI/hawk-core-contracts/graph"
	policycontracts "github.com/GrayCodeAI/hawk-core-contracts/policy"
	"github.com/GrayCodeAI/hawk/internal/graphjournal"
	"github.com/GrayCodeAI/hawk/internal/types"
	"github.com/GrayCodeAI/tok"
	tokgraph "github.com/GrayCodeAI/tok/runtimegraph"
)

func (s *Session) recordPolicyObservation(tc types.ToolCall, stage string, allowed bool, reason string) {
	sessionID := s.executionGraphSessionID()
	if sessionID == "" {
		return
	}
	verdict := policycontracts.Allow(reason)
	verdict.Rule = strings.TrimSpace(stage)
	verdict.Source = "hawk." + strings.TrimSpace(stage)
	if !allowed {
		verdict = policycontracts.Deny(reason, strings.TrimSpace(stage))
		verdict.Source = "hawk." + strings.TrimSpace(stage)
	}
	if err := graphjournal.AppendPolicy(sessionID, tc.ID, stage, verdict, time.Now()); err != nil {
		s.log.Warn("graph observation append failed", map[string]interface{}{
			"kind":  graphjournal.KindPolicy,
			"stage": stage,
		})
	}
}

func (s *Session) recordVerificationObservation(tc types.ToolCall, output string, isErr bool) {
	if canonicalToolName(tc.Name) != "VerifyPlanExecution" {
		return
	}
	sessionID := s.executionGraphSessionID()
	if sessionID == "" {
		return
	}

	var result struct {
		AllVerified bool `json:"allVerified"`
		TotalSteps  int  `json:"totalSteps"`
		Verified    int  `json:"verified"`
	}
	failed := isErr
	findingCount := 0
	if !isErr {
		if err := json.Unmarshal([]byte(output), &result); err != nil {
			failed = true
			findingCount = 1
		} else {
			failed = !result.AllVerified
			findingCount = result.TotalSteps - result.Verified
			if findingCount < 0 {
				findingCount = 0
			}
		}
	} else {
		findingCount = 1
	}

	maxSeverity := "info"
	if failed {
		maxSeverity = "medium"
	}
	if err := graphjournal.AppendVerification(
		sessionID,
		tc.ID,
		"verify-plan-execution",
		failed,
		findingCount,
		maxSeverity,
		"plan-execution",
		time.Now(),
	); err != nil {
		s.log.Warn("graph observation append failed", map[string]interface{}{
			"kind":  graphjournal.KindVerify,
			"stage": "verify-plan-execution",
		})
	}
}

func (s *Session) executionGraphSessionID() string {
	if s == nil {
		return ""
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return strings.TrimSpace(s.persistID)
}

// ConfigureContextGraphObservation binds Yaad recall projections to this
// persisted Hawk session. It is safe to call before either side is configured.
func (s *Session) ConfigureContextGraphObservation(repositoryDir string) {
	if s == nil || s.MemorySvc() == nil || s.MemorySvc().Yaad() == nil {
		return
	}
	if strings.TrimSpace(repositoryDir) == "" {
		repositoryDir, _ = os.Getwd()
	}
	repositoryID := ""
	if strings.TrimSpace(repositoryDir) != "" {
		repositoryID = filepath.Base(filepath.Clean(repositoryDir))
	}
	s.MemorySvc().Yaad().ConfigureGraphObservation(
		s.executionGraphSessionID(),
		graphcontracts.Scope{RepositoryID: repositoryID},
	)
}

func (s *Session) recordTokCompressionObservation(source, stage string, stats tok.Stats) {
	sessionID := s.executionGraphSessionID()
	if sessionID == "" || stats.OriginalTokens <= 0 {
		return
	}
	s.mu.RLock()
	repositoryDir := strings.TrimSpace(s.workingDir)
	s.mu.RUnlock()
	if repositoryDir == "" {
		repositoryDir, _ = os.Getwd()
	}
	repositoryID := ""
	if repositoryDir != "" {
		repositoryID = filepath.Base(filepath.Clean(repositoryDir))
	}
	observedAt := time.Now().UTC()
	export, err := tokgraph.Build(tokgraph.Input{
		Compression:   &stats,
		Source:        source,
		ObservedAt:    observedAt,
		Scope:         graphcontracts.Scope{RepositoryID: repositoryID},
		CorrelationID: sessionID,
	})
	if err == nil {
		err = graphjournal.AppendRuntimeGraph(
			sessionID, "", stage, "tok",
			export.Nodes, export.Edges, export.Events, observedAt,
		)
	}
	if err != nil {
		s.log.Warn("graph observation append failed", map[string]interface{}{
			"kind":  graphjournal.KindRuntime,
			"stage": stage,
		})
	}
}

func (s *Session) recordTokRedactionObservation(source string, matchCount int, types map[string]int) {
	sessionID := s.executionGraphSessionID()
	if sessionID == "" || matchCount <= 0 {
		return
	}
	s.mu.RLock()
	repositoryDir := strings.TrimSpace(s.workingDir)
	s.mu.RUnlock()
	if repositoryDir == "" {
		repositoryDir, _ = os.Getwd()
	}
	repositoryID := ""
	if repositoryDir != "" {
		repositoryID = filepath.Base(filepath.Clean(repositoryDir))
	}
	observedAt := time.Now().UTC()
	export, err := tokgraph.Build(tokgraph.Input{
		Redaction: &tokgraph.RedactionSummary{
			MatchCount: matchCount,
			Types:      types,
		},
		Source:        source,
		ObservedAt:    observedAt,
		Scope:         graphcontracts.Scope{RepositoryID: repositoryID},
		CorrelationID: sessionID,
	})
	if err == nil {
		err = graphjournal.AppendRuntimeGraph(
			sessionID, "", "response-redaction", "tok",
			export.Nodes, export.Edges, export.Events, observedAt,
		)
	}
	if err != nil {
		s.log.Warn("graph observation append failed", map[string]interface{}{
			"kind":  graphjournal.KindRuntime,
			"stage": "response-redaction",
		})
	}
}

func (s *Session) recordTokUsageBudgetObservation(
	tokens int,
	costUSD float64,
	provider, model string,
) {
	if s == nil || tokens <= 0 {
		return
	}
	tracker := s.ensureTokUsageTracker()
	tracker.Record(tokens, costUSD, provider, model)
	allowed, reason := tracker.CanProceed()
	usage := tracker.GetUsage()
	limits := tracker.GetLimits()

	sessionID := s.executionGraphSessionID()
	if sessionID == "" {
		return
	}
	s.mu.RLock()
	repositoryDir := strings.TrimSpace(s.workingDir)
	s.mu.RUnlock()
	if repositoryDir == "" {
		repositoryDir, _ = os.Getwd()
	}
	repositoryID := ""
	if repositoryDir != "" {
		repositoryID = filepath.Base(filepath.Clean(repositoryDir))
	}

	observedAt := time.Now().UTC()
	export, err := tokgraph.Build(tokgraph.Input{
		Usage: &usage,
		Budget: &tokgraph.BudgetDecision{
			Allowed:      allowed,
			Reason:       reason,
			HourlyLimit:  limits.HourlyTokens,
			DailyLimit:   limits.DailyTokens,
			SessionLimit: limits.SessionTokens,
			CostLimitUSD: limits.CostUSD,
		},
		Source:          provider + "\x00" + model,
		ObservedAt:      observedAt,
		Scope:           graphcontracts.Scope{RepositoryID: repositoryID},
		CorrelationID:   sessionID,
		ProducerVersion: "",
	})
	if err == nil {
		err = graphjournal.AppendRuntimeGraph(
			sessionID, "", "usage-budget", "tok",
			export.Nodes, export.Edges, export.Events, observedAt,
		)
	}
	if err != nil {
		s.log.Warn("graph observation append failed", map[string]interface{}{
			"kind":  graphjournal.KindRuntime,
			"stage": "usage-budget",
		})
	}
}

func (s *Session) ensureTokUsageTracker() *tok.UsageTracker {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.tokUsage == nil {
		// Default: token ceilings off (provider rate limits own throughput).
		// tok.NewUsageTracker ships non-zero defaults; explicitly disable them
		// so a fresh session doesn't fire usage-alerts. Budget caps are opt-in
		// via SetMaxBudgetUSD, which writes CostUSD into this tracker.
		s.tokUsage = tok.NewUsageTracker()
		s.tokUsage.SetLimits(tok.UsageLimits{})
	}
	return s.tokUsage
}

func (s *Session) currentTokUsageTracker() *tok.UsageTracker {
	if s == nil {
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.tokUsage
}

func (s *Session) tokUsageCanProceed() (bool, string) {
	tracker := s.currentTokUsageTracker()
	if tracker == nil {
		return true, ""
	}
	return tracker.CanProceed()
}

func (s *Session) recordEyrieOperationObservation(
	provider, model, finishReason, content string,
	toolCallCount int,
	usage *types.EyrieUsage,
) {
	sessionID := s.executionGraphSessionID()
	if sessionID == "" || usage == nil {
		return
	}
	s.mu.RLock()
	repositoryDir := strings.TrimSpace(s.workingDir)
	s.mu.RUnlock()
	if repositoryDir == "" {
		repositoryDir, _ = os.Getwd()
	}
	repositoryID := ""
	if repositoryDir != "" {
		repositoryID = filepath.Base(filepath.Clean(repositoryDir))
	}
	observedAt := time.Now().UTC()
	route := types.ResolvedRoute{Provider: provider, Model: model}
	export, err := eyrieengine.BuildOperationsGraph(eyrieengine.OperationsGraphInput{
		Route:         &route,
		Usage:         usage,
		FinishReason:  finishReason,
		Content:       content,
		ToolCallCount: toolCallCount,
		ObservedAt:    observedAt,
		Scope:         graphcontracts.Scope{RepositoryID: repositoryID},
		CorrelationID: sessionID,
	})
	if err == nil {
		err = graphjournal.AppendRuntimeGraph(
			sessionID, "", "model-generation", "eyrie",
			export.Nodes, export.Edges, export.Events, observedAt,
		)
	}
	if err != nil {
		s.log.Warn("graph observation append failed", map[string]interface{}{
			"kind":  graphjournal.KindRuntime,
			"stage": "model-generation",
		})
	}
}
