package tool

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
)

// CommitMessageGenerator produces high-quality commit messages using an LLM
// with a rule-based fallback.
type CommitMessageGenerator struct {
	// ChatFn calls the LLM with a prompt and returns the response.
	ChatFn func(ctx context.Context, prompt string) (string, error)

	// FallbackToConventional uses rule-based generation if LLM fails.
	FallbackToConventional bool

	// MaxLength is the maximum subject line length (default 72).
	MaxLength int

	// IncludeBody controls whether a body is appended (default true).
	IncludeBody bool

	// Style is one of "conventional", "descriptive", "concise".
	Style string
}

// CommitContext holds all the information needed to generate a commit message.
type CommitContext struct {
	// Diff is the git diff content.
	Diff string

	// FilesChanged lists the paths of files that changed.
	FilesChanged []string

	// ConversationGoal describes what the user asked for.
	ConversationGoal string

	// PreviousCommits holds the last 3 commit messages for style matching.
	PreviousCommits []string
}

// defaultMaxLength is the standard max subject line length.
const defaultMaxLength = 72

// validCommitTypes lists valid conventional commit types.
var validCommitTypes = []string{
	"feat", "fix", "refactor", "test", "docs", "style", "chore", "perf", "ci", "build",
}

// GenerateMessage uses the LLM to produce a commit message, falling back to
// rule-based generation if the LLM is unavailable or fails.
func (g *CommitMessageGenerator) GenerateMessage(ctx context.Context, commitCtx CommitContext) (string, error) {
	maxLen := g.maxLength()

	if g.ChatFn != nil {
		prompt := g.buildPrompt(commitCtx, maxLen)
		resp, err := g.ChatFn(ctx, prompt)
		if err == nil {
			msg := g.parseResponse(resp, maxLen)
			if msg != "" {
				return msg, nil
			}
		}
		// LLM failed or returned invalid response.
		if !g.FallbackToConventional {
			if err != nil {
				return "", fmt.Errorf("LLM commit message generation failed: %w", err)
			}
			return "", fmt.Errorf("LLM returned invalid commit message")
		}
	}

	// Rule-based fallback.
	msg := GenerateRuleBased(commitCtx)
	if g.IncludeBody {
		body := GenerateBody(commitCtx)
		if body != "" {
			msg += "\n\n" + body
		}
	}
	return msg, nil
}

// GenerateRuleBased produces a conventional commit message using heuristics.
func GenerateRuleBased(commitCtx CommitContext) string {
	commitType := DetectCommitType(commitCtx.Diff, commitCtx.FilesChanged)
	scope := DetectScope(commitCtx.FilesChanged)
	subject := GenerateSubject(commitType, commitCtx.FilesChanged, commitCtx.Diff)

	if scope != "" {
		return fmt.Sprintf("%s(%s): %s", commitType, scope, subject)
	}
	return fmt.Sprintf("%s: %s", commitType, subject)
}

