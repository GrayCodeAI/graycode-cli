package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

type SpecVersionTool struct{}

func (SpecVersionTool) Name() string { return "SpecVersion" }
func (SpecVersionTool) Aliases() []string {
	return []string{"spec_version", "spec:version"}
}

func (SpecVersionTool) Description() string {
	return "Stage and commit spec artifacts alongside code changes. Ensures specs are versioned in git with proper REQ references in commit messages. Use after implementation to commit spec + code together."
}

func (SpecVersionTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"message": map[string]interface{}{
				"type":        "string",
				"description": "Custom commit message (optional, auto-generated if empty)",
			},
			"dry_run": map[string]interface{}{
				"type":        "boolean",
				"description": "If true, show what would be committed without actually committing",
			},
			"include_specs": map[string]interface{}{
				"type":        "boolean",
				"description": "If true, include .hawk/specs/ in the commit (default true)",
			},
		},
	}
}

func (SpecVersionTool) Execute(ctx context.Context, input json.RawMessage) (string, error) {
	var p struct {
		Message      string `json:"message"`
		DryRun       bool   `json:"dry_run"`
		IncludeSpecs bool   `json:"include_specs"`
	}
	if input != nil {
		_ = json.Unmarshal(input, &p)
	}
	if !p.IncludeSpecs {
		p.IncludeSpecs = true
	}

	dir, err := specDir(ctx)
	if err != nil {
		return "", err
	}

	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}

	tasksContent := readFileStr(filepath.Join(dir, "tasks.md"))
	specContent := readFileStr(filepath.Join(dir, "spec.md"))

	if p.Message == "" {
		p.Message = generateCommitMessage(tasksContent, specContent)
	}

	specRelPath := filepath.Join(".hawk", "specs")

	var b strings.Builder
	b.WriteString("## Spec Versioning\n\n")

	if p.DryRun {
		b.WriteString("**Dry run** — showing what would be committed:\n\n")
	}

	slug := ""
	if parts := strings.Split(dir, string(os.PathSeparator)); len(parts) > 0 {
		slug = parts[len(parts)-1]
	}
	fmt.Fprintf(&b, "**Spec**: %s\n", slug)
	fmt.Fprintf(&b, "**Message**: %s\n\n", p.Message)

	if p.IncludeSpecs {
		specFiles := []string{"proposal.md", "spec.md", "design.md", "plan.md", "tasks.md", "constitution.md"}
		b.WriteString("### Spec Files\n\n")
		for _, f := range specFiles {
			path := filepath.Join(dir, f)
			if _, err := os.Stat(path); err == nil {
				fmt.Fprintf(&b, "- `%s` OK\n", f)
			}
		}
		b.WriteString("\n")
	}

	if tasksContent != "" {
		total, complete := countTasks(tasksContent)
		if total > 0 {
			fmt.Fprintf(&b, "**Progress**: %d/%d tasks complete\n\n", complete, total)
		}
	}

	if p.DryRun {
		b.WriteString("To commit, call with `dry_run: false`\n")
		return strings.TrimSpace(b.String()), nil
	}

	if output, err := runGitCmd(cwd, "add", specRelPath); err != nil {
		return "", fmt.Errorf("git add failed: %v\n%s", err, output)
	}

	fullMessage := fmt.Sprintf("%s\n\nSpec: %s\nTimestamp: %s", p.Message, slug, time.Now().Format(time.RFC3339))
	if output, err := runGitCmd(cwd, "commit", "-m", fullMessage); err != nil {
		return "", fmt.Errorf("git commit failed: %v\n%s", err, output)
	}

	b.WriteString("**Committed successfully** OK\n")
	return strings.TrimSpace(b.String()), nil
}

func generateCommitMessage(tasksContent, specContent string) string {
	var reqs []string
	reReq := regexp.MustCompile(`REQ-(\d+)(?:\.(\d+))?(?:\.(\d+))?`)
	for _, match := range reReq.FindAllString(specContent, -1) {
		if !sliceContains(reqs, match) {
			reqs = append(reqs, match)
		}
	}

	if len(reqs) > 0 {
		if len(reqs) > 5 {
			return fmt.Sprintf("feat: implement %s and %d more requirements", reqs[0], len(reqs)-1)
		}
		return fmt.Sprintf("feat: implement %s", strings.Join(reqs, ", "))
	}
	return "chore: update spec artifacts"
}

func sliceContains(slice []string, s string) bool {
	for _, v := range slice {
		if v == s {
			return true
		}
	}
	return false
}

func countTasks(content string) (total, complete int) {
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "- [ ]") {
			total++
		} else if strings.HasPrefix(trimmed, "- [x]") || strings.HasPrefix(trimmed, "- [X]") {
			total++
			complete++
		}
	}
	return
}

func runGitCmd(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func init() {
	_ = SpecVersionTool{}
}
