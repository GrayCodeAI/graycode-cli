package engine

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	graphcontracts "github.com/GrayCodeAI/graycode-cli/internal/contracts/graph"
	policycontracts "github.com/GrayCodeAI/graycode-cli/internal/contracts/policy"
	"github.com/GrayCodeAI/graycode-cli/internal/engine/token"
	"github.com/GrayCodeAI/graycode-cli/internal/graphjournal"
	"github.com/GrayCodeAI/graycode-cli/internal/types"
	graycoderouterengine "github.com/GrayCodeAI/graycode-router/engine"
	graycoderoutergraph "github.com/GrayCodeAI/graycode-router/graph"
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
	verdict.Source = "graycode." + strings.TrimSpace(stage)
	if !allowed {
		verdict = policycontracts.Deny(reason, strings.TrimSpace(stage))
		verdict.Source = "graycode." + strings.TrimSpace(stage)
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
// persisted Graycode session. It is safe to call before either side is configured.
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
			shrikeToContractNodes(export.Nodes), shrikeToContractEdges(export.Edges), shrikeToContractEvents(export.Events), observedAt,
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
			shrikeToContractNodes(export.Nodes), shrikeToContractEdges(export.Edges), shrikeToContractEvents(export.Events), observedAt,
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
			shrikeToContractNodes(export.Nodes), shrikeToContractEdges(export.Edges), shrikeToContractEvents(export.Events), observedAt,
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

func (s *Session) recordGraycodeRouterOperationObservation(
	provider, model, finishReason, content string,
	toolCallCount int,
	usage *types.GraycodeRouterUsage,
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
	export, err := graycoderouterengine.BuildOperationsGraph(graycoderouterengine.OperationsGraphInput{
		Route:         &route,
		Usage:         usage,
		FinishReason:  finishReason,
		Content:       content,
		ToolCallCount: toolCallCount,
		ObservedAt:    observedAt,
		Scope:         graycoderoutergraph.Scope{RepositoryID: repositoryID},
		CorrelationID: sessionID,
	})
	if err == nil {
		err = graphjournal.AppendRuntimeGraph(
			sessionID, "", "model-generation", "graycode-router",
			toContractNodes(export.Nodes), toContractEdges(export.Edges), toContractEvents(export.Events), observedAt,
		)
	}
	if err != nil {
		s.Logger().Warn("graph observation append failed", map[string]interface{}{
			"kind":  graphjournal.KindRuntime,
			"stage": "model-generation",
		})
	}
}

// The following helpers convert GraycodeRouter's vendored graph contract types into
// Graycode's contracts/graph contract types. The definitions are byte-identical, so
// conversion is a field-by-field copy at the sibling boundary.

func toContractNodes(nodes []graycoderoutergraph.Node) []graphcontracts.Node {
	out := make([]graphcontracts.Node, len(nodes))
	for i, n := range nodes {
		out[i] = toContractNode(n)
	}
	return out
}

func toContractNode(n graycoderoutergraph.Node) graphcontracts.Node {
	return graphcontracts.Node{
		ID:          n.ID,
		Kind:        graphcontracts.NodeKind(n.Kind),
		Scope:       toContractScope(n.Scope),
		CreatedAt:   n.CreatedAt,
		EffectiveAt: n.EffectiveAt,
		Provenance:  toContractProvenance(n.Provenance),
		Attributes:  n.Attributes,
	}
}

func toContractEdges(edges []graycoderoutergraph.Edge) []graphcontracts.Edge {
	out := make([]graphcontracts.Edge, len(edges))
	for i, e := range edges {
		out[i] = toContractEdge(e)
	}
	return out
}

func toContractEdge(e graycoderoutergraph.Edge) graphcontracts.Edge {
	return graphcontracts.Edge{
		ID:          e.ID,
		Kind:        graphcontracts.EdgeKind(e.Kind),
		From:        toContractRef(e.From),
		To:          toContractRef(e.To),
		Scope:       toContractScope(e.Scope),
		CreatedAt:   e.CreatedAt,
		EffectiveAt: e.EffectiveAt,
		Provenance:  toContractProvenance(e.Provenance),
		Attributes:  e.Attributes,
	}
}

func toContractEvents(events []graycoderoutergraph.Event) []graphcontracts.Event {
	out := make([]graphcontracts.Event, len(events))
	for i, ev := range events {
		out[i] = toContractEvent(ev)
	}
	return out
}

func toContractEvent(ev graycoderoutergraph.Event) graphcontracts.Event {
	return graphcontracts.Event{
		ID:             ev.ID,
		Type:           graphcontracts.EventType(ev.Type),
		Subject:        toContractRef(ev.Subject),
		Scope:          toContractScope(ev.Scope),
		OccurredAt:     ev.OccurredAt,
		CorrelationID:  ev.CorrelationID,
		CausationID:    ev.CausationID,
		IdempotencyKey: ev.IdempotencyKey,
		Provenance:     toContractProvenance(ev.Provenance),
	}
}

func toContractRef(r graycoderoutergraph.Ref) graphcontracts.Ref {
	return graphcontracts.Ref{Kind: graphcontracts.NodeKind(r.Kind), ID: r.ID}
}

func toContractScope(s graycoderoutergraph.Scope) graphcontracts.Scope {
	return graphcontracts.Scope{TenantID: s.TenantID, ProjectID: s.ProjectID, RepositoryID: s.RepositoryID}
}

func toContractProvenance(p graycoderoutergraph.Provenance) graphcontracts.Provenance {
	evidence := make([]graphcontracts.ArtifactRef, len(p.Evidence))
	for i, a := range p.Evidence {
		evidence[i] = graphcontracts.ArtifactRef{URI: a.URI, Digest: a.Digest, MediaType: a.MediaType}
	}
	return graphcontracts.Provenance{Producer: p.Producer, Version: p.Version, SourceID: p.SourceID, Evidence: evidence}
}

// Shrike's vendored graph contract types are byte-identical to contracts/graph, so
// conversion is a field-by-field copy at the sibling boundary.

func shrikeToContractNodes(nodes []shrikegraph.Node) []graphcontracts.Node {
	out := make([]graphcontracts.Node, len(nodes))
	for i, n := range nodes {
		out[i] = shrikeToContractNode(n)
	}
	return out
}

func shrikeToContractNode(n shrikegraph.Node) graphcontracts.Node {
	return graphcontracts.Node{
		ID:          n.ID,
		Kind:        graphcontracts.NodeKind(n.Kind),
		Scope:       shrikeToContractScope(n.Scope),
		CreatedAt:   n.CreatedAt,
		EffectiveAt: n.EffectiveAt,
		Provenance:  shrikeToContractProvenance(n.Provenance),
		Attributes:  n.Attributes,
	}
}

func shrikeToContractEdges(edges []shrikegraph.Edge) []graphcontracts.Edge {
	out := make([]graphcontracts.Edge, len(edges))
	for i, e := range edges {
		out[i] = shrikeToContractEdge(e)
	}
	return out
}

func shrikeToContractEdge(e shrikegraph.Edge) graphcontracts.Edge {
	return graphcontracts.Edge{
		ID:          e.ID,
		Kind:        graphcontracts.EdgeKind(e.Kind),
		From:        shrikeToContractRef(e.From),
		To:          shrikeToContractRef(e.To),
		Scope:       shrikeToContractScope(e.Scope),
		CreatedAt:   e.CreatedAt,
		EffectiveAt: e.EffectiveAt,
		Provenance:  shrikeToContractProvenance(e.Provenance),
		Attributes:  e.Attributes,
	}
}

func shrikeToContractEvents(events []shrikegraph.Event) []graphcontracts.Event {
	out := make([]graphcontracts.Event, len(events))
	for i, ev := range events {
		out[i] = shrikeToContractEvent(ev)
	}
	return out
}

func shrikeToContractEvent(ev shrikegraph.Event) graphcontracts.Event {
	return graphcontracts.Event{
		ID:             ev.ID,
		Type:           graphcontracts.EventType(ev.Type),
		Subject:        shrikeToContractRef(ev.Subject),
		Scope:          shrikeToContractScope(ev.Scope),
		OccurredAt:     ev.OccurredAt,
		CorrelationID:  ev.CorrelationID,
		CausationID:    ev.CausationID,
		IdempotencyKey: ev.IdempotencyKey,
		Provenance:     shrikeToContractProvenance(ev.Provenance),
	}
}

func shrikeToContractRef(r shrikegraph.Ref) graphcontracts.Ref {
	return graphcontracts.Ref{Kind: graphcontracts.NodeKind(r.Kind), ID: r.ID}
}

func shrikeToContractScope(s shrikegraph.Scope) graphcontracts.Scope {
	return graphcontracts.Scope{TenantID: s.TenantID, ProjectID: s.ProjectID, RepositoryID: s.RepositoryID}
}

func shrikeToContractProvenance(p shrikegraph.Provenance) graphcontracts.Provenance {
	evidence := make([]graphcontracts.ArtifactRef, len(p.Evidence))
	for i, a := range p.Evidence {
		evidence[i] = graphcontracts.ArtifactRef{URI: a.URI, Digest: a.Digest, MediaType: a.MediaType}
	}
	return graphcontracts.Provenance{Producer: p.Producer, Version: p.Version, SourceID: p.SourceID, Evidence: evidence}
}