// DetectCommitType analyzes the diff and file list to determine the commit type.
func DetectCommitType(diff string, files []string) string {
	if len(files) == 0 && diff == "" {
		return "chore"
	}

	// Check if only test files changed.
	allTest := len(files) > 0
	allDocs := len(files) > 0
	allConfig := len(files) > 0
	hasNewFiles := false

	for _, f := range files {
		base := filepath.Base(f)
		ext := filepath.Ext(f)

		isTest := strings.HasSuffix(base, "_test.go") ||
			strings.HasSuffix(base, "_test.js") ||
			strings.HasSuffix(base, "_test.ts") ||
			strings.HasSuffix(base, ".test.js") ||
			strings.HasSuffix(base, ".test.ts") ||
			strings.Contains(f, "/test/") ||
			strings.Contains(f, "/tests/")
		if !isTest {
			allTest = false
		}

		isDoc := ext == ".md" || ext == ".txt" || ext == ".rst" || base == "LICENSE"
		if !isDoc {
			allDocs = false
		}

		isConfig := base == "go.mod" || base == "go.sum" ||
			base == "package.json" || base == "package-lock.json" ||
			base == ".gitignore" || base == "Makefile" ||
			base == "Dockerfile" || base == "flake.nix" ||
			ext == ".yml" || ext == ".yaml" || ext == ".toml" ||
			strings.Contains(f, ".github/") || strings.Contains(f, "ci/")
		if !isConfig {
			allConfig = false
		}
	}

	if allTest {
		return "test"
	}
	if allDocs {
		return "docs"
	}
	if allConfig {
		return "chore"
	}

	// Check diff content for patterns.
	lines := strings.Split(diff, "\n")
	addedLines := 0
	removedLines := 0
	for _, line := range lines {
		if strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++") {
			addedLines++
			if strings.Contains(line, "new file mode") {
				hasNewFiles = true
			}
		}
		if strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "---") {
			removedLines++
		}
	}

	// Check for new file indicators in diff header.
	if strings.Contains(diff, "new file mode") {
		hasNewFiles = true
	}

	// Style: only whitespace/formatting changes.
	if isStyleOnly(diff) {
		return "style"
	}

	// Fix: error handling patterns.
	if isFixPattern(diff) {
		return "fix"
	}

	// Feat: new files or new exported functions.
	if hasNewFiles || hasNewExportedSymbols(diff) {
		return "feat"
	}

	// Refactor: similar number of additions and deletions.
	if addedLines > 0 && removedLines > 0 {
		ratio := float64(addedLines) / float64(removedLines)
		if ratio > 0.7 && ratio < 1.4 {
			return "refactor"
		}
	}

	// Default to feat for net additions, fix for modifications.
	if addedLines > removedLines*2 {
		return "feat"
	}
	return "fix"
}

// DetectScope determines the scope from file paths.
func DetectScope(files []string) string {
	if len(files) == 0 {
		return ""
	}

	if len(files) == 1 {
		base := filepath.Base(files[0])
		ext := filepath.Ext(base)
		return strings.TrimSuffix(base, ext)
	}

	// Find common directory prefix.
	dirs := make([]string, 0, len(files))
	for _, f := range files {
		dirs = append(dirs, filepath.Dir(f))
	}

	common := dirs[0]
	for _, d := range dirs[1:] {
		common = commonDirPrefix(common, d)
	}

	// Clean up: remove trailing slash, get last component.
	common = strings.TrimSuffix(common, "/")
	common = strings.TrimSuffix(common, string(filepath.Separator))
	if common == "" || common == "." {
		return ""
	}

	// Use the last path component as scope.
	scope := filepath.Base(common)
	if scope == "." || scope == "/" {
		return ""
	}
	return scope
}

// GenerateSubject creates a commit subject line based on the type, files, and diff.
func GenerateSubject(commitType string, files []string, diff string) string {
	maxLen := defaultMaxLength

	// Reserve space for type and scope prefix (estimate).
	reservedPrefix := len(commitType) + 2 // "type: "
	if len(files) == 1 {
		scope := DetectScope(files)
		if scope != "" {
			reservedPrefix = len(commitType) + len(scope) + 4 // "type(scope): "
		}
	}
	available := maxLen - reservedPrefix

	subject := generateSubjectContent(commitType, files, diff)
	if len(subject) > available {
		// Rune-safe truncation: never split a multibyte UTF-8 sequence.
		if runes := []rune(subject); len(runes) > available {
			subject = string(runes[:available-3]) + "..."
		}
	}
	return subject
}

