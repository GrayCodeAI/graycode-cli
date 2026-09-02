package engine

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	graphcontracts "github.com/GrayCodeAI/eagle/graph"
	policycontracts "github.com/GrayCodeAI/eagle/policy"
	eyrieengine "github.com/GrayCodeAI/eyrie/engine"
	eyriegraph "github.com/GrayCodeAI/eyrie/graph"
	"github.com/GrayCodeAI/hawk/internal/engine/token"
	"github.com/GrayCodeAI/hawk/internal/graphjournal"
	"github.com/GrayCodeAI/hawk/internal/types"
	shrikegraph "github.com/GrayCodeAI/shrike/graph"
)

func (s *Session) recordPolicyObservation(tc types.ToolCall, stage string, allowed bool, reason string) {
	// Tamper-evident security event log: record every denial regardless of
	// whether graph observation is available, so enforcement history is
	// auditable even when the session transcript is gone.
	if !allowed {
		s.recordSecurityDenial(tc, stage, reason)
	}
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
		s.Logger().Warn("graph observation append failed", map[string]interface{}{
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
		s.Logger().Warn("graph observation append failed", map[string]interface{}{
			"kind":  graphjournal.KindVerify,
			"stage": "verify-plan-execution",
		})
	}
}

func (s *Session) executionGraphSessionID() string {
	if s == nil {
		return ""
	}
	if p := s.Persistence(); p != nil {
		return strings.TrimSpace(p.PersistID())
	}
	return ""
}

func (s *Session) configuredWorkingDir() string {
	if s == nil || s.Tools() == nil {
		return ""
	}
	return strings.TrimSpace(s.Tools().WorkingDir())
}

// SessionID returns the persistence ID of this session, or "" before one is
// assigned. Used by lifecycle bookkeeping to attribute cost entries to the
// real session instead of fabricated IDs.
func (s *Session) SessionID() string {
	return s.executionGraphSessionID()
}

// ConfigureContextGraphObservation binds Harrier recall projections to this
// persisted Hawk session. It is safe to call before either side is configured.
func (s *Session) ConfigureContextGraphObservation(repositoryDir string) {
	if s == nil || s.MemorySvc() == nil || s.MemorySvc().Harrier() == nil {
		return
	}
	if strings.TrimSpace(repositoryDir) == "" {
		repositoryDir, _ = os.Getwd()
	}
	repositoryID := ""
	if strings.TrimSpace(repositoryDir) != "" {
		repositoryID = filepath.Base(filepath.Clean(repositoryDir))
	}
	s.MemorySvc().Harrier().ConfigureGraphObservation(
		s.executionGraphSessionID(),
		graphcontracts.Scope{RepositoryID: repositoryID},
	)
}

func (s *Session) recordShrikeCompressionObservation(source, stage string, stats token.Stats) {
	sessionID := s.executionGraphSessionID()
	if sessionID == "" || stats.OriginalTokens <= 0 {
		return
	}
	repositoryDir := s.configuredWorkingDir()
	if repositoryDir == "" {
		repositoryDir, _ = os.Getwd()
	}
	repositoryID := ""
	if repositoryDir != "" {
		repositoryID = filepath.Base(filepath.Clean(repositoryDir))
	}
	observedAt := time.Now().UTC()
	export, err := token.BuildRuntimeGraph(token.RuntimeGraphInput{
		Compression:   &stats,
		Source:        source,
		ObservedAt:    observedAt,
		Scope:         shrikegraph.Scope{RepositoryID: repositoryID},
		CorrelationID: sessionID,
	})
	if err == nil {
		err = graphjournal.AppendRuntimeGraph(
			sessionID, "", stage, "shrike",
			shrikeToEagleNodes(export.Nodes), shrikeToEagleEdges(export.Edges), shrikeToEagleEvents(export.Events), observedAt,
		)
	}
	if err != nil {
		s.Logger().Warn("graph observation append failed", map[string]interface{}{
			"kind":  graphjournal.KindRuntime,
			"stage": stage,
		})
	}
}

func (s *Session) recordShrikeRedactionObservation(source string, matchCount int, types map[string]int) {
	sessionID := s.executionGraphSessionID()
	if sessionID == "" || matchCount <= 0 {
		return
	}
	repositoryDir := s.configuredWorkingDir()
	if repositoryDir == "" {
		repositoryDir, _ = os.Getwd()
	}
	repositoryID := ""
	if repositoryDir != "" {
		repositoryID = filepath.Base(filepath.Clean(repositoryDir))
	}
	observedAt := time.Now().UTC()
	export, err := token.BuildRuntimeGraph(token.RuntimeGraphInput{
		Redaction: &token.RedactionSummary{
			MatchCount: matchCount,
			Types:      types,
		},
		Source:        source,
		ObservedAt:    observedAt,
		Scope:         shrikegraph.Scope{RepositoryID: repositoryID},
		CorrelationID: sessionID,
	})
	if err == nil {
		err = graphjournal.AppendRuntimeGraph(
			sessionID, "", "response-redaction", "shrike",
			shrikeToEagleNodes(export.Nodes), shrikeToEagleEdges(export.Edges), shrikeToEagleEvents(export.Events), observedAt,
		)
	}
	if err != nil {
		s.Logger().Warn("graph observation append failed", map[string]interface{}{
			"kind":  graphjournal.KindRuntime,
			"stage": "response-redaction",
		})
	}
}

func (s *Session) recordShrikeUsageBudgetObservation(
	tokens int,
	costUSD float64,
	provider, model string,
) {
	if s == nil || tokens <= 0 {
		return
	}
	tracker := s.ensureShrikeUsageTracker()
	tracker.Record(tokens, costUSD, provider, model)
	allowed, reason := tracker.CanProceed()
	usage := tracker.GetUsage()
	limits := tracker.GetLimits()

	sessionID := s.executionGraphSessionID()
	if sessionID == "" {
		return
	}
	repositoryDir := s.configuredWorkingDir()
	if repositoryDir == "" {
		repositoryDir, _ = os.Getwd()
	}
	repositoryID := ""
	if repositoryDir != "" {
		repositoryID = filepath.Base(filepath.Clean(repositoryDir))
	}

	observedAt := time.Now().UTC()
	export, err := token.BuildRuntimeGraph(token.RuntimeGraphInput{
		Usage: &usage,
		Budget: &token.BudgetDecision{
			Allowed:      allowed,
			Reason:       reason,
			HourlyLimit:  limits.HourlyTokens,
			DailyLimit:   limits.DailyTokens,
			SessionLimit: limits.SessionTokens,
			CostLimitUSD: limits.CostUSD,
		},
		Source:          provider + "\x00" + model,
		ObservedAt:      observedAt,
		Scope:           shrikegraph.Scope{RepositoryID: repositoryID},
		CorrelationID:   sessionID,
		ProducerVersion: "",
	})
	if err == nil {
		err = graphjournal.AppendRuntimeGraph(
			sessionID, "", "usage-budget", "shrike",
			shrikeToEagleNodes(export.Nodes), shrikeToEagleEdges(export.Edges), shrikeToEagleEvents(export.Events), observedAt,
		)
	}
	if err != nil {
		s.Logger().Warn("graph observation append failed", map[string]interface{}{
			"kind":  graphjournal.KindRuntime,
			"stage": "usage-budget",
		})
	}
}

func (s *Session) ensureShrikeUsageTracker() *token.UsageTracker {
	if s == nil || s.LifecycleSvc() == nil {
		return nil
	}
	return s.LifecycleSvc().EnsureUsageTracker()
}

func (s *Session) currentShrikeUsageTracker() *token.UsageTracker {
	if s == nil || s.LifecycleSvc() == nil {
		return nil
	}
	return s.LifecycleSvc().UsageTracker()
}

func (s *Session) shrikeUsageCanProceed() (bool, string) {
	tracker := s.currentShrikeUsageTracker()
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
	repositoryDir := s.configuredWorkingDir()
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
		Scope:         eyriegraph.Scope{RepositoryID: repositoryID},
		CorrelationID: sessionID,
	})
	if err == nil {
		err = graphjournal.AppendRuntimeGraph(
			sessionID, "", "model-generation", "eyrie",
			toEagleNodes(export.Nodes), toEagleEdges(export.Edges), toEagleEvents(export.Events), observedAt,
		)
	}
	if err != nil {
		s.Logger().Warn("graph observation append failed", map[string]interface{}{
			"kind":  graphjournal.KindRuntime,
			"stage": "model-generation",
		})
	}
}

