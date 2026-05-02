package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// CodeSearchTool searches the codebase semantically.
type CodeSearchTool struct{}

func (CodeSearchTool) Name() string { return "CodeSearch" }

func (CodeSearchTool) Aliases() []string { return []string{"code_search", "search_code"} }

func (CodeSearchTool) Description() string {
	return "Search the codebase semantically by meaning, not just text matching."
}

func (CodeSearchTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"query": map[string]interface{}{
				"type":        "string",
				"description": "The semantic search query describing what you are looking for.",
			},
			"limit": map[string]interface{}{
				"type":        "integer",
				"description": "Maximum number of results to return (default 5).",
			},
			"language": map[string]interface{}{
				"type":        "string",
				"description": "Optional language filter (e.g. go, python, typescript).",
			},
		},
		"required": []string{"query"},
	}
}

func (CodeSearchTool) Execute(ctx context.Context, input json.RawMessage) (string, error) {
	var params struct {
		Query    string `json:"query"`
		Limit    int    `json:"limit"`
		Language string `json:"language"`
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return "", fmt.Errorf("invalid input: %w", err)
	}

	if params.Query == "" {
		return "", fmt.Errorf("query is required")
	}
	if params.Limit <= 0 {
		params.Limit = 5
	}

	tc := GetToolContext(ctx)
	if tc == nil || tc.CodeSearchFn == nil {
		return "Code search is not available in this session.", nil
	}

	results, err := tc.CodeSearchFn(ctx, params.Query, params.Limit)
	if err != nil {
		return "", fmt.Errorf("code search failed: %w", err)
	}

	if len(results) == 0 {
		return "No results found.", nil
	}

	// Filter by language if specified
	if params.Language != "" {
		lang := strings.ToLower(params.Language)
		var filtered []CodeSearchResult
		for _, r := range results {
			if strings.ToLower(r.Language) == lang {
				filtered = append(filtered, r)
			}
		}
		results = filtered
		if len(results) == 0 {
			return fmt.Sprintf("No results found for language %q.", params.Language), nil
		}
	}

	// Format results
	var sb strings.Builder
	for i, r := range results {
		if i > 0 {
			sb.WriteString("\n---\n")
		}
		lang := r.Language
		if lang == "" {
			lang = "text"
		}
		sb.WriteString(fmt.Sprintf("%s:%d-%d\n```%s\n%s\n```\n", r.Path, r.StartLine, r.EndLine, lang, r.Content))
	}

	return sb.String(), nil
}