// GenerateBody creates a commit body with file change descriptions.
func GenerateBody(commitCtx CommitContext) string {
	if len(commitCtx.FilesChanged) == 0 && commitCtx.ConversationGoal == "" {
		return ""
	}

	var parts []string

	if commitCtx.ConversationGoal != "" {
		parts = append(parts, "Goal: "+commitCtx.ConversationGoal)
	}

	if len(commitCtx.FilesChanged) > 0 {
		fileList := "Files changed:"
		shown := commitCtx.FilesChanged
		if len(shown) > 5 {
			shown = shown[:5]
		}
		for _, f := range shown {
			fileList += "\n- " + filepath.Base(f)
		}
		if len(commitCtx.FilesChanged) > 5 {
			fileList += fmt.Sprintf("\n- ... and %d more", len(commitCtx.FilesChanged)-5)
		}
		parts = append(parts, fileList)
	}

	return strings.Join(parts, "\n\n")
}

// ValidateMessage checks a commit message for conventional commit compliance
// and returns a list of warnings.
func ValidateMessage(msg string) []string {
	var warnings []string

	lines := strings.Split(msg, "\n")
	if len(lines) == 0 {
		return []string{"empty commit message"}
	}

	subject := lines[0]

	// Subject line length.
	if len(subject) > 72 {
		warnings = append(warnings, fmt.Sprintf("subject line is %d chars (max 72)", len(subject)))
	}

	// Blank line between subject and body.
	if len(lines) > 1 && lines[1] != "" {
		warnings = append(warnings, "missing blank line between subject and body")
	}

	// Check for valid type prefix.
	colonIdx := strings.Index(subject, ":")
	if colonIdx < 0 {
		warnings = append(warnings, "missing colon separator in subject")
	} else {
		typeStr := subject[:colonIdx]
		// Strip scope: "type(scope)" → "type"
		if parenIdx := strings.Index(typeStr, "("); parenIdx > 0 {
			typeStr = typeStr[:parenIdx]
		}
		if !isValidCommitType(typeStr) {
			warnings = append(warnings, fmt.Sprintf("invalid commit type %q", typeStr))
		}

		// Subject should start lowercase after colon.
		afterColon := strings.TrimSpace(subject[colonIdx+1:])
		if len(afterColon) > 0 && afterColon[0] >= 'A' && afterColon[0] <= 'Z' {
			warnings = append(warnings, "subject should start with lowercase after colon")
		}
	}

	return warnings
}

// --- Internal helpers ---

func (g *CommitMessageGenerator) maxLength() int {
	if g.MaxLength > 0 {
		return g.MaxLength
	}
	return defaultMaxLength
}

func (g *CommitMessageGenerator) buildPrompt(commitCtx CommitContext, maxLen int) string {
	diff := commitCtx.Diff
	if len(diff) > 3000 {
		diff = diff[:3000] + "\n... (truncated)"
	}

	var sb strings.Builder
	sb.WriteString("Generate a git commit message in Conventional Commits format.\n\n")
	sb.WriteString("Rules:\n")
	sb.WriteString(fmt.Sprintf("- Subject line must be at most %d characters\n", maxLen))
	sb.WriteString("- Format: type(scope): subject\n")
	sb.WriteString("- Use lowercase after the colon\n")
	sb.WriteString("- Use imperative mood (e.g., 'add', 'fix', 'update')\n")
	sb.WriteString("- Valid types: feat, fix, refactor, test, docs, style, chore, perf, ci, build\n")

	if g.Style != "" {
		sb.WriteString(fmt.Sprintf("- Style preference: %s\n", g.Style))
	}

	if g.IncludeBody {
		sb.WriteString("- Include a brief body (3-5 lines) after a blank line\n")
	} else {
		sb.WriteString("- Only provide the subject line, no body\n")
	}

	sb.WriteString("\nFiles changed:\n")
	for _, f := range commitCtx.FilesChanged {
		sb.WriteString("- " + f + "\n")
	}

	if commitCtx.ConversationGoal != "" {
		sb.WriteString("\nUser's goal: " + commitCtx.ConversationGoal + "\n")
	}

	if len(commitCtx.PreviousCommits) > 0 {
		sb.WriteString("\nRecent commit messages (for style reference):\n")
		for _, c := range commitCtx.PreviousCommits {
			sb.WriteString("- " + c + "\n")
		}
	}

	sb.WriteString("\nDiff:\n```\n")
	sb.WriteString(diff)
	sb.WriteString("\n```\n")
	sb.WriteString("\nRespond with ONLY the commit message, nothing else.\n")

	return sb.String()
}

