package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// bugFindPrompt generates a comprehensive bug-finding prompt from a list of
// file paths and/or diff content. It instructs the LLM to look for bugs,
// security issues, race conditions, resource leaks, and more.
func bugFindPrompt(filePaths []string, diffContent string) string {
	var b strings.Builder

	b.WriteString(`Analyze the following code for potential issues:
- Bugs and logic errors
- Security vulnerabilities (injection, path traversal, etc.)
- Race conditions and concurrency issues
- Resource leaks (unclosed files, connections, channels)
- Null/nil pointer dereference risks
- Error handling gaps (ignored errors, missing checks)
- Off-by-one errors and boundary conditions

Report each issue with:
- File: the file path
- Line: approximate line number (if available)
- Severity: critical, high, medium, or low
- Description: what the issue is
- Fix suggestion: how to resolve it

`)

	if diffContent != "" {
		b.WriteString("## Diff to analyze\n\n```diff\n")
		b.WriteString(diffContent)
		b.WriteString("\n```\n\n")
	}

	for _, path := range filePaths {
		data, err := os.ReadFile(path)
		if err != nil {
			b.WriteString(fmt.Sprintf("## %s\n\n(could not read: %v)\n\n", path, err))
			continue
		}
		ext := filepath.Ext(path)
		lang := langFromExt(ext)
		b.WriteString(fmt.Sprintf("## %s\n\n```%s\n%s\n```\n\n", path, lang, string(data)))
	}

	return b.String()
}

// langFromExt maps file extensions to Markdown code-fence language identifiers.
func langFromExt(ext string) string {
	switch strings.ToLower(ext) {
	case ".go":
		return "go"
	case ".py":
		return "python"
	case ".js":
		return "javascript"
	case ".ts", ".tsx":
		return "typescript"
	case ".rs":
		return "rust"
	case ".java":
		return "java"
	case ".rb":
		return "ruby"
	case ".c", ".h":
		return "c"
	case ".cpp", ".cc", ".hpp":
		return "cpp"
	default:
		return ""
	}
}

// formatBugReport takes raw LLM output and wraps it in a clean report format
// with severity-based section headers.
func formatBugReport(findings string) string {
	if strings.TrimSpace(findings) == "" {
		return "No issues found."
	}

	var b strings.Builder
	b.WriteString("=== Bug Report ===\n\n")
	b.WriteString(findings)
	b.WriteString("\n")

	// Count severities for a summary line.
	lower := strings.ToLower(findings)
	critical := strings.Count(lower, "critical")
	high := strings.Count(lower, "high")
	medium := strings.Count(lower, "medium")
	low := strings.Count(lower, "low")

	total := critical + high + medium + low
	if total > 0 {
		b.WriteString(fmt.Sprintf("\n--- Summary: %d issue(s) ", total))
		parts := []string{}
		if critical > 0 {
			parts = append(parts, fmt.Sprintf("%d critical", critical))
		}
		if high > 0 {
			parts = append(parts, fmt.Sprintf("%d high", high))
		}
		if medium > 0 {
			parts = append(parts, fmt.Sprintf("%d medium", medium))
		}
		if low > 0 {
			parts = append(parts, fmt.Sprintf("%d low", low))
		}
		b.WriteString("(")
		b.WriteString(strings.Join(parts, ", "))
		b.WriteString(") ---\n")
	}

	return b.String()
}

// gitDiffContent returns the output of `git diff` in the current directory.
func gitDiffContent() (string, error) {
	cmd := exec.Command("git", "diff")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git diff: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

// gitDiffStagedContent returns the output of `git diff --staged` in the current directory.
func gitDiffStagedContent() (string, error) {
	cmd := exec.Command("git", "diff", "--staged")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git diff --staged: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}
