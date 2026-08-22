package tool

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// BatchTool executes a list of read-only tool calls in a single turn, reducing
// the round-trips the agent needs for fan-out research work (the safe core of
// codex's "code mode" batching, without an embedded script runtime).
//
// Security: only read-only tools (see IsReadOnly) are allowed. Every inner
// call is resolved through the session registry, schema-validated, and executed
// individually — no mutation can bypass the normal tool pipeline because a
// non-read-only call fails the request up front.
type BatchTool struct{}

func (BatchTool) Name() string { return "Batch" }

func (BatchTool) Aliases() []string { return []string{"batch"} }

func (BatchTool) Description() string {
	return "Run several read-only tool calls in one turn (fan-out research). Calls must be read-only tools."
}

func (BatchTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"calls": map[string]interface{}{
				"type":        "array",
				"description": "Read-only tool calls to execute in sequence",
				"items": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"tool":  map[string]interface{}{"type": "string", "description": "Read-only tool name (e.g. Read, Grep, Glob, LS, CodeSearch)"},
						"input": map[string]interface{}{"type": "object", "description": "Tool input object"},
					},
					"required": []string{"tool"},
				},
			},
		},
		"required": []string{"calls"},
	}
}

type batchCall struct {
	Tool  string          `json:"tool"`
	Input json.RawMessage `json:"input"`
}

func (BatchTool) Execute(ctx context.Context, input json.RawMessage) (string, error) {
	var p struct {
		Calls []batchCall `json:"calls"`
	}
	if err := json.Unmarshal(input, &p); err != nil {
		return "", fmt.Errorf("batch: %w", err)
	}
	if len(p.Calls) == 0 {
		return "", errors.New("batch: calls is required and must not be empty")
	}

	tc := GetToolContext(ctx)
	if tc == nil || tc.Registry == nil {
		return "", errors.New("batch: tool registry unavailable in this context")
	}

	var b strings.Builder
	for i, call := range p.Calls {
		name := strings.TrimSpace(call.Tool)
		if name == "" {
			return "", fmt.Errorf("batch: call %d: tool name is required", i)
		}
		if !IsReadOnly(name) {
			return "", fmt.Errorf("batch: call %d: %q is not read-only; batch only executes read-only tools", i, name)
		}
		inner, ok := tc.Registry.Get(name)
		if !ok {
			return "", fmt.Errorf("batch: call %d: unknown tool %q", i, name)
		}
		if err := ValidateToolInput(inner, call.Input); err != nil {
			return "", fmt.Errorf("batch: call %d (%s): %w", i, name, err)
		}
		out, err := inner.Execute(ctx, call.Input)
		if err != nil {
			return "", fmt.Errorf("batch: call %d (%s): %w", i, name, err)
		}
		fmt.Fprintf(&b, "## %s\n%s\n\n", name, strings.TrimSpace(out))
	}
	return strings.TrimRight(b.String(), "\n"), nil
}