func (g *CommitMessageGenerator) parseResponse(resp string, maxLen int) string {
	resp = strings.TrimSpace(resp)

	// Remove markdown code fences if present.
	resp = strings.TrimPrefix(resp, "```")
	resp = strings.TrimSuffix(resp, "```")
	resp = strings.TrimSpace(resp)

	if resp == "" {
		return ""
	}

	lines := strings.Split(resp, "\n")
	subject := lines[0]

	// Basic validation: must have a colon.
	if !strings.Contains(subject, ":") {
		return ""
	}

	// Truncate subject if too long.
	if len(subject) > maxLen {
		subject = subject[:maxLen]
	}

	if len(lines) == 1 {
		return subject
	}

	// Rebuild with proper blank line.
	var result strings.Builder
	result.WriteString(subject)
	if len(lines) > 1 {
		result.WriteString("\n")
		bodyStarted := false
		for i := 1; i < len(lines); i++ {
			if !bodyStarted && lines[i] == "" {
				bodyStarted = true
				result.WriteString("\n")
				continue
			}
			if bodyStarted || lines[i] != "" {
				bodyStarted = true
				if i > 1 || lines[i] != "" {
					result.WriteString(lines[i] + "\n")
				}
			}
		}
	}

	return strings.TrimRight(result.String(), "\n")
}

func generateSubjectContent(commitType string, files []string, diff string) string {
	switch commitType {
	case "feat":
		if len(files) == 1 {
			base := filepath.Base(files[0])
			name := strings.TrimSuffix(base, filepath.Ext(base))
			if strings.Contains(diff, "new file mode") {
				return "add " + name + " module"
			}
			return "add new functionality to " + name
		}
		return "add new features"

	case "fix":
		if containsPattern(diff, "nil") || containsPattern(diff, "null") {
			if len(files) == 1 {
				base := filepath.Base(files[0])
				name := strings.TrimSuffix(base, filepath.Ext(base))
				return "handle nil pointer in " + name
			}
			return "handle nil pointer dereference"
		}
		if containsPattern(diff, "err") || containsPattern(diff, "error") {
			return "handle error case properly"
		}
		if len(files) == 1 {
			base := filepath.Base(files[0])
			name := strings.TrimSuffix(base, filepath.Ext(base))
			return "resolve issue in " + name
		}
		return "resolve issue"

	case "test":
		if len(files) == 1 {
			base := filepath.Base(files[0])
			name := strings.TrimSuffix(base, filepath.Ext(base))
			name = strings.TrimSuffix(name, "_test")
			return "update test cases for " + name
		}
		return "update test cases"

	case "docs":
		if len(files) == 1 {
			base := filepath.Base(files[0])
			name := strings.TrimSuffix(base, filepath.Ext(base))
			return "update " + name + " documentation"
		}
		return "update documentation"

	case "refactor":
		if len(files) == 1 {
			base := filepath.Base(files[0])
			name := strings.TrimSuffix(base, filepath.Ext(base))
			return "restructure " + name
		}
		return "restructure code"

	case "style":
		return "format code"

	case "chore":
		if len(files) == 1 {
			base := filepath.Base(files[0])
			return "update " + base
		}
		return "update configuration"

	default:
		return "update files"
	}
}

