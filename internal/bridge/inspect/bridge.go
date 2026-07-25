package inspect

import (
	"context"
	"sync"
	"time"

	graphcontracts "github.com/GrayCodeAI/hawk-core-contracts/graph"
	verifycontracts "github.com/GrayCodeAI/hawk-core-contracts/verify"
	"github.com/GrayCodeAI/hawk/internal/graphjournal"
	inspectLib "github.com/GrayCodeAI/inspect"
	"github.com/GrayCodeAI/inspect/qualitygraph"
)

// Bridge connects hawk to the inspect site-auditing library.
// If initialization fails, all operations degrade gracefully and return
// empty results rather than errors.
type Bridge struct {
	scanner *inspectLib.Scanner
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

// NewBridge creates a bridge to the inspect library with the given options.
// Returns a bridge that silently no-ops if initialization fails.
func NewBridge(opts ...inspectLib.Option) *Bridge {
	b := &Bridge{}
	b.init(opts...)
	return b
}

func (b *Bridge) init(opts ...inspectLib.Option) {
	b.scanner = inspectLib.NewScanner(opts...)
	b.ready = true
}

// Ready reports whether the inspect bridge is initialized and usable.
func (b *Bridge) Ready() bool {
	return b.ready
}

// Run crawls the target URL and runs all configured checks, returning a
// complete report with findings and stats. Falls back silently if the
// bridge is not initialized.
func (b *Bridge) Run(ctx context.Context, target string, opts ...inspectLib.Option) (*inspectLib.Report, error) {
	if !b.ready {
		return &inspectLib.Report{Target: target}, nil
	}
	b.mu.Lock()
	defer b.mu.Unlock()

	// If additional per-call options are provided, create a one-off scanner;
	// otherwise reuse the bridge's scanner.
	if len(opts) > 0 {
		s := inspectLib.NewScanner(opts...)
		return s.Scan(ctx, target)
	}
	return b.scanner.Scan(ctx, target)
}

// RunContracts performs a verification scan and returns the neutral verification contract.
func (b *Bridge) RunContracts(ctx context.Context, target string, opts ...inspectLib.Option) (*verifycontracts.Report, error) {
	report, err := b.Run(ctx, target, opts...)
	if err != nil {
		return nil, err
	}
	return inspectLib.ToContractReport(report), nil
}

// RunContractsObserved performs a scan, journals Inspect's portable quality
// graph, and returns the existing neutral verification contract.
func (b *Bridge) RunContractsObserved(
	ctx context.Context,
	target string,
	observation GraphObservation,
	opts ...inspectLib.Option,
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
		stage = "inspect"
	}
	if err := graphjournal.AppendQualityGraph(
		observation.SessionID,
		observation.ToolCallID,
		stage,
		"inspect",
		export.Nodes,
		export.Edges,
		export.Events,
		observedAt,
	); err != nil {
		return nil, err
	}
	contractReport := inspectLib.ToContractReport(report)
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
