package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// StructuredEditTool applies search-and-replace blocks (structured diffs) to files.
// This is more precise and token-efficient than whole-file writes.
// Inspired by Aider's search/replace edit format.
type StructuredEditTool struct{}

func (StructuredEditTool) Name() string      { return "StructuredEdit" }
func (StructuredEditTool) RiskLevel() string { return "medium" }
func (StructuredEditTool) Aliases() []string { return []string{"sed", "search_replace"} }

func (StructuredEditTool) Description() string {
	return "Apply search-and-replace edits to a file. Provide one or more SEARCH/REPLACE blocks. Each block finds exact text and replaces it. The SEARCH text must match the file contents exactly, including whitespace."
}

func (StructuredEditTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"path": map[string]interface{}{
				"type":        "string",
				"description": "Path to the file to edit",
			},
			"blocks": map[string]interface{}{
				"type":        "array",
				"description": "List of SEARCH/REPLACE blocks",
				"items": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"search": map[string]interface{}{
							"type":        "string",
							"description": "Exact text to find. Must match the file exactly, including whitespace.",
						},
						"replace": map[string]interface{}{
							"type":        "string",
							"description": "Text to replace the search text with.",
						},
					},
					"required": []string{"search", "replace"},
				},
			},
			"auto_format": map[string]interface{}{
				"type":        "boolean",
				"description": "If true, ignore insignificant whitespace differences when matching (default: false)",
			},
		},
		"required": []string{"path", "blocks"},
	}
}

type structuredEditInput struct {
	Path       string               `json:"path"`
	Blocks     []searchReplaceBlock `json:"blocks"`
	AutoFormat bool                 `json:"auto_format"`
}

type searchReplaceBlock struct {
	Search  string `json:"search"`
	Replace string `json:"replace"`
}

func (s StructuredEditTool) Execute(ctx context.Context, input json.RawMessage) (string, error) {
	var p structuredEditInput
	if err := json.Unmarshal(input, &p); err != nil {
		return "", fmt.Errorf("invalid input: %w", err)
	}

	if p.Path == "" {
		return "", fmt.Errorf("path is required")
	}
	if len(p.Blocks) == 0 {
		return "", fmt.Errorf("at least one SEARCH/REPLACE block is required")
	}
	if err := validatePathAllowed(ctx, p.Path); err != nil {
		return "", err
	}

	// Read the file.
	data, err := os.ReadFile(p.Path)
	if err != nil {
		return "", fmt.Errorf("read %s: %w", p.Path, err)
	}
	content := string(data)

	// Apply each block in order.
	applied := 0
	skipped := 0
	for _, block := range p.Blocks {
		search := block.Search
		replace := block.Replace

		if p.AutoFormat {
			search = normalizeWhitespace(search)
		}

		if !strings.Contains(content, search) {
			skipped++
			continue
		}

		if strings.Count(content, search) > 1 {
			return "", fmt.Errorf("search text found %d times in %s — please provide more context to make the match unique", strings.Count(content, search), p.Path)
		}

		content = strings.Replace(content, search, replace, 1)
		applied++
	}

	if applied == 0 {
		return "", fmt.Errorf("no SEARCH/REPLACE blocks matched in %s (%d blocks skipped)", p.Path, skipped)
	}

	// Write the result.
	if err := os.WriteFile(p.Path, []byte(content), 0o600); err != nil {
		return "", fmt.Errorf("write %s: %w", p.Path, err)
	}

	msg := fmt.Sprintf("Applied %d block(s) to %s", applied, p.Path)
	if skipped > 0 {
		msg += fmt.Sprintf(" (%d block(s) skipped — no match found)", skipped)
	}
	if autoCommitEnabled(ctx) {
		_ = AutoCommit(ctx, p.Path, "StructuredEdit", msg)
	}
	return msg, nil
}
