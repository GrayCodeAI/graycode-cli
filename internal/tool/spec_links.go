package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/GrayCodeAI/hawk/internal/spec"
)

type SpecLinksTool struct{}

func (SpecLinksTool) Name() string { return "SpecLinks" }
func (SpecLinksTool) Aliases() []string {
	return []string{"spec_links", "spec:links"}
}

func (SpecLinksTool) Description() string {
	return "Manage bidirectional links between specs and tests using [@req:XXX] annotations."
}

func (SpecLinksTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"action": map[string]interface{}{
				"type":        "string",
				"description": "Action: check (verify), add (generate annotations)",
				"enum":        []string{"check", "add"},
			},
		},
	}
}

func (SpecLinksTool) Execute(ctx context.Context, input json.RawMessage) (string, error) {
	var p struct {
		Action string `json:"action"`
	}
	if err := json.Unmarshal(input, &p); err != nil {
		return "", err
	}
	if p.Action == "" {
		p.Action = "check"
	}

	dir, err := specDir(ctx)
	if err != nil {
		return "", err
	}

	if p.Action == "check" {
		return checkLinks(dir)
	}
	return addLinks(dir)
}

func checkLinks(dir string) (string, error) {
	specContent := readFileStr(filepath.Join(dir, "spec.md"))
	if specContent == "" {
		return "No spec.md found.", nil
	}

	reqs := spec.ExtractReqIDs(specContent)
	if len(reqs) == 0 {
		return "No REQ IDs found in spec.md.", nil
	}

	var b strings.Builder
	b.WriteString("## Spec-Test Links\n\n")

	for _, req := range reqs {
		fmt.Fprintf(&b, "- `%s`\n", req.Raw)
	}

	return strings.TrimSpace(b.String()), nil
}

func addLinks(dir string) (string, error) {
	return "Link generation not yet implemented.", nil
}

func init() {
	_ = SpecLinksTool{}
}