func isStyleOnly(diff string) bool {
	lines := strings.Split(diff, "\n")
	for _, line := range lines {
		if !strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "-") {
			continue
		}
		if strings.HasPrefix(line, "+++") || strings.HasPrefix(line, "---") {
			continue
		}

		var content string
		if strings.HasPrefix(line, "+") {
			content = line[1:]
		} else if strings.HasPrefix(line, "-") {
			content = line[1:]
		}

		trimmed := strings.TrimSpace(content)
		// Non-empty lines that differ only by whitespace are style changes.
		// If the trimmed content is non-trivial and we see both + and -, it could be style.
		if trimmed != "" && trimmed != content {
			continue // whitespace-only difference
		}
	}

	// Check if additions and removals are the same content-wise.
	added := extractContent(diff, "+")
	removed := extractContent(diff, "-")

	if len(added) == 0 && len(removed) == 0 {
		return false
	}

	// Compare normalized content.
	normalizedAdded := commitNormalizeWhitespace(strings.Join(added, "\n"))
	normalizedRemoved := commitNormalizeWhitespace(strings.Join(removed, "\n"))

	return normalizedAdded == normalizedRemoved && normalizedAdded != ""
}

func isFixPattern(diff string) bool {
	patterns := []string{
		"+\tif err != nil",
		"+\tif err != nil {",
		"+ if err != nil",
		"+\treturn err",
		"+\treturn fmt.Errorf",
		"+ return fmt.Errorf",
		"+\t\treturn nil, err",
		"+\tdefer ",
		"+ defer ",
		"+\trecover()",
	}
	for _, p := range patterns {
		if strings.Contains(diff, p) {
			return true
		}
	}
	return false
}

func hasNewExportedSymbols(diff string) bool {
	lines := strings.Split(diff, "\n")
	for _, line := range lines {
		if !strings.HasPrefix(line, "+") || strings.HasPrefix(line, "+++") {
			continue
		}
		content := line[1:]
		trimmed := strings.TrimSpace(content)
		if strings.HasPrefix(trimmed, "func ") {
			// Check if it's an exported function (starts with uppercase after "func ").
			rest := strings.TrimPrefix(trimmed, "func ")
			// Skip method receiver.
			if strings.HasPrefix(rest, "(") {
				closeIdx := strings.Index(rest, ")")
				if closeIdx > 0 && closeIdx+1 < len(rest) {
					rest = strings.TrimSpace(rest[closeIdx+1:])
				}
			}
			if len(rest) > 0 && rest[0] >= 'A' && rest[0] <= 'Z' {
				return true
			}
		}
		if strings.HasPrefix(trimmed, "type ") && strings.Contains(trimmed, "struct") {
			rest := strings.TrimPrefix(trimmed, "type ")
			if len(rest) > 0 && rest[0] >= 'A' && rest[0] <= 'Z' {
				return true
			}
		}
	}
	return false
}

func extractContent(diff string, prefix string) []string {
	var result []string
	lines := strings.Split(diff, "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, prefix) && !strings.HasPrefix(line, prefix+prefix+prefix) {
			result = append(result, line[1:])
		}
	}
	return result
}

func commitNormalizeWhitespace(s string) string {
	var result strings.Builder
	prevSpace := false
	for _, r := range s {
		if r == ' ' || r == '\t' || r == '\n' {
			if !prevSpace {
				result.WriteRune(' ')
				prevSpace = true
			}
		} else {
			result.WriteRune(r)
			prevSpace = false
		}
	}
	return strings.TrimSpace(result.String())
}

func commonDirPrefix(a, b string) string {
	partsA := strings.Split(filepath.ToSlash(a), "/")
	partsB := strings.Split(filepath.ToSlash(b), "/")

	var common []string
	for i := 0; i < len(partsA) && i < len(partsB); i++ {
		if partsA[i] == partsB[i] {
			common = append(common, partsA[i])
		} else {
			break
		}
	}

	return strings.Join(common, "/")
}

func containsPattern(diff, pattern string) bool {
	lines := strings.Split(diff, "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "+") && strings.Contains(line, pattern) {
			return true
		}
	}
	return false
}

func isValidCommitType(t string) bool {
	for _, valid := range validCommitTypes {
		if t == valid {
			return true
		}
	}
	return false
}
