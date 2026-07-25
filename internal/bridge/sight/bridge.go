package sight

import (
	"context"
	"sync"
	"time"

	graphcontracts "github.com/GrayCodeAI/hawk-core-contracts/graph"
	reviewcontracts "github.com/GrayCodeAI/hawk-core-contracts/review"
	"github.com/GrayCodeAI/hawk/internal/graphjournal"
	"github.com/GrayCodeAI/hawk/internal/types"
	sightLib "github.com/GrayCodeAI/sight"
	"github.com/GrayCodeAI/sight/qualitygraph"
)

// EyrieAdapter implements sight's Provider interface using hawk's eyrie client.
// It translates between sight.Message/sight.ChatOpts and Hawk runtime DTOs.
type EyrieAdapter struct {
	client   types.ChatProvider
	provider string
}

// NewEyrieAdapter creates an adapter that satisfies sight.Provider using
// the given eyrie client and provider name (e.g. "anthropic", "openai").
func NewEyrieAdapter(c types.ChatProvider, provider string) *EyrieAdapter {
	return &EyrieAdapter{client: c, provider: provider}
}

// Chat translates a sight LLM request into an eyrie call and returns the
// result in sight's Response format.
func (a *EyrieAdapter) Chat(ctx context.Context, messages []sightLib.Message, opts sightLib.ChatOpts) (*sightLib.Response, error) {
	eyrieMessages := make([]types.EyrieMessage, len(messages))
	for i, m := range messages {
		eyrieMessages[i] = types.EyrieMessage{
			Role:    m.Role,
			Content: m.Content,
		}
	}

	var temp *float64
	if opts.Temperature != 0 {
		t := opts.Temperature
		temp = &t
	}

	eyrieOpts := types.ChatOptions{
		Provider:    a.provider,
		Model:       opts.Model,
		MaxTokens:   opts.MaxTokens,
		Temperature: temp,
		System:      opts.System,
	}

	resp, err := a.client.Chat(ctx, eyrieMessages, eyrieOpts)
	if err != nil {
		return nil, err
	}

	tokensUsed := 0
	if resp.Usage != nil {
		tokensUsed = resp.Usage.TotalTokens
	}

	return &sightLib.Response{
		Content:    resp.Content,
		TokensUsed: tokensUsed,
	}, nil
}

// Bridge connects hawk to the sight code-review library.
// If initialization fails, all operations degrade gracefully and return
// empty results rather than errors.
type Bridge struct {
	adapter  *EyrieAdapter
	reviewer *sightLib.Reviewer
	opts     []sightLib.Option
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

// NewBridge creates a bridge to the sight library using the given Hawk
// transport client and provider name. Additional sight options (model,
// concerns, etc.) are applied to all operations.
func NewBridge(c types.ChatProvider, provider string, opts ...sightLib.Option) *Bridge {
	b := &Bridge{}
	b.init(c, provider, opts...)
	return b
}

func (b *Bridge) init(c types.ChatProvider, provider string, opts ...sightLib.Option) {
	if c == nil {
		return
	}
	b.adapter = NewEyrieAdapter(c, provider)
	// Prepend the provider option so callers don't have to.
	b.opts = append([]sightLib.Option{sightLib.WithProvider(b.adapter)}, opts...)
	b.reviewer = sightLib.NewReviewer(b.opts...)
	b.ready = true
}

// Ready reports whether the sight bridge is initialized and usable.
func (b *Bridge) Ready() bool {
	return b.ready
}

// Review performs an AI-powered code review on a unified diff string.
// Falls back silently if the bridge is not initialized.
func (b *Bridge) Review(ctx context.Context, diff string) (*sightLib.Result, error) {
	if !b.ready {
		return &sightLib.Result{Report: "sight bridge not initialized"}, nil
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
	return sightLib.ToContractResult(result), nil
}

// ReviewContractsObserved reviews a diff, journals Sight's portable quality
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
		Scope:         observation.Scope,
		CorrelationID: observation.SessionID,
		Source:        diff,
		MaxFindings:   observation.MaxFindings,
	})
	if err != nil {
		return nil, err
	}
	stage := observation.Stage
	if stage == "" {
		stage = "sight-review"
	}
	if err := graphjournal.AppendQualityGraph(
		observation.SessionID,
		observation.ToolCallID,
		stage,
		"sight",
		export.Nodes,
		export.Edges,
		export.Events,
		observedAt,
	); err != nil {
		return nil, err
	}
	contractResult := sightLib.ToContractResult(result)
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
	return contractResult, nil
}

// Describe generates a PR description from a unified diff string.
// Falls back silently if the bridge is not initialized.
func (b *Bridge) Describe(ctx context.Context, diff string) (*sightLib.Description, error) {
	if !b.ready {
		return &sightLib.Description{Title: "sight bridge not initialized"}, nil
	}
	b.mu.Lock()
	defer b.mu.Unlock()

	return sightLib.Describe(ctx, diff, b.opts...)
}

// Improve analyzes a diff and suggests code improvements.
// Falls back silently if the bridge is not initialized.
func (b *Bridge) Improve(ctx context.Context, diff string) (*sightLib.ImproveResult, error) {
	if !b.ready {
		return &sightLib.ImproveResult{}, nil
	}
	b.mu.Lock()
	defer b.mu.Unlock()

	return sightLib.Improve(ctx, diff, b.opts...)
}
