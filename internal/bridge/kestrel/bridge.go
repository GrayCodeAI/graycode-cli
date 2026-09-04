package kestrel

import (
	"context"
	"sync"
	"time"

	graphcontracts "github.com/GrayCodeAI/graycode-cli/internal/contracts/graph"
	reviewcontracts "github.com/GrayCodeAI/graycode-cli/internal/contracts/review"
	typescontracts "github.com/GrayCodeAI/graycode-cli/internal/contracts/types"
	"github.com/GrayCodeAI/graycode-cli/internal/graphjournal"
	"github.com/GrayCodeAI/graycode-cli/internal/types"
	kestrelLib "github.com/GrayCodeAI/kestrel"
	kestrelgraph "github.com/GrayCodeAI/kestrel/graph"
	"github.com/GrayCodeAI/kestrel/qualitygraph"
	kestrelreview "github.com/GrayCodeAI/kestrel/review"
)

// GraycodeRouterAdapter implements kestrel's Provider interface using graycode's graycode-router client.
// It translates between kestrel.Message/kestrel.ChatOpts and Graycode runtime DTOs.
type GraycodeRouterAdapter struct {
	client   types.ChatProvider
	provider string
}

// NewGraycodeRouterAdapter creates an adapter that satisfies kestrel.Provider using
// the given graycode-router client and provider name (e.g. "anthropic", "openai").
func NewGraycodeRouterAdapter(c types.ChatProvider, provider string) *GraycodeRouterAdapter {
	return &GraycodeRouterAdapter{client: c, provider: provider}
}

// Chat translates a kestrel LLM request into an graycode-router call and returns the
// result in kestrel's Response format.
func (a *GraycodeRouterAdapter) Chat(ctx context.Context, messages []kestrelLib.Message, opts kestrelLib.ChatOpts) (*kestrelLib.Response, error) {
	graycodeRouterMessages := make([]types.GraycodeRouterMessage, len(messages))
	for i, m := range messages {
		graycodeRouterMessages[i] = types.GraycodeRouterMessage{
			Role:    m.Role,
			Content: m.Content,
		}
	}

	var temp *float64
	if opts.Temperature != 0 {
		t := opts.Temperature
		temp = &t
	}

	graycodeRouterOpts := types.ChatOptions{
		Provider:    a.provider,
		Model:       opts.Model,
		MaxTokens:   opts.MaxTokens,
		Temperature: temp,
		System:      opts.System,
	}

	resp, err := a.client.Chat(ctx, graycodeRouterMessages, graycodeRouterOpts)
	if err != nil {
		return nil, err
	}

	tokensUsed := 0
	if resp.Usage != nil {
		tokensUsed = resp.Usage.TotalTokens
	}

	return &kestrelLib.Response{
		Content:    resp.Content,
		TokensUsed: tokensUsed,
	}, nil
}

// Bridge connects graycode to the kestrel code-review library.
// If initialization fails, all operations degrade gracefully and return
// empty results rather than errors.
type Bridge struct {
	adapter  *GraycodeRouterAdapter
	reviewer *kestrelLib.Reviewer
	opts     []kestrelLib.Option
	mu       sync.Mutex
	ready    bool
}

type GraphObservation struct {
	SessionID   string
	ToolCallID  string
	Stage       string
	Scope       graphcontracts.Scope
	ObservedAt  time.Time
	MaxFindings int
}

// NewBridge creates a bridge to the kestrel library using the given Graycode
// transport client and provider name. Additional kestrel options (model,
// concerns, etc.) are applied to all operations.
func NewBridge(c types.ChatProvider, provider string, opts ...kestrelLib.Option) *Bridge {
	b := &Bridge{}
	b.init(c, provider, opts...)
	return b
}

func (b *Bridge) init(c types.ChatProvider, provider string, opts ...kestrelLib.Option) {
	if c == nil {
		return
	}
	b.adapter = NewGraycodeRouterAdapter(c, provider)
	// Prepend the provider option so callers don't have to.
	b.opts = append([]kestrelLib.Option{kestrelLib.WithProvider(b.adapter)}, opts...)
	b.reviewer = kestrelLib.NewReviewer(b.opts...)
	b.ready = true
}

// Ready reports whether the kestrel bridge is initialized and usable.
func (b *Bridge) Ready() bool {
	return b.ready
}