// The following helpers convert Eyrie's vendored graph contract types into
// Hawk's eagle/graph contract types. The definitions are byte-identical, so
// conversion is a field-by-field copy at the sibling boundary.

func toEagleNodes(nodes []eyriegraph.Node) []graphcontracts.Node {
	out := make([]graphcontracts.Node, len(nodes))
	for i, n := range nodes {
		out[i] = toEagleNode(n)
	}
	return out
}

func toEagleNode(n eyriegraph.Node) graphcontracts.Node {
	return graphcontracts.Node{
		ID:          n.ID,
		Kind:        graphcontracts.NodeKind(n.Kind),
		Scope:       toEagleScope(n.Scope),
		CreatedAt:   n.CreatedAt,
		EffectiveAt: n.EffectiveAt,
		Provenance:  toEagleProvenance(n.Provenance),
		Attributes:  n.Attributes,
	}
}

func toEagleEdges(edges []eyriegraph.Edge) []graphcontracts.Edge {
	out := make([]graphcontracts.Edge, len(edges))
	for i, e := range edges {
		out[i] = toEagleEdge(e)
	}
	return out
}

func toEagleEdge(e eyriegraph.Edge) graphcontracts.Edge {
	return graphcontracts.Edge{
		ID:          e.ID,
		Kind:        graphcontracts.EdgeKind(e.Kind),
		From:        toEagleRef(e.From),
		To:          toEagleRef(e.To),
		Scope:       toEagleScope(e.Scope),
		CreatedAt:   e.CreatedAt,
		EffectiveAt: e.EffectiveAt,
		Provenance:  toEagleProvenance(e.Provenance),
		Attributes:  e.Attributes,
	}
}

