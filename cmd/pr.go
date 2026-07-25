package cmd

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

var (
	prBase         string
	prPostComments bool
	prDraft        bool
	prTitle        string
	prUpdate       bool
)

var prCmd = &cobra.Command{
	Use:   "pr",
	Short: "AI-powered pull request workflow",
	Long: `Manage pull requests with AI assistance.

Subcommands:
  review    Review a PR or current branch diff
  create    Create a new PR with generated description
  describe  Generate or update a PR description

Examples:
  hawk pr review
  hawk pr review 42 --post-comments
  hawk pr create --base develop --draft
  hawk pr describe 42 --update`,
}

var prReviewCmd = &cobra.Command{
	Use:   "review [number]",
	Short: "Review a pull request or current branch diff",
	Long: `Review code changes using structured analysis.

If a PR number is provided, fetches the diff from GitHub.
Otherwise, reviews the diff between the base branch and HEAD.`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireGH(); err != nil {
			return err
		}

		var diff string
		var prNumber int
		var err error

		if len(args) > 0 {
			prNumber, err = strconv.Atoi(args[0])
			if err != nil {
				return fmt.Errorf("invalid PR number %q: %w", args[0], err)
			}
			diff, err = ghPRDiff(prNumber)
			if err != nil {
				return fmt.Errorf("failed to get PR diff: %w", err)
			}
		} else {
			diff, err = gitDiffBase(prBase)
			if err != nil {
				return fmt.Errorf("failed to get branch diff: %w", err)
			}
		}

		if strings.TrimSpace(diff) == "" {
			cmd.Println("No changes found.")
			return nil
		}

		review := formatReview(diff, prNumber)
		cmd.Print(review)

		if prPostComments && prNumber > 0 {
			if err := ghPRComment(prNumber, review); err != nil {
				return fmt.Errorf("failed to post comment: %w", err)
			}
			cmd.Println("\nReview posted as comment on PR #" + strconv.Itoa(prNumber))
		}

		return nil
	},
}

var prCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a pull request with AI-generated description",
	Long: `Analyzes commits since the base branch, generates a title and body,
then creates a pull request via the GitHub CLI.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireGH(); err != nil {
			return err
		}

		base := prBase

		// Get commits
		commits, err := gitLogOneline(base)
		if err != nil {
			return fmt.Errorf("failed to get commits: %w", err)
		}
		if strings.TrimSpace(commits) == "" {
			return fmt.Errorf("no commits found between %s and HEAD", base)
		}

		// Get diff
		diff, err := gitDiffBase(base)
		if err != nil {
			return fmt.Errorf("failed to get diff: %w", err)
		}

		// Generate title and body
		title := prTitle
		if title == "" {
			title = generatePRTitle(commits)
		}
		body := generatePRBody(commits, diff)

		// Build gh pr create command
		ghArgs := []string{"pr", "create", "--title", title, "--body", body}
		if prDraft {
			ghArgs = append(ghArgs, "--draft")
		}
		ghArgs = append(ghArgs, "--base", base)

		ctx := context.Background()
		ghCmd := exec.CommandContext(ctx, "gh", ghArgs...) // #nosec G204 -- fixed command 'gh' with args, not user-controlled binary
		ghCmd.Stderr = os.Stderr
		out, err := ghCmd.Output()
		if err != nil {
			return fmt.Errorf("gh pr create failed: %w", err)
		}

		prURL := strings.TrimSpace(string(out))
		cmd.Println("Pull request created: " + prURL)
		return nil
	},
}

var prDescribeCmd = &cobra.Command{
	Use:   "describe [number]",
	Short: "Generate or update a pull request description",
	Long: `Fetches the diff for a pull request and generates a structured description.
Use --update to write the description back to the PR.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireGH(); err != nil {
			return err
		}

		prNumber, err := strconv.Atoi(args[0])
		if err != nil {
			return fmt.Errorf("invalid PR number %q: %w", args[0], err)
		}

		diff, err := ghPRDiff(prNumber)
		if err != nil {
			return fmt.Errorf("failed to get PR diff: %w", err)
		}
		if strings.TrimSpace(diff) == "" {
			cmd.Println("No changes found in PR #" + strconv.Itoa(prNumber))
			return nil
		}

		description := generateDescription(diff)
		cmd.Print(description)

		if prUpdate {
			ctx := context.Background()
			ghCmd := exec.CommandContext(ctx, "gh", "pr", "edit", strconv.Itoa(prNumber), "--body", description) // #nosec G204 -- fixed command 'gh' with args, not user-controlled binary
			ghCmd.Stderr = os.Stderr
			if err := ghCmd.Run(); err != nil {
				return fmt.Errorf("failed to update PR description: %w", err)
			}
			cmd.Println("\nPR #" + strconv.Itoa(prNumber) + " description updated.")
		}

		return nil
	},
}

