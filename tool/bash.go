package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/GrayCodeAI/hawk/sandbox"
)

// dangerousCommands are commands that should ALWAYS be blocked.
var dangerousCommands = map[string]bool{
	"rm": true, "rmdir": true, "mkfs": true, "dd": true,
	"shred": true, "wipefs": true,
}

// dangerousPatterns catches structural patterns that bypass simple word matching.
var dangerousSubstrings = []string{
	"rm -rf /", "rm -rf ~", "rm -rf .",
	":(){ :|:& };:", // fork bomb
	"chmod -r 777 /",
	"> /dev/sd", "> /dev/nv",
}

// suspiciousPatterns indicate commands that need extra scrutiny (force permission prompt).
var suspiciousPatterns = []string{
	"eval ", "exec ", "$(", "`", // command substitution / eval
	"| sh", "| bash", "| zsh", // pipe to shell
	"|sh", "|bash", "|zsh", // pipe to shell (no space)
	"sudo ", "su -", // privilege escalation
	"curl ", "wget ", // network downloads (when piped)
	"> /", ">> /", // writing to absolute paths
	"git push --force", "git reset --hard",
	"DROP ", "DELETE FROM", "TRUNCATE ", // SQL
}

// zshDangerousCommands are Zsh-specific commands that can bypass security checks.
var zshDangerousCommands = map[string]bool{
	"zmodload": true, "emulate": true,
	"sysopen": true, "sysread": true, "syswrite": true, "sysseek": true,
	"zpty": true, "ztcp": true, "zsocket": true,
	"zf_rm": true, "zf_mv": true, "zf_ln": true, "zf_chmod": true,
	"zf_chown": true, "zf_mkdir": true, "zf_rmdir": true, "zf_chgrp": true,
}

// Pre-compiled regexes for performance.
var (
	zshEqualsExpansionRe    = regexp.MustCompile(`(?:^|[\s;&|])=[a-zA-Z_]`)
	ifsInjectionRe          = regexp.MustCompile(`\$IFS|\$\{[^}]*IFS`)
	procEnvironRe           = regexp.MustCompile(`/proc/.*environ`)
	ansiCQuotingRe          = regexp.MustCompile(`\$'[^']*'`)
	localeQuotingRe         = regexp.MustCompile(`\$"[^"]*"`)
	emptyQuotePairRe        = regexp.MustCompile(`(?:''|"")+\s*-`)
	consecutiveQuotesRe     = regexp.MustCompile(`(?:^|\s)['"]{3,}`)
	heredocSubstitutionRe   = regexp.MustCompile(`\$\(.*<<`)
	commandSubstitutionRe   = regexp.MustCompile(`\$\(`)
	heredocRe               = regexp.MustCompile(`<<`)
	gitCommitRe             = regexp.MustCompile(`^git\s+commit\s+[^;&|$<>()\n\r]*?-m\s+["']([^"']+)["']\s*$`)
	zmodloadRe              = regexp.MustCompile(`\bzmodload\b`)
	processSubstitutionRe   = regexp.MustCompile(`<\(|>\(|=\(`)
	consecutiveQuotesExecRe = regexp.MustCompile(`['"]{3,}`)
)
var commandSubstitutionPatterns = []*regexp.Regexp{
	regexp.MustCompile(`<\(`),              // process substitution <()
	regexp.MustCompile(`>\(`),              // process substitution >()
	regexp.MustCompile(`=\(`),              // zsh process substitution =()
	regexp.MustCompile(`\$\(`),             // $() command substitution
	regexp.MustCompile(`\$\{`),             // ${} parameter substitution
	regexp.MustCompile(`\$\[`),             // $[] legacy arithmetic expansion
	regexp.MustCompile(`~\[`),              // zsh-style parameter expansion
	regexp.MustCompile(`\(\+`),             // zsh glob qualifier with command execution
	regexp.MustCompile(`\}\s*always\s*\{`), // zsh always block
}

type BashTool struct{}

func (BashTool) Name() string        { return "Bash" }
func (BashTool) RiskLevel() string   { return "high" }
func (BashTool) Aliases() []string   { return []string{"bash"} }
func (BashTool) Description() string { return "Run a shell command." }
func (BashTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"command": map[string]interface{}{"type": "string", "description": "The shell command to run"},
			"timeout": map[string]interface{}{"type": "integer", "description": "Timeout in seconds (default 120)"},
			"run_in_background": map[string]interface{}{
				"type":        "boolean",
				"description": "Run command in the background and return a task_id for TaskOutput/TaskStop",
			},
		},
		"required": []string{"command"},
	}
}

