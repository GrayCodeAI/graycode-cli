package merlin

import (
	"context"
	"sync"
	"time"

	graphcontracts "github.com/GrayCodeAI/graycode-cli/internal/contracts/graph"
	typescontracts "github.com/GrayCodeAI/graycode-cli/internal/contracts/types"
	verifycontracts "github.com/GrayCodeAI/graycode-cli/internal/contracts/verify"
	"github.com/GrayCodeAI/graycode-cli/internal/graphjournal"
	merlinLib "github.com/GrayCodeAI/merlin"
	merlingraph "github.com/GrayCodeAI/merlin/graph"
	"github.com/GrayCodeAI/merlin/qualitygraph"
	merlinverify "github.com/GrayCodeAI/merlin/verify"
)

// Bridge connects graycode to the merlin site-auditing library.
// If initialization fails, all operations degrade gracefully and return
// empty results rather than errors.
type Bridge struct {
	scanner *merlinLib.Scanner
	mu      sync.Mutex
	ready   bool
}

// GraphObservation identifies an opt-in Graycode quality-graph journal record.
type GraphObservation struct {
	SessionID   string
	ToolCallID  string
	Stage       string
	Scope       graphcontracts.Scope
	ObservedAt  time.Time
	MaxFindings int
}

// NewBridge creates a bridge to the merlin library with the given options.
// Returns a bridge that silently no-ops if initialization fails.
func NewBridge(opts ...merlinLib.Option) *Bridge {
	b := &Bridge{}
	b.init(opts...)
	return b
}

func (b *Bridge) init(opts ...merlinLib.Option) {
	b.scanner = merlinLib.NewScanner(opts...)
	b.ready = true
}

// Ready reports whether the merlin bridge is initialized and usable.
func (b *Bridge) Ready() bool {
	return b.ready
}

// Run crawls the target URL and runs all configured checks, returning a
// complete report with findings and stats. Falls back silently if the
// bridge is not initialized.
func (b *Bridge) Run(ctx context.Context, target string, opts ...merlinLib.Option) (*merlinLib.Report, error) {
	if !b.ready {
		return &merlinLib.Report{Target: target}, nil
	}
	b.mu.Lock()
	defer b.mu.Unlock()

	// If additional per-call options are provided, create a one-off scanner;
	// otherwise reuse the bridge's scanner.
	if len(opts) > 0 {
		s := merlinLib.NewScanner(opts...)
		return s.Scan(ctx, target)
	}
	return b.scanner.Scan(ctx, target)
}

// RunContracts performs a verification scan and returns the neutral verification contract.
func (b *Bridge) RunContracts(ctx context.Context, target string, opts ...merlinLib.Option) (*verifycontracts.Report, error) {
	report, err := b.Run(ctx, target, opts...)
	if err != nil {
		return nil, err
	}
	return toContractReport(merlinLib.ToContractReport(report)), nil
}

// RunContractsObserved performs a scan, journals Merlin's portable quality
// graph, and returns the existing neutral verification contract.
func (b *Bridge) RunContractsObserved(
	ctx context.Context,
	target string,
	observation GraphObservation,
	opts ...merlinLib.Option,
) (*verifycontracts.Report, error) {
	report, err := b.Run(ctx, target, opts...)
	if err != nil {
		return nil, err
	}
	observedAt := observation.ObservedAt.UTC()
	if observedAt.IsZero() {
		observedAt = time.Now().UTC()
	}
	export, err := qualitygraph.Build(report, qualitygraph.Options{
		ObservedAt:    observedAt,
		Scope:         toMerlinScope(observation.Scope),
		CorrelationID: observation.SessionID,
		MaxFindings:   observation.MaxFindings,
	})
	if err != nil {
		return nil, err
	}
	stage := observation.Stage
	if stage == "" {
		stage = "merlin"
	}
	if err := graphjournal.AppendQualityGraph(
		observation.SessionID,
		observation.ToolCallID,
		stage,
		"merlin",
		toContractNodes(export.Nodes),
		toContractEdges(export.Edges),
		toContractEvents(export.Events),
		observedAt,
	); err != nil {
		return nil, err
	}
	contractReport := merlinLib.ToContractReport(report)
	if err := graphjournal.AppendVerification(
		observation.SessionID,
		observation.ToolCallID,
		stage,
		contractReport.Failed(),
		len(contractReport.Findings),
		contractReport.MaxSeverity().String(),
		target,
		observedAt,
	); err != nil {
		return nil, err
	}
	return toContractReport(contractReport), nil
}