// Review performs an AI-powered code review on a unified diff string.
// Falls back silently if the bridge is not initialized.
func (b *Bridge) Review(ctx context.Context, diff string) (*kestrelLib.Result, error) {
	if !b.ready {
		return &kestrelLib.Result{Report: "kestrel bridge not initialized"}, nil
	}
	b.mu.Lock()
	defer b.mu.Unlock()

	return b.reviewer.Review(ctx, diff)
}

// ReviewContracts performs a review and returns the neutral review contract.
func (b *Bridge) ReviewContracts(ctx context.Context, diff string) (*reviewcontracts.Result, error) {
	result, err := b.Review(ctx, diff)
	if err != nil {
		return nil, err
	}
	return toContractResult(kestrelLib.ToContractResult(result)), nil
}

// ReviewContractsObserved reviews a diff, journals Kestrel's portable quality
// graph, and returns the existing neutral review contract.
func (b *Bridge) ReviewContractsObserved(
	ctx context.Context,
	diff string,
	observation GraphObservation,
) (*reviewcontracts.Result, error) {
	result, err := b.Review(ctx, diff)
	if err != nil {
		return nil, err
	}
	observedAt := observation.ObservedAt.UTC()
	if observedAt.IsZero() {
		observedAt = time.Now().UTC()
	}
	export, err := qualitygraph.Build(result, qualitygraph.Options{
		ObservedAt:    observedAt,
		Scope:         toKestrelScope(observation.Scope),
		CorrelationID: observation.SessionID,
		Source:        diff,
		MaxFindings:   observation.MaxFindings,
	})
	if err != nil {
		return nil, err
	}
	stage := observation.Stage
	if stage == "" {
		stage = "kestrel-review"
	}
	if err := graphjournal.AppendQualityGraph(
		observation.SessionID,
		observation.ToolCallID,
		stage,
		"kestrel",
		toContractNodes(export.Nodes),
		toContractEdges(export.Edges),
		toContractEvents(export.Events),
		observedAt,
	); err != nil {
		return nil, err
	}
	contractResult := kestrelLib.ToContractResult(result)
	if err := graphjournal.AppendVerification(
		observation.SessionID,
		observation.ToolCallID,
		stage,
		contractResult.Failed(),
		len(contractResult.Findings),
		contractResult.MaxSeverity().String(),
		diff,
		observedAt,
	); err != nil {
		return nil, err
	}
	return toContractResult(contractResult), nil
}

// Describe generates a PR description from a unified diff string.
// Falls back silently if the bridge is not initialized.
func (b *Bridge) Describe(ctx context.Context, diff string) (*kestrelLib.Description, error) {
	if !b.ready {
		return &kestrelLib.Description{Title: "kestrel bridge not initialized"}, nil
	}
	b.mu.Lock()
	defer b.mu.Unlock()

	return kestrelLib.Describe(ctx, diff, b.opts...)
}

// Improve analyzes a diff and suggests code improvements.
// Falls back silently if the bridge is not initialized.
func (b *Bridge) Improve(ctx context.Context, diff string) (*kestrelLib.ImproveResult, error) {
	if !b.ready {
		return &kestrelLib.ImproveResult{}, nil
	}
	b.mu.Lock()
	defer b.mu.Unlock()

	return kestrelLib.Improve(ctx, diff, b.opts...)
}

// The following helpers convert Kestrel's vendored contract types into
// Graycode's contracts/* contract types (and the reverse for scope). The definitions
// are byte-identical, so conversion is a field-by-field copy at the boundary.

func toKestrelScope(s graphcontracts.Scope) kestrelgraph.Scope {
	return kestrelgraph.Scope{TenantID: s.TenantID, ProjectID: s.ProjectID, RepositoryID: s.RepositoryID}
}

func toContractNodes(nodes []kestrelgraph.Node) []graphcontracts.Node {
	out := make([]graphcontracts.Node, len(nodes))
	for i, n := range nodes {
		out[i] = toContractNode(n)
	}
	return out
}

