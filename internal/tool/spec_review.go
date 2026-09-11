package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/GrayCodeAI/hawk/internal/spec"
)

type SpecReviewTool struct{}

func (SpecReviewTool) Name() string { return "SpecReview" }
func (SpecReviewTool) Aliases() []string {
	return []string{"spec_review", "spec:review"}
}

func (SpecReviewTool) Description() string {
	return "Post-implementation review against specs."
}

func (SpecReviewTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"scope": map[string]interface{}{
				"type":        "string",
				"description": "Review scope: spec, diff, full",
				"enum":        []string{"spec", "diff", "full"},
			},
		},
	}
}

func (SpecReviewTool) Execute(ctx context.Context, input json.RawMessage) (string, error) {
	var p struct {
		Scope string `json:"scope"`
	}
	if err := json.Unmarshal(input, &p); err != nil {
		return "", err
	}
	if p.Scope == "" {
		p.Scope = "spec"
	}

	dir, err := specDir(ctx)
	if err != nil {
		return "", err
	}

	if p.Scope == "spec" {
		return reviewAgainstSpec(dir)
	} else if p.Scope == "diff" {
		return reviewDiff(dir)
	}
	return reviewAgainstSpec(dir)
}

func reviewAgainstSpec(dir string) (string, error) {
	specContent := readFileStr(filepath.Join(dir, "spec.md"))
	if specContent == "" {
		return "No spec.md found.", nil
	}

	reqs := spec.ExtractReqIDs(specContent)

	var b strings.Builder
	b.WriteString("## Spec Compliance Review\n\n")
	fmt.Fprintf(&b, "**%d requirements** to verify\n\n", len(reqs))

	for _, req := range reqs {
		fmt.Fprintf(&b, "- `%s`\n", req.Raw)
	}

	return strings.TrimSpace(b.String()), nil
}

func reviewDiff(dir string) (string, error) {
	cmd := exec.Command("git", "diff", "--stat")
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Sprintf("git diff failed: %v", err), nil
	}

	var b strings.Builder
	b.WriteString("## Diff Review\n\n")
	b.WriteString("```\n")
	b.Write(output)
	b.WriteString("\n```\n")

	return strings.TrimSpace(b.String()), nil
}

func init() {
	_ = SpecReviewTool{}
}
