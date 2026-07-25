package engine

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/GrayCodeAI/hawk/internal/tool"
	"github.com/GrayCodeAI/hawk/internal/ui/icons"
)

// SelfHealer implements a wolverine-inspired self-healing execution loop.
// It runs a script, captures crash output, sends it to an LLM for a fix,
// applies the patch, and re-runs until success or max attempts.
type SelfHealer struct {
	// MaxAttempts is the maximum number of heal attempts before giving up.
	// Default is 5.
	MaxAttempts int

	// Timeout is the maximum time allowed for each script execution.
	// Default is 60s.
	Timeout time.Duration

	// History stores all healing attempts for inspection.
	History []HealAttempt

	// ChatFn is the LLM chat function used to generate fixes.
	ChatFn func(ctx context.Context, prompt string) (string, error)

	mu sync.Mutex
}

// HealAttempt records a single attempt within the healing loop.
type HealAttempt struct {
	Attempt  int
	Script   string
	ExitCode int
	Output   string
	Error    string
	Fix      string
	Duration time.Duration
	Success  bool
}

// HealResult summarizes the outcome of a complete healing session.
type HealResult struct {
	Attempts      []HealAttempt
	FinalSuccess  bool
	TotalDuration time.Duration
	FixesApplied  int
}

// FileFix represents a single structured fix to apply to a file.
type FileFix struct {
	File       string
	Line       int
	Action     string // "replace", "insert", "delete"
	OldContent string
	NewContent string
}

// NewSelfHealer creates a SelfHealer with sensible defaults.
func NewSelfHealer(chatFn func(context.Context, string) (string, error)) *SelfHealer {
	return &SelfHealer{
		MaxAttempts: 5,
		Timeout:     60 * time.Second,
		History:     make([]HealAttempt, 0),
		ChatFn:      chatFn,
	}
}

// Heal runs a script and iteratively fixes it until it succeeds or max attempts are exhausted.
func (sh *SelfHealer) Heal(ctx context.Context, scriptPath string) (*HealResult, error) {
	start := time.Now()
	result := &HealResult{
		Attempts: make([]HealAttempt, 0, sh.MaxAttempts),
	}

	for i := 1; i <= sh.MaxAttempts; i++ {
		select {
		case <-ctx.Done():
			result.TotalDuration = time.Since(start)
			return result, ctx.Err()
		default:
		}

		attemptStart := time.Now()
		stdout, stderr, exitCode, runErr := sh.RunScript(ctx, scriptPath)

		attempt := HealAttempt{
			Attempt:  i,
			Script:   scriptPath,
			ExitCode: exitCode,
			Output:   stdout,
			Error:    stderr,
			Duration: time.Since(attemptStart),
		}

		if exitCode == 0 && runErr == nil {
			attempt.Success = true
			result.Attempts = append(result.Attempts, attempt)
			sh.recordAttempt(attempt)
			result.FinalSuccess = true
			result.TotalDuration = time.Since(start)
			return result, nil
		}

		// Build prompt and ask LLM for fix
		scriptContent, readErr := os.ReadFile(scriptPath) // #nosec G304 -- path provided by caller via tool/task parameters, inherent to this dev CLI's file operations
		if readErr != nil {
			result.Attempts = append(result.Attempts, attempt)
			result.TotalDuration = time.Since(start)
			return result, fmt.Errorf("cannot read script %s: %w", scriptPath, readErr)
		}

		prompt := sh.BuildFixPrompt(string(scriptContent), stdout+"\n"+stderr, stderr, i)
		fixResponse, chatErr := sh.ChatFn(ctx, prompt)
		if chatErr != nil {
			result.Attempts = append(result.Attempts, attempt)
			result.TotalDuration = time.Since(start)
			return result, fmt.Errorf("LLM chat failed on attempt %d: %w", i, chatErr)
		}

		attempt.Fix = fixResponse

		// Parse and apply fixes
		fixes, parseErr := sh.ParseFix(fixResponse)
		if parseErr != nil {
			// If we can't parse the fix, record the attempt and continue
			result.Attempts = append(result.Attempts, attempt)
			sh.recordAttempt(attempt)
			continue
		}

		if applyErr := sh.ApplyFixes(fixes); applyErr != nil {
			result.Attempts = append(result.Attempts, attempt)
			sh.recordAttempt(attempt)
			continue
		}

		result.FixesApplied++
		result.Attempts = append(result.Attempts, attempt)
		sh.recordAttempt(attempt)
	}

	result.TotalDuration = time.Since(start)
	return result, nil
}