// SegmentCommand splits a command string on &&, ||, ;, and | (respecting quotes)
// into individual segments for independent analysis.
func SegmentCommand(cmd string) []string {
	var segments []string
	var current strings.Builder
	inSingle, inDouble := false, false
	runes := []rune(cmd)
	for i := 0; i < len(runes); i++ {
		ch := runes[i]
		if ch == '\'' && !inDouble {
			inSingle = !inSingle
			current.WriteRune(ch)
			continue
		}
		if ch == '"' && !inSingle {
			inDouble = !inDouble
			current.WriteRune(ch)
			continue
		}
		if inSingle || inDouble {
			current.WriteRune(ch)
			continue
		}
		// Check for &&, ||
		if i+1 < len(runes) && ((ch == '&' && runes[i+1] == '&') || (ch == '|' && runes[i+1] == '|')) {
			if s := strings.TrimSpace(current.String()); s != "" {
				segments = append(segments, s)
			}
			current.Reset()
			i++ // skip second char
			continue
		}
		// Check for ; or single |
		if ch == ';' || ch == '|' {
			if s := strings.TrimSpace(current.String()); s != "" {
				segments = append(segments, s)
			}
			current.Reset()
			continue
		}
		current.WriteRune(ch)
	}
	if s := strings.TrimSpace(current.String()); s != "" {
		segments = append(segments, s)
	}
	return segments
}

// IsSuspicious returns true if the command needs a permission prompt.
// This is fail-closed: anything we can't confidently classify as safe gets flagged.
func IsSuspicious(command string) bool {
	// Whole-command checks that apply regardless of segmentation
	if strings.Contains(command, "\r") {
		return true
	}
	if ifsInjectionRe.MatchString(command) {
		return true
	}
	if procEnvironRe.MatchString(command) {
		return true
	}
	if ansiCQuotingRe.MatchString(command) {
		return true
	}
	if localeQuotingRe.MatchString(command) {
		return true
	}
	if emptyQuotePairRe.MatchString(command) {
		return true
	}
	if consecutiveQuotesRe.MatchString(command) {
		return true
	}
	if commandSubstitutionRe.MatchString(command) && heredocRe.MatchString(command) {
		return true
	}

	// Check full command for patterns that span operators (e.g. "| bash")
	lower := strings.ToLower(command)
	for _, pat := range suspiciousPatterns {
		if strings.Contains(lower, strings.ToLower(pat)) {
			return true
		}
	}

	// Check each segment independently
	for _, seg := range SegmentCommand(command) {
		if isSegmentSuspicious(seg) {
			return true
		}
	}
	return false
}

// isSegmentSuspicious checks a single command segment for suspicious patterns.
func isSegmentSuspicious(segment string) bool {
	lower := strings.ToLower(segment)

	for _, pat := range dangerousSubstrings {
		if strings.Contains(lower, pat) {
			return true
		}
	}
	for _, pat := range suspiciousPatterns {
		if strings.Contains(lower, strings.ToLower(pat)) {
			return true
		}
	}
	for _, re := range commandSubstitutionPatterns {
		if re.MatchString(segment) {
			return true
		}
	}
	if zshEqualsExpansionRe.MatchString(segment) {
		return true
	}
	words := strings.Fields(segment)
	for _, word := range words {
		base := strings.TrimLeft(word, "\\/")
		base = strings.TrimSpace(base)
		if zshDangerousCommands[base] {
			return true
		}
	}
	if len(words) > 0 {
		base := words[0]
		if i := strings.LastIndex(base, "/"); i >= 0 {
			base = base[i+1:]
		}
		base = strings.TrimLeft(base, "\\")
		if dangerousCommands[base] {
			return true
		}
	}
	return false
}

// IsSafeGitCommit checks if a git commit command is safe.
// Git commits with simple quoted messages are considered safe.
func IsSafeGitCommit(command string) bool {
	// Only allow git commit with simple quoted message
	// Note: backtick is excluded from the character class for security
	match := gitCommitRe.FindStringSubmatch(command)
	if match == nil {
		return false
	}
	// Check for suspicious content in the message
	msg := match[1]
	return !strings.Contains(msg, "$(") && !strings.Contains(msg, "`") && !strings.Contains(msg, "${")
}