func toEagleEvents(events []eyriegraph.Event) []graphcontracts.Event {
	out := make([]graphcontracts.Event, len(events))
	for i, ev := range events {
		out[i] = toEagleEvent(ev)
	}
	return out
}

func toEagleEvent(ev eyriegraph.Event) graphcontracts.Event {
	return graphcontracts.Event{
		ID:             ev.ID,
		Type:           graphcontracts.EventType(ev.Type),
		Subject:        toEagleRef(ev.Subject),
		Scope:          toEagleScope(ev.Scope),
		OccurredAt:     ev.OccurredAt,
		CorrelationID:  ev.CorrelationID,
		CausationID:    ev.CausationID,
		IdempotencyKey: ev.IdempotencyKey,
		Provenance:     toEagleProvenance(ev.Provenance),
	}
}

func toEagleRef(r eyriegraph.Ref) graphcontracts.Ref {
	return graphcontracts.Ref{Kind: graphcontracts.NodeKind(r.Kind), ID: r.ID}
}

func toEagleScope(s eyriegraph.Scope) graphcontracts.Scope {
	return graphcontracts.Scope{TenantID: s.TenantID, ProjectID: s.ProjectID, RepositoryID: s.RepositoryID}
}

func toEagleProvenance(p eyriegraph.Provenance) graphcontracts.Provenance {
	evidence := make([]graphcontracts.ArtifactRef, len(p.Evidence))
	for i, a := range p.Evidence {
		evidence[i] = graphcontracts.ArtifactRef{URI: a.URI, Digest: a.Digest, MediaType: a.MediaType}
	}
	return graphcontracts.Provenance{Producer: p.Producer, Version: p.Version, SourceID: p.SourceID, Evidence: evidence}
}