// HealCommand runs an arbitrary shell command and iteratively fixes related files
// until the command succeeds or max attempts are exhausted.
func (sh *SelfHealer) HealCommand(ctx context.Context, command string) (*HealResult, error) {
	start := time.Now()
	result := &HealResult{
		Attempts: make([]HealAttempt, 0, sh.MaxAttempts),
	}

	for i := 1; i <= sh.MaxAttempts; i++ {
		select {
		case <-ctx.Done():
			result.TotalDuration = time.Since(start)
			return result, ctx.Err()
		default:
		}

		attemptStart := time.Now()
		stdout, stderr, exitCode, _ := sh.runCommand(ctx, command)

		attempt := HealAttempt{
			Attempt:  i,
			Script:   command,
			ExitCode: exitCode,
			Output:   stdout,
			Error:    stderr,
			Duration: time.Since(attemptStart),
		}

		if exitCode == 0 {
			attempt.Success = true
			result.Attempts = append(result.Attempts, attempt)
			sh.recordAttempt(attempt)
			result.FinalSuccess = true
			result.TotalDuration = time.Since(start)
			return result, nil
		}

		// Build prompt and ask LLM for fix
		prompt := sh.BuildFixPrompt(command, stdout+"\n"+stderr, stderr, i)
		fixResponse, chatErr := sh.ChatFn(ctx, prompt)
		if chatErr != nil {
			result.Attempts = append(result.Attempts, attempt)
			result.TotalDuration = time.Since(start)
			return result, fmt.Errorf("LLM chat failed on attempt %d: %w", i, chatErr)
		}

		attempt.Fix = fixResponse

		fixes, parseErr := sh.ParseFix(fixResponse)
		if parseErr != nil {
			result.Attempts = append(result.Attempts, attempt)
			sh.recordAttempt(attempt)
			continue
		}

		if applyErr := sh.ApplyFixes(fixes); applyErr != nil {
			result.Attempts = append(result.Attempts, attempt)
			sh.recordAttempt(attempt)
			continue
		}

		result.FixesApplied++
		result.Attempts = append(result.Attempts, attempt)
		sh.recordAttempt(attempt)
	}

	result.TotalDuration = time.Since(start)
	return result, nil
}