func init() {
	// Review flags
	prReviewCmd.Flags().StringVar(&prBase, "base", "main", "base branch for diff comparison")
	prReviewCmd.Flags().BoolVar(&prPostComments, "post-comments", false, "post review findings as a PR comment")

	// Create flags
	prCreateCmd.Flags().StringVar(&prBase, "base", "main", "base branch for the pull request")
	prCreateCmd.Flags().BoolVar(&prDraft, "draft", false, "create as draft pull request")
	prCreateCmd.Flags().StringVar(&prTitle, "title", "", "override the generated title")

	// Describe flags
	prDescribeCmd.Flags().StringVar(&prBase, "base", "main", "base branch for diff comparison")
	prDescribeCmd.Flags().BoolVar(&prUpdate, "update", false, "update the PR description on GitHub")

	// Register subcommands
	prCmd.AddCommand(prReviewCmd)
	prCmd.AddCommand(prCreateCmd)
	prCmd.AddCommand(prDescribeCmd)

	// Register parent command
	rootCmd.AddCommand(prCmd)
}

// requireGH checks that the gh CLI is available on the system PATH.
func requireGH() error {
	_, err := exec.LookPath("gh")
	if err != nil {
		return fmt.Errorf("GitHub CLI (gh) not found in PATH; install it from https://cli.github.com")
	}
	return nil
}

