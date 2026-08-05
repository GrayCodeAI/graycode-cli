package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type SpecAdrTool struct{}

func (SpecAdrTool) Name() string { return "SpecAdr" }
func (SpecAdrTool) Aliases() []string {
	return []string{"spec_adr", "spec:adr"}
}

func (SpecAdrTool) Description() string {
	return "Create and manage Architecture Decision Records. Documents key technical decisions with context, options considered, rationale, and consequences. Links decisions to requirements they satisfy."
}

func (SpecAdrTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"action": map[string]interface{}{
				"type":        "string",
				"description": "Action: create (new ADR), list (show all), link (link to requirement)",
				"enum":        []string{"create", "list", "link"},
			},
			"title": map[string]interface{}{
				"type":        "string",
				"description": "ADR title (required for create)",
			},
			"req_id": map[string]interface{}{
				"type":        "string",
				"description": "REQ ID to link to (required for link)",
			},
		},
	}
}

func (SpecAdrTool) Execute(ctx context.Context, input json.RawMessage) (string, error) {
	var p struct {
		Action string `json:"action"`
		Title  string `json:"title"`
		ReqID  string `json:"req_id"`
	}
	if err := json.Unmarshal(input, &p); err != nil {
		return "", err
	}
	if p.Action == "" {
		p.Action = "list"
	}

	dir, err := specDir(ctx)
	if err != nil {
		return "", err
	}

	switch p.Action {
	case "create":
		return createAdr(dir, p.Title)
	case "list":
		return listAdrs(dir)
	case "link":
		return linkAdr(dir, p.ReqID)
	default:
		return "", fmt.Errorf("unknown action %q", p.Action)
	}
}

func createAdr(dir, title string) (string, error) {
	if title == "" {
		return "", fmt.Errorf("title is required for create")
	}

	adrDir := filepath.Join(dir, "adr")
	_ = os.MkdirAll(adrDir, 0o700)

	entries, _ := os.ReadDir(adrDir)
	num := len(entries) + 1

	filename := fmt.Sprintf("%04d-%s.md", num, strings.ToLower(strings.ReplaceAll(title, " ", "-")))
	path := filepath.Join(adrDir, filename)

	content := fmt.Sprintf(`# ADR %04d: %s

## Status

Proposed

## Context

What is the issue that were seeing that motivates this decision?

## Decision

What is the change that were proposing and/or doing?

## Alternatives Considered

| Option | Pros | Cons |
|--------|------|------|
| Option 1 | | |
| Option 2 | | |

## Consequences

### Positive

-

### Negative

-

### Neutral

-

## Requirements

<!-- Link to requirements this decision supports -->
<!-- Example: - REQ-001.1 -->

## References

<!-- Links to related specs, docs, discussions -->

`, num, title)

	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		return "", fmt.Errorf("write ADR: %w", err)
	}

	return fmt.Sprintf("Created ADR: %s", path), nil
}

func listAdrs(dir string) (string, error) {
	adrDir := filepath.Join(dir, "adr")
	entries, err := os.ReadDir(adrDir)
	if err != nil || len(entries) == 0 {
		return "No ADRs found. Use action='create' to create one.", nil
	}

	var b strings.Builder
	b.WriteString("## Architecture Decision Records\n\n")
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".md") {
			fmt.Fprintf(&b, "- `%s`\n", e.Name())
		}
	}
	return strings.TrimSpace(b.String()), nil
}

func linkAdr(dir, reqID string) (string, error) {
	if reqID == "" {
		return "", fmt.Errorf("req_id is required for link")
	}
	return fmt.Sprintf("To link requirement %s to an ADR, edit the ADR file and add it under the Requirements section.", reqID), nil
}

func init() {
	_ = SpecAdrTool{}
}