// Shrike's vendored graph contract types are byte-identical to eagle/graph, so
// conversion is a field-by-field copy at the sibling boundary.

func shrikeToEagleNodes(nodes []shrikegraph.Node) []graphcontracts.Node {
	out := make([]graphcontracts.Node, len(nodes))
	for i, n := range nodes {
		out[i] = shrikeToEagleNode(n)
	}
	return out
}

func shrikeToEagleNode(n shrikegraph.Node) graphcontracts.Node {
	return graphcontracts.Node{
		ID:          n.ID,
		Kind:        graphcontracts.NodeKind(n.Kind),
		Scope:       shrikeToEagleScope(n.Scope),
		CreatedAt:   n.CreatedAt,
		EffectiveAt: n.EffectiveAt,
		Provenance:  shrikeToEagleProvenance(n.Provenance),
		Attributes:  n.Attributes,
	}
}

func shrikeToEagleEdges(edges []shrikegraph.Edge) []graphcontracts.Edge {
	out := make([]graphcontracts.Edge, len(edges))
	for i, e := range edges {
		out[i] = shrikeToEagleEdge(e)
	}
	return out
}

func shrikeToEagleEdge(e shrikegraph.Edge) graphcontracts.Edge {
	return graphcontracts.Edge{
		ID:          e.ID,
		Kind:        graphcontracts.EdgeKind(e.Kind),
		From:        shrikeToEagleRef(e.From),
		To:          shrikeToEagleRef(e.To),
		Scope:       shrikeToEagleScope(e.Scope),
		CreatedAt:   e.CreatedAt,
		EffectiveAt: e.EffectiveAt,
		Provenance:  shrikeToEagleProvenance(e.Provenance),
		Attributes:  e.Attributes,
	}
}

func shrikeToEagleEvents(events []shrikegraph.Event) []graphcontracts.Event {
	out := make([]graphcontracts.Event, len(events))
	for i, ev := range events {
		out[i] = shrikeToEagleEvent(ev)
	}
	return out
}

func shrikeToEagleEvent(ev shrikegraph.Event) graphcontracts.Event {
	return graphcontracts.Event{
		ID:             ev.ID,
		Type:           graphcontracts.EventType(ev.Type),
		Subject:        shrikeToEagleRef(ev.Subject),
		Scope:          shrikeToEagleScope(ev.Scope),
		OccurredAt:     ev.OccurredAt,
		CorrelationID:  ev.CorrelationID,
		CausationID:    ev.CausationID,
		IdempotencyKey: ev.IdempotencyKey,
		Provenance:     shrikeToEagleProvenance(ev.Provenance),
	}
}

func shrikeToEagleRef(r shrikegraph.Ref) graphcontracts.Ref {
	return graphcontracts.Ref{Kind: graphcontracts.NodeKind(r.Kind), ID: r.ID}
}

func shrikeToEagleScope(s shrikegraph.Scope) graphcontracts.Scope {
	return graphcontracts.Scope{TenantID: s.TenantID, ProjectID: s.ProjectID, RepositoryID: s.RepositoryID}
}

func shrikeToEagleProvenance(p shrikegraph.Provenance) graphcontracts.Provenance {
	evidence := make([]graphcontracts.ArtifactRef, len(p.Evidence))
	for i, a := range p.Evidence {
		evidence[i] = graphcontracts.ArtifactRef{URI: a.URI, Digest: a.Digest, MediaType: a.MediaType}
	}
	return graphcontracts.Provenance{Producer: p.Producer, Version: p.Version, SourceID: p.SourceID, Evidence: evidence}
}