// ghPRDiff fetches the diff for a given PR number using gh.
func ghPRDiff(number int) (string, error) {
	ctx := context.Background()
	cmd := exec.CommandContext(ctx, "gh", "pr", "diff", strconv.Itoa(number)) // #nosec G204 -- fixed command 'gh' with args, not user-controlled binary
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// ghPRComment posts a comment on a PR.
func ghPRComment(number int, body string) error {
	ctx := context.Background()
	cmd := exec.CommandContext(ctx, "gh", "pr", "comment", strconv.Itoa(number), "--body", body) // #nosec G204 -- fixed command 'gh' with args, not user-controlled binary
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// gitDiffBase returns the diff between the base branch and HEAD.
func gitDiffBase(base string) (string, error) {
	ctx := context.Background()
	cmd := exec.CommandContext(ctx, "git", "diff", base+"...HEAD") // #nosec G204 -- fixed command 'git' with args, not user-controlled binary
	out, err := cmd.Output()
	if err != nil {
		// Fallback to two-dot diff
		cmd = exec.CommandContext(ctx, "git", "diff", base, "HEAD")
		out, err = cmd.Output()
		if err != nil {
			return "", err
		}
	}
	return string(out), nil
}

// gitLogOneline returns the oneline log between base and HEAD.
func gitLogOneline(base string) (string, error) {
	ctx := context.Background()
	cmd := exec.CommandContext(ctx, "git", "log", base+"...HEAD", "--oneline") // #nosec G204 -- fixed command 'git' with args, not user-controlled binary
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// formatReview produces a structured review output from a diff.
func formatReview(diff string, prNumber int) string {
	var b strings.Builder

	if prNumber > 0 {
		b.WriteString(fmt.Sprintf("## PR Review: #%d\n\n", prNumber))
	} else {
		b.WriteString("## PR Review: current branch\n\n")
	}

	findings := analyzeDiff(diff)

	b.WriteString("### Findings\n\n")
	if len(findings) == 0 {
		b.WriteString("No issues detected.\n")
	} else {
		for i, f := range findings {
			b.WriteString(fmt.Sprintf("%d. [%s] %s:%d — %s\n", i+1, f.severity, f.file, f.line, f.description))
		}
	}

	b.WriteString("\n### Summary\n\n")
	b.WriteString(generateReviewSummary(diff, findings))
	b.WriteString("\n")

	return b.String()
}

type finding struct {
	severity    string
	file        string
	line        int
	description string
}

// analyzeDiff performs a basic static analysis pass over a unified diff.
func analyzeDiff(diff string) []finding {
	var findings []finding

	lines := strings.Split(diff, "\n")
	currentFile := ""
	currentLine := 0

	for _, line := range lines {
		// Track current file
		if strings.HasPrefix(line, "+++ b/") {
			currentFile = strings.TrimPrefix(line, "+++ b/")
			currentLine = 0
			continue
		}

		// Track line numbers from hunk headers
		if strings.HasPrefix(line, "@@") {
			// Parse @@ -a,b +c,d @@
			parts := strings.Split(line, "+")
			if len(parts) >= 2 {
				numStr := strings.Split(parts[1], ",")[0]
				if n, err := strconv.Atoi(numStr); err == nil {
					currentLine = n
					continue
				}
			}
		}

		// Only analyze added lines
		if !strings.HasPrefix(line, "+") || strings.HasPrefix(line, "+++") {
			if strings.HasPrefix(line, "+") || strings.HasPrefix(line, " ") {
				currentLine++
			}
			continue
		}

		added := line[1:]
		currentLine++

		// Check for common issues
		if f := checkLine(added, currentFile, currentLine); f != nil {
			findings = append(findings, *f)
		}
	}

	return findings
}

// checkLine applies heuristic checks to a single added line.
func checkLine(line, file string, lineNum int) *finding {
	trimmed := strings.TrimSpace(line)

	// TODO/FIXME/HACK markers
	upper := strings.ToUpper(trimmed)
	if strings.Contains(upper, "TODO") || strings.Contains(upper, "FIXME") || strings.Contains(upper, "HACK") {
		return &finding{
			severity:    "info",
			file:        file,
			line:        lineNum,
			description: "Contains TODO/FIXME/HACK marker",
		}
	}

	// Hardcoded secrets patterns
	lower := strings.ToLower(trimmed)
	if (strings.Contains(lower, "password") || strings.Contains(lower, "secret") ||
		strings.Contains(lower, "api_key") || strings.Contains(lower, "apikey")) &&
		(strings.Contains(trimmed, "=") || strings.Contains(trimmed, ":")) &&
		!strings.HasPrefix(trimmed, "//") && !strings.HasPrefix(trimmed, "#") &&
		!strings.HasPrefix(trimmed, "*") {
		return &finding{
			severity:    "high",
			file:        file,
			line:        lineNum,
			description: "Possible hardcoded secret or credential",
		}
	}

	// Panic in Go code
	if strings.HasSuffix(file, ".go") && strings.Contains(trimmed, "panic(") &&
		!strings.HasPrefix(trimmed, "//") {
		return &finding{
			severity:    "medium",
			file:        file,
			line:        lineNum,
			description: "Use of panic() — consider returning an error instead",
		}
	}

	// Unchecked error in Go (basic heuristic)
	if strings.HasSuffix(file, ".go") && strings.Contains(trimmed, ", _ =") &&
		(strings.Contains(trimmed, "os.") || strings.Contains(trimmed, "io.") ||
			strings.Contains(trimmed, "http.") || strings.Contains(trimmed, "exec.")) {
		return &finding{
			severity:    "medium",
			file:        file,
			line:        lineNum,
			description: "Discarded error from I/O or system call",
		}
	}

	// fmt.Println in non-test Go code (potential debug leftover)
	if strings.HasSuffix(file, ".go") && !strings.HasSuffix(file, "_test.go") &&
		strings.Contains(trimmed, "fmt.Println(") &&
		!strings.HasPrefix(trimmed, "//") {
		return &finding{
			severity:    "low",
			file:        file,
			line:        lineNum,
			description: "fmt.Println may be a debug leftover — consider using structured logging",
		}
	}

	return nil
}

// generateReviewSummary produces an overall assessment of the diff.
func generateReviewSummary(diff string, findings []finding) string {
	lines := strings.Split(diff, "\n")
	addedCount := 0
	removedCount := 0
	filesChanged := make(map[string]bool)

	for _, line := range lines {
		if strings.HasPrefix(line, "+++ b/") {
			filesChanged[strings.TrimPrefix(line, "+++ b/")] = true
		} else if strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++") {
			addedCount++
		} else if strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "---") {
			removedCount++
		}
	}

	var b strings.Builder
	b.WriteString(fmt.Sprintf("Reviewed %d file(s) with %d addition(s) and %d deletion(s).\n",
		len(filesChanged), addedCount, removedCount))

	if len(findings) == 0 {
		b.WriteString("No issues detected. The changes look clean.")
	} else {
		sevCounts := make(map[string]int)
		for _, f := range findings {
			sevCounts[f.severity]++
		}
		b.WriteString(fmt.Sprintf("Found %d issue(s):", len(findings)))
		for sev, count := range sevCounts {
			b.WriteString(fmt.Sprintf(" %s=%d", sev, count))
		}
		b.WriteString(".")
	}

	return b.String()
}

// generatePRTitle creates a title from the commit log output.
func generatePRTitle(commits string) string {
	lines := strings.Split(strings.TrimSpace(commits), "\n")
	if len(lines) == 0 {
		return "Update"
	}

	if len(lines) == 1 {
		// Single commit — use its message as the title (strip hash)
		parts := strings.SplitN(lines[0], " ", 2)
		if len(parts) == 2 {
			title := parts[1]
			if len(title) > 72 {
				if runes := []rune(title); len(runes) > 72 {
					title = string(runes[:69]) + "..."
				}
			}
			return title
		}
		return lines[0]
	}

	// Multiple commits — summarize
	// Try to detect a common conventional commit prefix
	types := make(map[string]int)
	for _, line := range lines {
		parts := strings.SplitN(line, " ", 2)
		if len(parts) < 2 {
			continue
		}
		msg := parts[1]
		if colonIdx := strings.Index(msg, ":"); colonIdx > 0 && colonIdx < 20 {
			prefix := msg[:colonIdx]
			prefix = strings.TrimSuffix(prefix, "!")
			if parenIdx := strings.Index(prefix, "("); parenIdx > 0 {
				prefix = prefix[:parenIdx]
			}
			prefix = strings.TrimSpace(prefix)
			if isValidCommitType(prefix) {
				types[prefix]++
			}
		}
	}

	if len(types) == 1 {
		for t := range types {
			title := fmt.Sprintf("%s: %d changes", t, len(lines))
			return title
		}
	}
	if len(types) > 1 {
		var typeNames []string
		for t := range types {
			typeNames = append(typeNames, t)
		}
		title := fmt.Sprintf("Multiple changes (%s)", strings.Join(typeNames, ", "))
		if len(title) > 72 {
			if runes := []rune(title); len(runes) > 72 {
				title = string(runes[:69]) + "..."
			}
		}
		return title
	}

	// Fallback: use first commit message
	parts := strings.SplitN(lines[0], " ", 2)
	if len(parts) == 2 {
		title := parts[1]
		if len(title) > 72 {
			if runes := []rune(title); len(runes) > 72 {
				title = string(runes[:69]) + "..."
			}
		}
		return title
	}
	return fmt.Sprintf("%d commits", len(lines))
}

func isValidCommitType(t string) bool {
	valid := []string{"feat", "fix", "refactor", "test", "docs", "style", "chore", "perf", "ci", "build"}
	for _, v := range valid {
		if t == v {
			return true
		}
	}
	return false
}

// generatePRBody creates a PR body from commits and diff.
func generatePRBody(commits, diff string) string {
	var b strings.Builder

	commitLines := strings.Split(strings.TrimSpace(commits), "\n")

	// Summary section
	b.WriteString("## Summary\n\n")
	if len(commitLines) == 1 {
		parts := strings.SplitN(commitLines[0], " ", 2)
		if len(parts) == 2 {
			b.WriteString(parts[1])
		}
	} else {
		b.WriteString(fmt.Sprintf("This PR includes %d commit(s):\n", len(commitLines)))
		shown := commitLines
		if len(shown) > 15 {
			shown = shown[:15]
		}
		for _, line := range shown {
			parts := strings.SplitN(line, " ", 2)
			if len(parts) == 2 {
				b.WriteString(fmt.Sprintf("- %s\n", parts[1]))
			}
		}
		if len(commitLines) > 15 {
			b.WriteString(fmt.Sprintf("- ... and %d more\n", len(commitLines)-15))
		}
	}
	b.WriteString("\n")

	// Changes section — list files from diff
	b.WriteString("## Changes\n\n")
	filesChanged := extractFilesFromDiff(diff)
	if len(filesChanged) > 0 {
		shown := filesChanged
		if len(shown) > 20 {
			shown = shown[:20]
		}
		for _, f := range shown {
			b.WriteString(fmt.Sprintf("- `%s`\n", f))
		}
		if len(filesChanged) > 20 {
			b.WriteString(fmt.Sprintf("- ... and %d more files\n", len(filesChanged)-20))
		}
	} else {
		b.WriteString("See diff for details.\n")
	}
	b.WriteString("\n")

	// Test plan placeholder
	b.WriteString("## Test Plan\n\n")
	b.WriteString("- [ ] Tests pass locally\n")
	b.WriteString("- [ ] Manual verification of changes\n")

	return b.String()
}

// generateDescription creates a structured description from a PR diff.
func generateDescription(diff string) string {
	var b strings.Builder

	filesChanged := extractFilesFromDiff(diff)
	title := describeChangeTitle(diff, filesChanged)

	b.WriteString("## " + title + "\n\n")

	// Summary
	b.WriteString("### Summary\n\n")
	lines := strings.Split(diff, "\n")
	addedCount := 0
	removedCount := 0
	for _, line := range lines {
		if strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++") {
			addedCount++
		} else if strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "---") {
			removedCount++
		}
	}
	b.WriteString(fmt.Sprintf("Changes across %d file(s): %d addition(s), %d deletion(s).\n\n",
		len(filesChanged), addedCount, removedCount))

	// Changes section
	b.WriteString("### Changes\n\n")
	if len(filesChanged) > 0 {
		shown := filesChanged
		if len(shown) > 25 {
			shown = shown[:25]
		}
		for _, f := range shown {
			desc := describeFileByPath(f)
			if desc != "" {
				b.WriteString(fmt.Sprintf("- `%s` — %s\n", f, desc))
			} else {
				b.WriteString(fmt.Sprintf("- `%s`\n", f))
			}
		}
		if len(filesChanged) > 25 {
			b.WriteString(fmt.Sprintf("- ... and %d more files\n", len(filesChanged)-25))
		}
	}
	b.WriteString("\n")

	return b.String()
}

