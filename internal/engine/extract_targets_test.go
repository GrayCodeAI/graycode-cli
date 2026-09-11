package engine

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/GrayCodeAI/hawk/internal/types"
)

// fakeToolForSchema is a minimal tool.Tool implementation that returns a
// fixed JSON Schema, used to exercise the schema-aware extraction logic.
type fakeToolForSchema struct {
	name   string
	schema map[string]interface{}
}

func (f fakeToolForSchema) Name() string                       { return f.name }
func (f fakeToolForSchema) Description() string                { return "fake tool for schema tests" }
func (f fakeToolForSchema) Parameters() map[string]interface{} { return f.schema }
func (f fakeToolForSchema) Execute(_ context.Context, _ json.RawMessage) (string, error) {
	return "", nil
}

func TestExtractTargetsFromSchema(t *testing.T) {
	cases := []struct {
		name   string
		schema map[string]interface{}
		call   types.ToolCall
		want   []string
	}{
		{
			name: "conventional file_path",
			schema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"file_path": map[string]interface{}{"type": "string"},
				},
			},
			call: types.ToolCall{Arguments: map[string]interface{}{"file_path": "/tmp/x"}},
			want: []string{"/tmp/x"},
		},
		{
			name: "non-conventional: target_path",
			schema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"target_path": map[string]interface{}{"type": "string"},
				},
			},
			call: types.ToolCall{Arguments: map[string]interface{}{"target_path": "/tmp/y"}},
			want: []string{"/tmp/y"},
		},
		{
			name: "non-conventional: destFile",
			schema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"destFile": map[string]interface{}{"type": "string"},
				},
			},
			call: types.ToolCall{Arguments: map[string]interface{}{"destFile": "/tmp/z"}},
			want: []string{"/tmp/z"},
		},
		{
			name: "description-inferred: backup",
			schema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"backup": map[string]interface{}{
						"type":        "string",
						"description": "Path to the backup file to write",
					},
				},
			},
			call: types.ToolCall{Arguments: map[string]interface{}{"backup": "/tmp/bak"}},
			want: []string{"/tmp/bak"},
		},
		{
			name: "non-string type is skipped",
			schema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"file_path": map[string]interface{}{"type": "integer"},
				},
			},
			call: types.ToolCall{Arguments: map[string]interface{}{"file_path": 42}},
			want: nil,
		},
		{
			name: "non-path arg is skipped",
			schema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"recursive": map[string]interface{}{"type": "boolean"},
					"max_depth": map[string]interface{}{"type": "integer"},
				},
			},
			call: types.ToolCall{Arguments: map[string]interface{}{"recursive": true, "max_depth": 5}},
			want: nil,
		},
		{
			name:   "missing schema falls back to conventional",
			schema: nil,
			call:   types.ToolCall{Arguments: map[string]interface{}{"file_path": "/tmp/fallback"}},
			want:   []string{"/tmp/fallback"},
		},
		{
			name: "multiple path-like args",
			schema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"src_path": map[string]interface{}{"type": "string"},
					"dst_path": map[string]interface{}{"type": "string"},
				},
			},
			call: types.ToolCall{Arguments: map[string]interface{}{
				"src_path": "/tmp/src",
				"dst_path": "/tmp/dst",
			}},
			want: []string{"/tmp/src", "/tmp/dst"},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ft := fakeToolForSchema{name: "Fake", schema: c.schema}
			got := ExtractTargetsFromSchema(ft, c.call)
			if !equalStringSlices(got, c.want) {
				t.Fatalf("ExtractTargetsFromSchema() = %v, want %v", got, c.want)
			}
		})
	}
}

func equalStringSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
