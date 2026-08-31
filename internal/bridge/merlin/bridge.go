package merlin

import (
	"context"
	"sync"
	"time"

	graphcontracts "github.com/GrayCodeAI/eagle/graph"
	verifycontracts "github.com/GrayCodeAI/eagle/verify"
	"github.com/GrayCodeAI/hawk/internal/graphjournal"
	merlinLib "github.com/GrayCodeAI/merlin"
	"github.com/GrayCodeAI/merlin/qualitygraph"
)

// Bridge connects hawk to the merlin site-auditing library.
// If initialization fails, all operations degrade gracefully and return
// empty results rather than errors.
type Bridge struct {
	scanner *merlinLib.Scanner
	mu      sync.Mutex
	ready   bool
}

// GraphObservation identifies an opt-in Hawk quality-graph journal record.
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
	return merlinLib.ToContractReport(report), nil
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
		Scope:         observation.Scope,
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
		export.Nodes,
		export.Edges,
		export.Events,
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
	return contractReport, nil
}