// extractFilesFromDiff parses file paths from a unified diff.
func extractFilesFromDiff(diff string) []string {
	var files []string
	seen := make(map[string]bool)
	for _, line := range strings.Split(diff, "\n") {
		if strings.HasPrefix(line, "+++ b/") {
			f := strings.TrimPrefix(line, "+++ b/")
			if !seen[f] {
				seen[f] = true
				files = append(files, f)
			}
		}
	}
	return files
}

// describeChangeTitle generates a title from the diff content.
func describeChangeTitle(diff string, files []string) string {
	if len(files) == 0 {
		return "Update"
	}
	if len(files) == 1 {
		return fmt.Sprintf("Update %s", files[0])
	}

	// Try to find a common directory
	dirs := make(map[string]int)
	for _, f := range files {
		parts := strings.Split(f, "/")
		if len(parts) > 1 {
			dirs[parts[0]]++
		}
	}

	maxDir := ""
	maxCount := 0
	for d, c := range dirs {
		if c > maxCount {
			maxCount = c
			maxDir = d
		}
	}

	if maxDir != "" && maxCount > len(files)/2 {
		return fmt.Sprintf("Update %s (%d files)", maxDir, len(files))
	}

	return fmt.Sprintf("Update %d files", len(files))
}

// describeFileByPath returns a brief description based on file path heuristics.
func describeFileByPath(path string) string {
	base := strings.ToLower(path)

	if strings.HasSuffix(base, "_test.go") || strings.Contains(base, "test") {
		return "Tests"
	}
	if strings.HasSuffix(base, ".md") {
		return "Documentation"
	}
	if strings.Contains(base, "config") || strings.Contains(base, "setting") {
		return "Configuration"
	}
	if strings.Contains(base, "handler") || strings.Contains(base, "api") || strings.Contains(base, "route") {
		return "API"
	}
	if strings.Contains(base, "model") || strings.Contains(base, "schema") || strings.Contains(base, "migration") {
		return "Data model"
	}
	if strings.HasSuffix(base, "go.mod") || strings.HasSuffix(base, "go.sum") ||
		strings.HasSuffix(base, "package.json") || strings.HasSuffix(base, "package-lock.json") {
		return "Dependencies"
	}
	return ""
}