// BuildFixPrompt constructs the prompt sent to the LLM, including script content,
// error output, and previous attempt context.
func (sh *SelfHealer) BuildFixPrompt(script, output, errorMsg string, attempt int) string {
	var b strings.Builder

	b.WriteString("You are a code-fixing assistant. A script has crashed and needs repair.\n\n")

	b.WriteString("## Script Content\n```\n")
	b.WriteString(script)
	b.WriteString("\n```\n\n")

	b.WriteString("## Error Output\n```\n")
	b.WriteString(output)
	b.WriteString("\n```\n\n")

	if errorMsg != "" && errorMsg != output {
		b.WriteString("## Stderr\n```\n")
		b.WriteString(errorMsg)
		b.WriteString("\n```\n\n")
	}

	b.WriteString(fmt.Sprintf("## Attempt %d of %d\n\n", attempt, sh.MaxAttempts))

	// Include previous attempts for context
	sh.mu.Lock()
	if len(sh.History) > 0 {
		b.WriteString("## Previous Attempts\n")
		for _, h := range sh.History {
			if h.Script == script || strings.Contains(script, h.Script) {
				b.WriteString(fmt.Sprintf("- Attempt %d: exit code %d, error: %s\n", h.Attempt, h.ExitCode, healTruncate(h.Error, 200)))
				if h.Fix != "" {
					b.WriteString(fmt.Sprintf("  Applied fix: %s\n", healTruncate(h.Fix, 100)))
				}
			}
		}
		b.WriteString("\n")
	}
	sh.mu.Unlock()

	b.WriteString(`Respond with structured fixes in the following format (one or more blocks):

@@FIX
FILE: <filepath>
LINE: <line number>
ACTION: <replace|insert|delete>
OLD_CONTENT:
<original line(s) to match>
END_OLD
NEW_CONTENT:
<replacement line(s)>
END_NEW
@@END

Rules:
- FILE must be an actual file path
- LINE must be a valid line number
- ACTION must be one of: replace, insert, delete
- For "delete" action, OLD_CONTENT is required, NEW_CONTENT can be empty
- For "insert" action, OLD_CONTENT can be empty, LINE indicates where to insert
- Provide only the minimal fix needed
`)

	return b.String()
}

// ParseFix parses the structured LLM response into a slice of FileFix values.
func (sh *SelfHealer) ParseFix(response string) ([]FileFix, error) {
	var fixes []FileFix
	blocks := strings.Split(response, "@@FIX")

	for _, block := range blocks[1:] { // skip text before first @@FIX
		endIdx := strings.Index(block, "@@END")
		if endIdx == -1 {
			endIdx = len(block)
		}
		block = block[:endIdx]

		fix := FileFix{}
		scanner := bufio.NewScanner(strings.NewReader(block))

		for scanner.Scan() {
			line := scanner.Text()
			trimmed := strings.TrimSpace(line)

			if strings.HasPrefix(trimmed, "FILE:") {
				fix.File = strings.TrimSpace(strings.TrimPrefix(trimmed, "FILE:"))
			} else if strings.HasPrefix(trimmed, "LINE:") {
				lineStr := strings.TrimSpace(strings.TrimPrefix(trimmed, "LINE:"))
				n, err := strconv.Atoi(lineStr)
				if err == nil {
					fix.Line = n
				}
			} else if strings.HasPrefix(trimmed, "ACTION:") {
				fix.Action = strings.TrimSpace(strings.TrimPrefix(trimmed, "ACTION:"))
			} else if trimmed == "OLD_CONTENT:" {
				fix.OldContent = readUntilMarker(scanner, "END_OLD")
			} else if trimmed == "NEW_CONTENT:" {
				fix.NewContent = readUntilMarker(scanner, "END_NEW")
			}
		}

		if fix.File == "" || fix.Action == "" {
			continue
		}
		if fix.Action != "replace" && fix.Action != "insert" && fix.Action != "delete" {
			continue
		}
		fixes = append(fixes, fix)
	}

	if len(fixes) == 0 {
		return nil, fmt.Errorf("no valid fixes parsed from response")
	}
	return fixes, nil
}

// ApplyFixes applies a slice of file fixes to disk.
func (sh *SelfHealer) ApplyFixes(fixes []FileFix) error {
	for _, fix := range fixes {
		if err := sh.applyFix(fix); err != nil {
			return fmt.Errorf("applying fix to %s line %d: %w", fix.File, fix.Line, err)
		}
	}
	return nil
}

