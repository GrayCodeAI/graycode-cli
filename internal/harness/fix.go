package harness

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// FixResult summarizes the repairs performed by FixWorkspaceHarness.
type FixResult struct {
	RepairsPerformed []string `json:"repairs_performed"`
	FilesCreated     []string `json:"files_created"`
}

// FixWorkspaceHarness attempts to automatically repair missing or partial harness assets.
func FixWorkspaceHarness(ctx context.Context, targetPath string, report *HarnessReport) (*FixResult, error) {
	if report == nil {
		var err error
		report, err = EvaluateWorkspace(ctx, targetPath, EvaluateOptions{TargetPath: targetPath})
		if err != nil {
			return nil, fmt.Errorf("failed to evaluate workspace before repair: %w", err)
		}
	}

	result := &FixResult{
		RepairsPerformed: make([]string, 0),
		FilesCreated:     make([]string, 0),
	}

	root := report.TargetPath

	// 1. Repair missing AGENTS.md
	if !report.Assets.AgentsMD {
		agentsPath := filepath.Join(root, "AGENTS.md")
		template := buildAgentsMDTemplate(report.Assets)
		if err := os.WriteFile(agentsPath, []byte(template), 0o644); err == nil {
			result.RepairsPerformed = append(result.RepairsPerformed, "Created baseline AGENTS.md with build, lint, and test conventions")
			result.FilesCreated = append(result.FilesCreated, agentsPath)
		}
	}

	// 2. Repair missing .zero/skills/ directory & starter skill
	skillsDir := filepath.Join(root, ".zero", "skills")
	if !dirExists(skillsDir) {
		if err := os.MkdirAll(skillsDir, 0o755); err == nil {
			starterSkillDir := filepath.Join(skillsDir, "code-review")
			_ = os.MkdirAll(starterSkillDir, 0o755)
			starterSkillFile := filepath.Join(starterSkillDir, "SKILL.md")
			skillContent := `---
description: Standard Go and Project Code Review Conventions
globs: "*.go"
alwaysApply: true
---

# Code Review Conventions

1. Ensure all public functions have clear godoc comments.
2. Check for unhandled error returns.
3. Confirm tests exist alongside modified source files (*_test.go).
`
			if err := os.WriteFile(starterSkillFile, []byte(skillContent), 0o644); err == nil {
				result.RepairsPerformed = append(result.RepairsPerformed, "Created .zero/skills/ directory and starter code-review skill")
				result.FilesCreated = append(result.FilesCreated, starterSkillFile)
			}
		}
	}

	// 3. Repair missing .hawk/specs/ directory
	specsDir := filepath.Join(root, ".hawk", "specs")
	if !dirExists(specsDir) {
		if err := os.MkdirAll(specsDir, 0o755); err == nil {
			result.RepairsPerformed = append(result.RepairsPerformed, "Created .hawk/specs/ directory for task specification management")
		}
	}

	return result, nil
}

func buildAgentsMDTemplate(assets AssetsDetected) string {
	var sb strings.Builder
	sb.WriteString("# Project Conventions & Agent Guidelines\n\n")
	sb.WriteString("## Development Workflow\n\n")
	if sliceContains(assets.TestRunners, "make test") {
		sb.WriteString("- Run tests with `make test` before opening PRs.\n")
	} else if sliceContains(assets.TestRunners, "go test") {
		sb.WriteString("- Run tests with `go test ./...` before committing changes.\n")
	} else if sliceContains(assets.TestRunners, "npm test") {
		sb.WriteString("- Run tests with `npm test` before committing changes.\n")
	} else {
		sb.WriteString("- Run project test suite before opening PRs.\n")
	}

	if sliceContains(assets.Linters, "golangci-lint") || sliceContains(assets.Linters, "make lint") {
		sb.WriteString("- Execute `make lint` or `golangci-lint run` to verify formatting and lint rules.\n")
	} else {
		sb.WriteString("- Ensure code passes standard linter checks prior to review.\n")
	}

	sb.WriteString("- Keep unit tests next to source files (`foo_test.go` next to `foo.go`).\n")
	sb.WriteString("- Do not edit vendored or third-party files.\n")

	return sb.String()
}
