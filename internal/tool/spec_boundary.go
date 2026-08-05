package tool

import (
	"context"
	"encoding/json"
)

type SpecBoundaryTool struct{}

func (SpecBoundaryTool) Name() string { return "SpecBoundary" }

func (SpecBoundaryTool) Execute(ctx context.Context, input json.RawMessage) (string, error) {
	return "boundary analysis", nil
}

func init() { _ = SpecBoundaryTool{} }