func toContractNode(n kestrelgraph.Node) graphcontracts.Node {
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

func toContractEdges(edges []kestrelgraph.Edge) []graphcontracts.Edge {
	out := make([]graphcontracts.Edge, len(edges))
	for i, e := range edges {
		out[i] = toContractEdge(e)
	}
	return out
}

func toContractEdge(e kestrelgraph.Edge) graphcontracts.Edge {
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

func toContractEvents(events []kestrelgraph.Event) []graphcontracts.Event {
	out := make([]graphcontracts.Event, len(events))
	for i, ev := range events {
		out[i] = toContractEvent(ev)
	}
	return out
}

func toContractEvent(ev kestrelgraph.Event) graphcontracts.Event {
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

func toContractRef(r kestrelgraph.Ref) graphcontracts.Ref {
	return graphcontracts.Ref{Kind: graphcontracts.NodeKind(r.Kind), ID: r.ID}
}

func toContractScope(s kestrelgraph.Scope) graphcontracts.Scope {
	return graphcontracts.Scope{TenantID: s.TenantID, ProjectID: s.ProjectID, RepositoryID: s.RepositoryID}
}

func toContractProvenance(p kestrelgraph.Provenance) graphcontracts.Provenance {
	evidence := make([]graphcontracts.ArtifactRef, len(p.Evidence))
	for i, a := range p.Evidence {
		evidence[i] = graphcontracts.ArtifactRef{URI: a.URI, Digest: a.Digest, MediaType: a.MediaType}
	}
	return graphcontracts.Provenance{Producer: p.Producer, Version: p.Version, SourceID: p.SourceID, Evidence: evidence}
}

func toContractResult(r *kestrelreview.Result) *reviewcontracts.Result {
	if r == nil {
		return nil
	}
	return &reviewcontracts.Result{
		Findings:            toContractFindings(r.Findings),
		Comments:            toContractComments(r.Comments),
		Stats:               toContractStats(r.Stats),
		Report:              r.Report,
		FailOn:              typescontracts.Severity(r.FailOn),
		FailOnSet:           r.FailOnSet,
		SASTFusion:          toContractSASTFusion(r.SASTFusion),
		ConfidenceBreakdown: toContractConfidenceBreakdown(r.ConfidenceBreakdown),
	}
}

func toContractFindings(findings []kestrelreview.Finding) []reviewcontracts.Finding {
	out := make([]reviewcontracts.Finding, len(findings))
	for i, f := range findings {
		out[i] = reviewcontracts.Finding{
			Concern:    f.Concern,
			Severity:   typescontracts.Severity(f.Severity),
			File:       f.File,
			Line:       f.Line,
			EndLine:    f.EndLine,
			Message:    f.Message,
			Fix:        f.Fix,
			Reasoning:  f.Reasoning,
			CWE:        f.CWE,
			Confidence: f.Confidence,
			SASTSource: f.SASTSource,
		}
	}
	return out
}

func toContractComments(comments []kestrelreview.InlineComment) []reviewcontracts.InlineComment {
	out := make([]reviewcontracts.InlineComment, len(comments))
	for i, c := range comments {
		out[i] = reviewcontracts.InlineComment{
			Path:       c.Path,
			StartLine:  c.StartLine,
			EndLine:    c.EndLine,
			Body:       c.Body,
			Suggestion: c.Suggestion,
		}
	}
	return out
}

func toContractStats(s kestrelreview.Stats) reviewcontracts.Stats {
	bySeverity := make(map[typescontracts.Severity]int, len(s.BySeverity))
	for sev, count := range s.BySeverity {
		bySeverity[typescontracts.Severity(sev)] = count
	}
	return reviewcontracts.Stats{
		FilesReviewed:       s.FilesReviewed,
		HunksAnalyzed:       s.HunksAnalyzed,
		FindingsTotal:       s.FindingsTotal,
		BySeverity:          bySeverity,
		ByConcern:           s.ByConcern,
		TokensUsed:          s.TokensUsed,
		DurationPerConcern:  s.DurationPerConcern,
		AverageConfidence:   s.AverageConfidence,
		HighConfidenceCount: s.HighConfidenceCount,
		LowConfidenceCount:  s.LowConfidenceCount,
		LLMErrors:           s.LLMErrors,
	}
}

func toContractSASTFusion(f *kestrelreview.SASTFusionResult) *reviewcontracts.SASTFusionResult {
	if f == nil {
		return nil
	}
	return &reviewcontracts.SASTFusionResult{
		Confirmed:   toContractFindings(f.Confirmed),
		Dismissed:   toContractFindings(f.Dismissed),
		Unaddressed: toContractFindings(f.Unaddressed),
	}
}

func toContractConfidenceBreakdown(c *kestrelreview.ConfidenceBreakdown) *reviewcontracts.ConfidenceBreakdown {
	if c == nil {
		return nil
	}
	return &reviewcontracts.ConfidenceBreakdown{
		High:   toContractFindings(c.High),
		Medium: toContractFindings(c.Medium),
		Low:    toContractFindings(c.Low),
	}
}