func (sh *SelfHealer) applyFix(fix FileFix) error {
	data, err := os.ReadFile(fix.File)
	if err != nil {
		return err
	}

	lines := strings.Split(string(data), "\n")

	switch fix.Action {
	case "replace":
		if fix.OldContent != "" {
			// Find and replace the old content
			content := string(data)
			if strings.Contains(content, fix.OldContent) {
				content = strings.Replace(content, fix.OldContent, fix.NewContent, 1)
				return os.WriteFile(fix.File, []byte(content), 0o600)
			}
		}
		// Fallback: replace by line number
		if fix.Line > 0 && fix.Line <= len(lines) {
			oldLines := strings.Split(fix.OldContent, "\n")
			newLines := strings.Split(fix.NewContent, "\n")
			if len(oldLines) == 0 {
				oldLines = []string{lines[fix.Line-1]}
			}
			startIdx := fix.Line - 1
			endIdx := startIdx + len(oldLines)
			if endIdx > len(lines) {
				endIdx = len(lines)
			}
			result := make([]string, 0, len(lines)-len(oldLines)+len(newLines))
			result = append(result, lines[:startIdx]...)
			result = append(result, newLines...)
			result = append(result, lines[endIdx:]...)
			return os.WriteFile(fix.File, []byte(strings.Join(result, "\n")), 0o600)
		}

	case "insert":
		if fix.Line < 1 {
			fix.Line = 1
		}
		if fix.Line > len(lines)+1 {
			fix.Line = len(lines) + 1
		}
		newLines := strings.Split(fix.NewContent, "\n")
		result := make([]string, 0, len(lines)+len(newLines))
		insertIdx := fix.Line - 1
		result = append(result, lines[:insertIdx]...)
		result = append(result, newLines...)
		result = append(result, lines[insertIdx:]...)
		return os.WriteFile(fix.File, []byte(strings.Join(result, "\n")), 0o600)

	case "delete":
		if fix.OldContent != "" {
			content := string(data)
			if strings.Contains(content, fix.OldContent) {
				content = strings.Replace(content, fix.OldContent, "", 1)
				return os.WriteFile(fix.File, []byte(content), 0o600)
			}
		}
		// Fallback: delete by line number
		if fix.Line > 0 && fix.Line <= len(lines) {
			result := make([]string, 0, len(lines)-1)
			result = append(result, lines[:fix.Line-1]...)
			result = append(result, lines[fix.Line:]...)
			return os.WriteFile(fix.File, []byte(strings.Join(result, "\n")), 0o600)
		}
	}

	return nil
}

// RunScript executes a script at the given path and returns stdout, stderr, exit code, and any error.
func (sh *SelfHealer) RunScript(ctx context.Context, path string) (stdout, stderr string, exitCode int, err error) {
	ctx, cancel := context.WithTimeout(ctx, sh.Timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "sh", "-c", path)
	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf

	runErr := cmd.Run()
	stdout = outBuf.String()
	stderr = errBuf.String()

	if runErr != nil {
		if exitErr, ok := runErr.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			exitCode = -1
			err = runErr
		}
	}
	return
}

// runCommand executes an arbitrary shell command.
func (sh *SelfHealer) runCommand(ctx context.Context, command string) (stdout, stderr string, exitCode int, err error) {
	// Route model-generated shell through the same safety stack the Bash
	// tool enforces. Without this, a jailbroken model could read
	// ~/.hawk/provider.json, fork-bomb, or rm -rf from a "fix" attempt.
	if reason := tool.CommandReferencesSensitivePath(command); reason != "" {
		return "", "", -1, fmt.Errorf("self-heal: command references sensitive path (%s): %s", reason, command)
	}
	if tool.IsDestructiveCommand(command) {
		return "", "", -1, fmt.Errorf("self-heal: command flagged as destructive: %s", command)
	}

	ctx, cancel := context.WithTimeout(ctx, sh.Timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "sh", "-c", command)
	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf

	runErr := cmd.Run()
	stdout = outBuf.String()
	stderr = errBuf.String()

	if runErr != nil {
		if exitErr, ok := runErr.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			exitCode = -1
			err = runErr
		}
	}
	return
}

