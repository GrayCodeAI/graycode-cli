package tool

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestCodeSearchTool_NoSearchFn(t *testing.T) {
	cs := CodeSearchTool{}

	// Verify basic metadata
	if cs.Name() != "CodeSearch" {
		t.Errorf("expected name CodeSearch, got %q", cs.Name())
	}
	aliases := cs.Aliases()
	if len(aliases) != 2 || aliases[0] != "code_search" || aliases[1] != "search_code" {
		t.Errorf("unexpected aliases: %v", aliases)
	}

	// Execute without CodeSearchFn should return graceful message
	input, _ := json.Marshal(map[string]interface{}{"query": "test query"})
	ctx := context.Background()
	result, err := cs.Execute(ctx, input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "not available") {
		t.Errorf("expected 'not available' message, got %q", result)
	}
}

func TestCodeSearchTool_WithResults(t *testing.T) {
	cs := CodeSearchTool{}

	mockResults := []CodeSearchResult{
		{
			Path:      "handler.go",
			StartLine: 10,
			EndLine:   20,
			Content:   "func HandleRequest() {}",
			Symbol:    "HandleRequest",
			Language:  "go",
			Score:     0.9,
		},
		{
			Path:      "auth.py",
			StartLine: 5,
			EndLine:   15,
			Content:   "def authenticate(token):\n    pass",
			Symbol:    "authenticate",
			Language:  "python",
			Score:     0.7,
		},
	}

	tc := &ToolContext{
		CodeSearchFn: func(ctx context.Context, query string, limit int) ([]CodeSearchResult, error) {
			return mockResults, nil
		},
	}

	ctx := WithToolContext(context.Background(), tc)
	input, _ := json.Marshal(map[string]interface{}{"query": "handle request", "limit": 5})
	result, err := cs.Execute(ctx, input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should contain both results
	if !strings.Contains(result, "handler.go:10-20") {
		t.Errorf("expected handler.go path in output, got %q", result)
	}
	if !strings.Contains(result, "auth.py:5-15") {
		t.Errorf("expected auth.py path in output, got %q", result)
	}
	if !strings.Contains(result, "```go") {
		t.Errorf("expected go code block in output, got %q", result)
	}
	if !strings.Contains(result, "```python") {
		t.Errorf("expected python code block in output, got %q", result)
	}
}