func (BashTool) Execute(ctx context.Context, input json.RawMessage) (string, error) {
	var p struct {
		Command         string `json:"command"`
		Timeout         int    `json:"timeout"`
		RunInBackground bool   `json:"run_in_background"`
	}
	if err := json.Unmarshal(input, &p); err != nil {
		return "", err
	}
	if p.Command == "" {
		return "", fmt.Errorf("command is required")
	}

	// Safety layer: block destructive commands before any execution.
	if IsDestructiveCommand(p.Command) {
		return "", fmt.Errorf("blocked: destructive command pattern detected — %s", p.Command)
	}

	// Hard block: always-dangerous patterns
	lower := strings.ToLower(p.Command)
	for _, pat := range dangerousSubstrings {
		if strings.Contains(lower, pat) {
			return "", fmt.Errorf("blocked: dangerous command pattern detected")
		}
	}

	// Block zsh zmodload which enables dangerous modules
	if zmodloadRe.MatchString(p.Command) {
		return "", fmt.Errorf("blocked: zmodload can enable dangerous zsh modules")
	}

	// Block process substitution
	if processSubstitutionRe.MatchString(p.Command) {
		return "", fmt.Errorf("blocked: process substitution requires approval")
	}

	// Block IFS injection
	if ifsInjectionRe.MatchString(p.Command) {
		return "", fmt.Errorf("blocked: IFS variable usage bypasses security validation")
	}

	// Block carriage return
	if strings.Contains(p.Command, "\r") {
		return "", fmt.Errorf("blocked: carriage return can cause shell-quote/bash tokenization differential")
	}

	// Block /proc/*/environ access
	if procEnvironRe.MatchString(p.Command) {
		return "", fmt.Errorf("blocked: /proc/*/environ access can expose environment variables")
	}

	// Block heredoc in substitution (complex validation)
	if heredocSubstitutionRe.MatchString(p.Command) {
		return "", fmt.Errorf("blocked: heredoc in command substitution requires approval")
	}

	// Block ANSI-C quoting
	if ansiCQuotingRe.MatchString(p.Command) {
		return "", fmt.Errorf("blocked: ANSI-C quoting can hide dangerous characters")
	}

	// Block empty quote pairs before dash
	if emptyQuotePairRe.MatchString(p.Command) {
		return "", fmt.Errorf("blocked: empty quote pair before dash can hide flags")
	}

	// Block consecutive quotes
	if consecutiveQuotesExecRe.MatchString(p.Command) {
		return "", fmt.Errorf("blocked: consecutive quotes indicate obfuscation attempt")
	}

	// Apply per-tool timeout from safety config, allow explicit override.
	timeout := time.Duration(p.Timeout) * time.Second
	if timeout == 0 {
		timeout = ToolTimeout("Bash")
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	if p.RunInBackground {
		id, err := startBackgroundBash(ctx, p.Command)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("Started background task %s. Use TaskOutput with task_id=%q to read output, or TaskStop to stop it.", id, id), nil
	}

	// Sandbox wrapping: if a sandbox mode is configured, wrap the command
	// with sandbox-exec (macOS Seatbelt) when available.
	execName := "bash"
	execArgs := []string{"-c", p.Command}
	if sbMode := sandbox.ModeFromContext(ctx); sbMode != sandbox.ModeOff {
		workDir, _ := os.Getwd()
		cfg := sandbox.SandboxConfig{Mode: sbMode, WorkspaceDir: workDir, AllowNetwork: true}
		if sandbox.Available() {
			execName, execArgs = sandbox.WrapCommand(p.Command, cfg)
		}
	}

	cmd := exec.CommandContext(ctx, execName, execArgs...)
	out, err := cmd.CombinedOutput()
	result := string(out)

	// Apply safety output truncation (50KB).
	result = TruncateOutput(result)
	result = strings.TrimRight(result, "\n")

	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return result + "\n\n(command timed out)", nil
		}
		return fmt.Sprintf("%s\n\nexit code: %s", result, err.Error()), nil
	}
	return result, nil
}

// countRunes returns the number of UTF-8 code points in a string.
func countRunes(s string) int {
	return utf8.RuneCountInString(s)
}

// validateHeredocSafety performs enhanced validation for heredoc patterns.
func validateHeredocSafety(command string) bool {
	// Check for heredoc in command substitution
	if !heredocSubstitutionRe.MatchString(command) {
		return true
	}
	// For security, any heredoc in command substitution requires approval
	return false
}