// FormatResult produces a human-readable summary of a healing session.
func FormatHealResult(result *HealResult) string {
	if result == nil || len(result.Attempts) == 0 {
		return "Self-Healing: no attempts made"
	}

	var b strings.Builder
	scriptName := result.Attempts[0].Script
	b.WriteString(fmt.Sprintf("Self-Healing: %s\n", scriptName))
	b.WriteString(strings.Repeat("─", 25))
	b.WriteString("\n")

	for _, a := range result.Attempts {
		if a.Success {
			b.WriteString(fmt.Sprintf("Attempt %d: SUCCESS "+icons.CheckBold()+"\n", a.Attempt))
		} else {
			errSummary := extractErrorSummary(a.Error)
			b.WriteString(fmt.Sprintf("Attempt %d: FAILED (%s)\n", a.Attempt, errSummary))
			if a.Fix != "" {
				fixSummary := extractFixSummary(a.Fix)
				b.WriteString(fmt.Sprintf("  Fix: %s\n", fixSummary))
			}
		}
	}

	b.WriteString(fmt.Sprintf("\nTotal: %d attempts, %d fixes applied, %.1fs\n",
		len(result.Attempts), result.FixesApplied, result.TotalDuration.Seconds()))

	return b.String()
}

// recordAttempt safely appends an attempt to history.
func (sh *SelfHealer) recordAttempt(attempt HealAttempt) {
	sh.mu.Lock()
	defer sh.mu.Unlock()
	sh.History = append(sh.History, attempt)
}

// readUntilMarker reads scanner lines until a marker line is found.
func readUntilMarker(scanner *bufio.Scanner, marker string) string {
	var lines []string
	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == marker {
			break
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

// healTruncate shortens a string to the given max length.
func healTruncate(s string, maxLen int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) <= maxLen {
		return s
	}
	// Rune-safe truncation: never split a multibyte UTF-8 sequence.
	if runes := []rune(s); len(runes) > maxLen {
		return string(runes[:maxLen-3]) + "..."
	}
	return s
}

// extractErrorSummary pulls the first meaningful error line from stderr.
func extractErrorSummary(stderr string) string {
	if stderr == "" {
		return "unknown error"
	}
	lines := strings.Split(strings.TrimSpace(stderr), "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" && !strings.HasPrefix(trimmed, "Traceback") {
			if len(trimmed) > 60 {
				// Rune-safe truncation: never split a multibyte UTF-8 sequence.
				if runes := []rune(trimmed); len(runes) > 60 {
					return string(runes[:57]) + "..."
				}
			}
			return trimmed
		}
	}
	if len(lines) > 0 {
		last := strings.TrimSpace(lines[len(lines)-1])
		if len(last) > 60 {
			// Rune-safe truncation: never split a multibyte UTF-8 sequence.
			if runes := []rune(last); len(runes) > 60 {
				return string(runes[:57]) + "..."
			}
		}
		return last
	}
	return "unknown error"
}

// extractFixSummary produces a brief description of the fix applied.
func extractFixSummary(fix string) string {
	// Look for action lines in the fix response
	lines := strings.Split(fix, "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "ACTION:") {
			action := strings.TrimSpace(strings.TrimPrefix(trimmed, "ACTION:"))
			// Find the file
			for _, l2 := range lines {
				t2 := strings.TrimSpace(l2)
				if strings.HasPrefix(t2, "FILE:") {
					file := strings.TrimSpace(strings.TrimPrefix(t2, "FILE:"))
					return fmt.Sprintf("%s in %s", action, file)
				}
			}
			return action
		}
	}
	// Fallback: first non-empty line
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" && !strings.HasPrefix(trimmed, "@@") {
			if len(trimmed) > 50 {
				// Rune-safe truncation: never split a multibyte UTF-8 sequence.
				if runes := []rune(trimmed); len(runes) > 50 {
					return string(runes[:47]) + "..."
				}
			}
			return trimmed
		}
	}
	return "applied fix"
}