// The following helpers convert Merlin's vendored contract types into Graycode's
// contracts/* contract types (and the reverse for scope). The definitions are
// byte-identical, so conversion is a field-by-field copy at the boundary.

func toMerlinScope(s graphcontracts.Scope) merlingraph.Scope {
	return merlingraph.Scope{TenantID: s.TenantID, ProjectID: s.ProjectID, RepositoryID: s.RepositoryID}
}

func toContractNodes(nodes []merlingraph.Node) []graphcontracts.Node {
	out := make([]graphcontracts.Node, len(nodes))
	for i, n := range nodes {
		out[i] = toContractNode(n)
	}
	return out
}

func toContractNode(n merlingraph.Node) graphcontracts.Node {
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

func toContractEdges(edges []merlingraph.Edge) []graphcontracts.Edge {
	out := make([]graphcontracts.Edge, len(edges))
	for i, e := range edges {
		out[i] = toContractEdge(e)
	}
	return out
}

func toContractEdge(e merlingraph.Edge) graphcontracts.Edge {
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

func toContractEvents(events []merlingraph.Event) []graphcontracts.Event {
	out := make([]graphcontracts.Event, len(events))
	for i, ev := range events {
		out[i] = toContractEvent(ev)
	}
	return out
}

func toContractEvent(ev merlingraph.Event) graphcontracts.Event {
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

func toContractRef(r merlingraph.Ref) graphcontracts.Ref {
	return graphcontracts.Ref{Kind: graphcontracts.NodeKind(r.Kind), ID: r.ID}
}

func toContractScope(s merlingraph.Scope) graphcontracts.Scope {
	return graphcontracts.Scope{TenantID: s.TenantID, ProjectID: s.ProjectID, RepositoryID: s.RepositoryID}
}

func toContractProvenance(p merlingraph.Provenance) graphcontracts.Provenance {
	evidence := make([]graphcontracts.ArtifactRef, len(p.Evidence))
	for i, a := range p.Evidence {
		evidence[i] = graphcontracts.ArtifactRef{URI: a.URI, Digest: a.Digest, MediaType: a.MediaType}
	}
	return graphcontracts.Provenance{Producer: p.Producer, Version: p.Version, SourceID: p.SourceID, Evidence: evidence}
}

func toContractReport(r *merlinverify.Report) *verifycontracts.Report {
	if r == nil {
		return nil
	}
	findings := make([]verifycontracts.Finding, len(r.Findings))
	for i, f := range r.Findings {
		findings[i] = verifycontracts.Finding{
			Check:    f.Check,
			Severity: typescontracts.Severity(f.Severity),
			URL:      f.URL,
			Element:  f.Element,
			Message:  f.Message,
			Fix:      f.Fix,
			Evidence: f.Evidence,
		}
	}
	bySeverity := make(map[typescontracts.Severity]int, len(r.Stats.BySeverity))
	for sev, count := range r.Stats.BySeverity {
		bySeverity[typescontracts.Severity(sev)] = count
	}
	return &verifycontracts.Report{
		Target:   r.Target,
		Findings: findings,
		Stats: verifycontracts.Stats{
			PagesScanned:     r.Stats.PagesScanned,
			FindingsTotal:    r.Stats.FindingsTotal,
			BySeverity:       bySeverity,
			ByCheck:          r.Stats.ByCheck,
			DurationPerCheck: r.Stats.DurationPerCheck,
		},
		CrawledURLs: r.CrawledURLs,
		Duration:    r.Duration,
		FailOn:      typescontracts.Severity(r.FailOn),
		FailOnSet:   r.FailOnSet,
	}
}
