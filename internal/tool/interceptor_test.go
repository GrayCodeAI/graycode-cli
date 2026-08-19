package tool

import (
	"context"
	"errors"
	"testing"

	"github.com/GrayCodeAI/hawk/internal/types"
)

func TestChainRunsInOrder(t *testing.T) {
	var order []string
	c := &Chain{}
	c.Append(func(ctx context.Context, req ToolRequest, res *ToolResult, next func() error) error {
		order = append(order, "a")
		return next()
	})
	c.Append(func(ctx context.Context, req ToolRequest, res *ToolResult, next func() error) error {
		order = append(order, "b")
		return next()
	})
	if err := c.Run(context.Background(), ToolRequest{}, nil); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(order) != 2 || order[0] != "a" || order[1] != "b" {
		t.Fatalf("order = %v, want [a b]", order)
	}
}

func TestChainShortCircuits(t *testing.T) {
	var ran []string
	c := &Chain{}
	c.Append(func(ctx context.Context, req ToolRequest, res *ToolResult, next func() error) error {
		ran = append(ran, "first")
		return ShortCircuitDeny("nope")
	})
	c.Append(func(ctx context.Context, req ToolRequest, res *ToolResult, next func() error) error {
		ran = append(ran, "second")
		return next()
	})
	err := c.Run(context.Background(), ToolRequest{}, nil)
	var sc *ShortCircuit
	if !errors.As(err, &sc) {
		t.Fatalf("Run() error = %v, want *ShortCircuit", err)
	}
	if msg, isErr := sc.ToolError(); msg != "nope" || !isErr {
		t.Fatalf("ToolError() = %q, %v", msg, isErr)
	}
	if len(ran) != 1 || ran[0] != "first" {
		t.Fatalf("ran = %v, want [first]", ran)
	}
}

func TestChainRemoveHeadAndTail(t *testing.T) {
	var calls []int
	k1 := func(ctx context.Context, req ToolRequest, res *ToolResult, next func() error) error {
		calls = append(calls, 1)
		return next()
	}
	k2 := func(ctx context.Context, req ToolRequest, res *ToolResult, next func() error) error {
		calls = append(calls, 2)
		return next()
	}
	k3 := func(ctx context.Context, req ToolRequest, res *ToolResult, next func() error) error {
		calls = append(calls, 3)
		return next()
	}

	c := &Chain{}
	d1 := c.Append(k1)
	c.Append(k2)
	d3 := c.Append(k3)
	if c.Len() != 3 {
		t.Fatalf("Len = %d, want 3", c.Len())
	}

	d1() // remove head
	_ = c.Run(context.Background(), ToolRequest{}, nil)
	if len(calls) != 2 || calls[0] != 2 || calls[1] != 3 {
		t.Fatalf("after head removal calls = %v, want [2 3]", calls)
	}
	calls = nil
	d3() // remove tail
	_ = c.Run(context.Background(), ToolRequest{}, nil)
	if len(calls) != 1 || calls[0] != 2 {
		t.Fatalf("after tail removal calls = %v, want [2]", calls)
	}
	if c.Len() != 1 {
		t.Fatalf("Len = %d, want 1", c.Len())
	}
}

func TestPipelineStageIsolation(t *testing.T) {
	var seen []string
	p := NewPipeline()
	p.Register(StagePreExecute, func(ctx context.Context, req ToolRequest, res *ToolResult, next func() error) error {
		seen = append(seen, "pre")
		return next()
	})
	p.Register(StagePostExecute, func(ctx context.Context, req ToolRequest, res *ToolResult, next func() error) error {
		seen = append(seen, "post")
		return next()
	})
	req := ToolRequest{Call: types.ToolCall{Name: "Read"}}
	_ = p.Run(StagePreExecute, context.Background(), req, nil)
	_ = p.Run(StagePostExecute, context.Background(), req, nil)
	if len(seen) != 2 || seen[0] != "pre" || seen[1] != "post" {
		t.Fatalf("stage order = %v, want [pre post]", seen)
	}
}

func TestPipelineFailClosed(t *testing.T) {
	p := NewPipeline()
	p.Register(StagePreExecute, func(ctx context.Context, req ToolRequest, res *ToolResult, next func() error) error {
		return ShortCircuitDeny("no handler")
	})
	err := p.Run(StagePreExecute, context.Background(), ToolRequest{}, nil)
	var sc *ShortCircuit
	if !errors.As(err, &sc) {
		t.Fatalf("error = %v, want ShortCircuit", err)
	}
	if msg, isErr := sc.ToolError(); msg != "no handler" || !isErr {
		t.Fatalf("ToolError = %q, %v", msg, isErr)
	}
}

func TestDisposerRestoresPriorState(t *testing.T) {
	p := NewPipeline()
	p.Register(StagePreExecute, func(ctx context.Context, req ToolRequest, res *ToolResult, next func() error) error { return next() })
	var ran bool
	dispose := p.Register(StagePreExecute, func(ctx context.Context, req ToolRequest, res *ToolResult, next func() error) error {
		ran = true
		return next()
	})
	dispose()
	_ = p.Run(StagePreExecute, context.Background(), ToolRequest{}, nil)
	if ran {
		t.Fatal("disposed interceptor should not run")
	}
}
