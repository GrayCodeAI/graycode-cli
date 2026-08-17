package engine

import (
	"context"
	"testing"

	"github.com/GrayCodeAI/hawk/internal/tool"
	"github.com/GrayCodeAI/hawk/internal/types"
)

func TestToolServiceNormalizeOutputKeepsSmallResults(t *testing.T) {
	service := NewToolService(nil)
	const want = "short tool result"
	if got := service.NormalizeOutput(want, "Read", "call-1", 128_000); got != want {
		t.Fatalf("NormalizeOutput changed a small result: got %q, want %q", got, want)
	}
}

func TestToolServicePostProcessRunsPostExecuteStage(t *testing.T) {
	service := NewToolService(nil)
	p := service.Pipeline()
	if p == nil {
		t.Fatal("lazy Pipeline() returned nil; SetPipeline/DefaultToolPipeline seam broken")
	}
	p.Register(tool.StagePostExecute, func(ctx context.Context, req tool.ToolRequest, res *tool.ToolResult, next func() error) error {
		if err := next(); err != nil {
			return err
		}
		res.Output = "rewritten by post-execute stage"
		res.IsError = false
		return nil
	})

	got := service.PostProcess(context.Background(), toolExecResult{
		tc:     types.ToolCall{Name: "Read", ID: "r1"},
		output: "raw output",
		isErr:  true,
	}, 0, "", 128_000)

	if got.isErr {
		t.Fatal("post-execute stage did not clear error")
	}
	if got.output != "rewritten by post-execute stage" {
		t.Fatalf("output = %q, want rewritten", got.output)
	}
}

func TestDefaultToolPipelineIsEmptyPassThrough(t *testing.T) {
	p := DefaultToolPipeline()
	if p == nil {
		t.Fatal("DefaultToolPipeline returned nil")
	}
	// An empty pipeline must be a strict pass-through: no registered interceptor,
	// so Run delegates immediately without modifying the provider result.
	res := &tool.ToolResult{Output: "unchanged", IsError: false}
	if err := p.Run(tool.StagePostExecute, context.Background(), tool.ToolRequest{}, res); err != nil {
		t.Fatalf("empty pipeline Run: %v", err)
	}
	if res.Output != "unchanged" {
		t.Fatalf("output = %q, want unchanged", res.Output)
	}
}
