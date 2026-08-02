package tool

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"regexp"
	"strings"
	"time"

	homepkg "github.com/GrayCodeAI/hawk/internal/home"
	"github.com/GrayCodeAI/hawk/internal/sandbox"
)

// dangerousCommands are commands that should ALWAYS be blocked.
var dangerousCommands = map[string]bool{
	"rm": true, "rmdir": true, "mkfs": true, "dd": true,
	"shred": true, "wipefs": true,
}

// dangerousPatterns catches structural patterns that bypass simple word matching.
var dangerousSubstrings = []string{
	"rm -rf /", "rm -rf /*",
	"rm -rf ~", "rm -rf .",
	":(){ :|:& };:", // fork bomb
	"chmod -r 777 /",
	"> /dev/sd", "> /dev/nv",
}

// tildeUserRe matches a ~username reference (e.g. ~root/, ~alice ) so it can be
// expanded to that user's home directory. The current-user forms (~ and ~/)
// are handled separately above.
var tildeUserRe = regexp.MustCompile(`~([a-zA-Z_][a-zA-Z0-9_-]*)(/|\s|$)`)

// normalizeCommand normalizes a command to prevent trivial bypass of
// dangerous-command detection. It expands tilde and collapses repeated root globs.
func normalizeCommand(cmd string) string {
	homeDir := homepkg.MustDir()
	// Expand ~/ and ~ to the user's home directory
	if homeDir != "" {
		cmd = strings.ReplaceAll(cmd, "~/", homeDir+"/")
		if strings.HasPrefix(cmd, "~") && (len(cmd) == 1 || cmd[1] == ' ' || cmd[1] == '\t') {
			cmd = homeDir + cmd[1:]
		}
	}
	// Expand ~username/ references to the real home directory so a command
	// targeting another user's home (e.g. "rm -rf ~root") is normalized to its
	// concrete path before dangerous-command detection runs, instead of being
	// left as an unexpanded tilde the checks would miss.
	cmd = tildeUserRe.ReplaceAllStringFunc(cmd, func(m string) string {
		sub := tildeUserRe.FindStringSubmatch(m)
		if sub == nil {
			return m
		}
		if u, err := user.Lookup(sub[1]); err == nil && u.HomeDir != "" {
			return u.HomeDir + sub[2]
		}
		return m
	})
	// Collapse repeated /* sequences: rm -rf /* -> rm -rf /
	for strings.Contains(cmd, "/*") {
		cmd = strings.ReplaceAll(cmd, "/*", "/")
	}
	return cmd
}

// shellFunctionRe matches shell function definitions (bash/zsh): "name() {" or "name () {"
var shellFunctionRe = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*\s*\(\s*\)`)

// varExpansionRe matches variable expansion used as a command: $cmd, ${cmd}, etc.
var varExpansionRe = regexp.MustCompile(`^\$[{(]?[a-zA-Z_][a-zA-Z0-9_]*[)}]?$`)

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
	envDumpRe               = regexp.MustCompile(`(?i)(^|[;&|]\s*|\s)(printenv|env)(\s|$)`)
	hawkEnvReadRe           = regexp.MustCompile(`(?i)\b(cat|type|head|less|more|dd)\b[^\n;|]*\.hawk/(env|\.env)\b`)
	apiKeyEchoRe            = regexp.MustCompile(`(?i)\becho\s+[^\n;|]*\$?(ANTHROPIC|OPENAI|OPENROUTER|GEMINI|GROK|XAI)_API_KEY`)
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

// ContainerExecutor allows BashTool to route commands through a container
// instead of local execution. When set via context, all commands run inside
// the container (Docker-first mode).
type ContainerExecutor interface {
	Exec(ctx context.Context, command string, timeout time.Duration) (string, error)
	Running() bool
}

type containerExecKey struct{}

// WithContainerExecutor injects a container executor into the context.
// When present, BashTool routes all commands through it instead of local shell.
func WithContainerExecutor(ctx context.Context, ce ContainerExecutor) context.Context {
	return context.WithValue(ctx, containerExecKey{}, ce)
}

// ContainerExecutorFromContext extracts the container executor, if any.
func ContainerExecutorFromContext(ctx context.Context) ContainerExecutor {
	if ce, ok := ctx.Value(containerExecKey{}).(ContainerExecutor); ok {
		return ce
	}
	return nil
}

// limitedWriter is an io.Writer that caps the total bytes stored in its
// internal buffer. Once the limit is reached, subsequent writes are silently
// discarded (but still counted). This prevents unbounded memory growth from
// commands that produce huge output (e.g., cat /dev/urandom, yes) — the
// command continues to run and is eventually killed by the context timeout,
// but the process memory stays bounded.
type limitedWriter struct {
	buf      bytes.Buffer
	maxBytes int
}

func (w *limitedWriter) Write(p []byte) (int, error) {
	if w.buf.Len() >= w.maxBytes {
		return len(p), nil // silently discard, keep the command unblocked
	}
	remaining := w.maxBytes - w.buf.Len()
	if len(p) > remaining {
		w.buf.Write(p[:remaining])
		return len(p), nil
	}
	return w.buf.Write(p)
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

// SegmentCommand splits a command string on &&, ||, ;, and | (respecting quotes
// and heredocs) into individual segments for independent analysis.
func SegmentCommand(cmd string) []string {
	var segments []string
	var current strings.Builder
	inSingle, inDouble := false, false
	inHeredoc := false
	heredocDelim := ""
	runes := []rune(cmd)
	for i := 0; i < len(runes); i++ {
		ch := runes[i]

		// If inside a heredoc body, consume until we find the delimiter on its own line
		if inHeredoc {
			current.WriteRune(ch)
			if ch == '\n' {
				lineStart := i + 1
				lineEnd := lineStart
				for lineEnd < len(runes) && runes[lineEnd] != '\n' {
					lineEnd++
				}
				line := strings.TrimSpace(string(runes[lineStart:lineEnd]))
				if line == heredocDelim {
					for j := lineStart; j <= lineEnd && j < len(runes); j++ {
						current.WriteRune(runes[j])
					}
					i = lineEnd
					inHeredoc = false
					heredocDelim = ""
				}
			}
			continue
		}

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

		// Detect heredoc: <<EOF, << EOF, <<-EOF
		if ch == '<' && i+1 < len(runes) && runes[i+1] == '<' {
			j := i + 2
			if j < len(runes) && runes[j] == '-' {
				j++
			}
			for j < len(runes) && runes[j] == ' ' {
				j++
			}
			if j < len(runes) {
				delimStart := j
				delimQuote := rune(0)
				if runes[j] == '\'' || runes[j] == '"' {
					delimQuote = runes[j]
					j++
					delimStart = j
					for j < len(runes) && runes[j] != delimQuote {
						j++
					}
				} else {
					for j < len(runes) && runes[j] != ' ' && runes[j] != '\n' && runes[j] != '<' && runes[j] != '>' && runes[j] != '|' && runes[j] != '&' && runes[j] != ';' {
						j++
					}
				}
				if j > delimStart {
					heredocDelim = string(runes[delimStart:j])
					inHeredoc = true
					for k := i; k < j; k++ {
						current.WriteRune(runes[k])
					}
					i = j - 1
					continue
				}
			}
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
	// Credential files the file tools block (SSH keys, .env, …) must also
	// force scrutiny when referenced through the shell.
	if CommandReferencesSensitivePath(command) != "" {
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
	segment = normalizeCommand(segment)
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
	// Block shell function definitions (bypasses dangerousCommands check)
	if shellFunctionRe.MatchString(segment) {
		return true
	}
	// Block variable expansion used as a command (e.g. $cmd -rf /)
	if len(words) > 0 && varExpansionRe.MatchString(words[0]) {
		return true
	}
	// Block `command` builtin with dangerous commands
	if len(words) > 1 && words[0] == "command" {
		cmdBase := words[1]
		if i := strings.LastIndex(cmdBase, "/"); i >= 0 {
			cmdBase = cmdBase[i+1:]
		}
		if dangerousCommands[cmdBase] {
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

// hardDenySubstrings is the strict subset of suspiciousPatterns that should
// always be hard-blocked even in contexts where the permission-system prompt
// is bypassed (e.g. run_in_background=true, --dangerously-skip-permissions).
// Kept narrow on purpose: it excludes "writing to absolute paths" and
// "curl/wget" which are common in legitimate agent workflows.
var hardDenySubstrings = []string{
	"eval ",
	"exec ",
	"$(",
	"`",
	"| sh",
	"| bash",
	"| zsh",
	"|sh",
	"|bash",
	"|zsh",
	"sudo ",
	"su -",
}

// isHardDeny returns true if the command contains a hard-deny substring.
// Used to gate command-substitution, eval, and pipe-to-shell patterns
// that should never execute without a human approval, even in background mode.
func isHardDeny(command string) bool {
	lower := strings.ToLower(command)
	for _, pat := range hardDenySubstrings {
		if strings.Contains(lower, pat) {
			return true
		}
	}
	// Reading credential files must never run without a human in the loop;
	// in prompt-bypassing contexts that means a hard block.
	return CommandReferencesSensitivePath(command) != ""
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

	// Explore/plan hard gate: segment-aware read-only allowlist (PACK-02).
	if tc := GetToolContext(ctx); tc != nil && tc.ReadOnlyBash {
		if err := ExploreBashAllowed(p.Command); err != nil {
			return "", fmt.Errorf("blocked (explore/plan read-only bash): %w", err)
		}
		// Background bash can still mutate via long-running processes; deny.
		if p.RunInBackground {
			return "", fmt.Errorf("blocked (explore/plan read-only bash): run_in_background is not allowed")
		}
	}

	// Safety layer: block destructive commands before any execution.
	if IsDestructiveCommand(p.Command) {
		return "", fmt.Errorf("blocked: destructive command pattern detected — %s", p.Command)
	}

	// AST safety layer: walk the bash AST looking for nested dangers
	// (substitution bodies containing destructive commands, heredoc
	// bodies with eval/exec, process substitutions). This is the
	// second-pass safety check that catches what the regex layer
	// misses — for example, the regex layer flags `echo $(rm -rf /)`
	// because the outer string contains "rm -rf", but it does NOT flag
	// the safer-looking `echo $(date +%Y)`. The AST layer is the one
	// that actually checks the INNER content. The findings are
	// surfaced as a hard-block error so a future sub-agent turn cannot
	// build on top of a command that contains a nested destructive
	// command.
	astFindings := bashASTAnalyze(p.Command)
	if len(astFindings) > 0 {
		// Format findings as a single error message.
		var parts []string
		for _, f := range astFindings {
			parts = append(parts, f.String())
		}
		return "", fmt.Errorf("blocked: AST safety layer flagged %d finding(s): %s", len(astFindings), strings.Join(parts, "; "))
	}

	// Normalize command to prevent trivial bypass of dangerous-command detection.
	normalized := normalizeCommand(p.Command)

	// Hard block: always-dangerous patterns
	lower := strings.ToLower(normalized)
	for _, pat := range dangerousSubstrings {
		if strings.Contains(lower, pat) {
			return "", fmt.Errorf("blocked: dangerous command pattern detected")
		}
	}

	// Hard-block the most-dangerous suspicious patterns even when no
	// permission prompt is in scope (e.g. run_in_background=true skips the
	// human-in-the-loop approval). This is a strict subset of the
	// suspiciousPatterns list — it deliberately excludes
	// "writing to absolute paths" and "curl/wget" which are common in
	// legitimate agent tasks.
	if isHardDeny(p.Command) {
		return "", fmt.Errorf("blocked: hard-deny pattern (e.g. eval/command-substitution) cannot run in hard-deny contexts like run_in_background — %s", p.Command)
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
	if envDumpRe.MatchString(p.Command) {
		return "", fmt.Errorf("blocked: dumping environment variables can expose API keys")
	}
	if hawkEnvReadRe.MatchString(p.Command) {
		return "", fmt.Errorf("blocked: reading ~/.hawk env files can expose API keys")
	}
	if apiKeyEchoRe.MatchString(p.Command) {
		return "", fmt.Errorf("blocked: echoing API key environment variables is not allowed")
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
		// Apply sandbox wrapping for background bash, same as the foreground
		// path below. Background bash previously bypassed the sandbox entirely
		// (C2) — it returned before the sandbox-wrapping block was reached.
		bgExecName := "bash"
		bgExecArgs := []string{"-c", p.Command}
		if sbMode := sandbox.ModeFromContext(ctx); sbMode != sandbox.ModeOff {
			workDir, _ := os.Getwd()
			cfg := sandbox.SandboxConfig{Mode: sbMode, WorkspaceDir: workDir, AllowNetwork: sandbox.ModeAllowsNetwork(sbMode)}
			switch sbMode {
			case sandbox.ModeStrict:
				cfg.Tier = sandbox.TierStrict
			case sandbox.ModeWorkspace:
				cfg.Tier = sandbox.TierWorkspace
			}
			var wrapErr error
			bgExecName, bgExecArgs, wrapErr = sandbox.WrapCommand(p.Command, cfg)
			if wrapErr != nil {
				return "", fmt.Errorf("sandbox unavailable (mode=%s): %w", sbMode, wrapErr)
			}
		}
		id, err := startBackgroundBash(ctx, p.Command, bgExecName, bgExecArgs)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("Started background task %s. Use TaskOutput with task_id=%q to read output, or TaskStop to stop it.", id, id), nil
	}

	// Container mode: if a ContainerExecutor is in context, route through Docker.
	// Full container isolation — no permission prompts needed.
	if ce := ContainerExecutorFromContext(ctx); ce != nil && ce.Running() {
		result, err := ce.Exec(ctx, p.Command, timeout)
		result = TruncateOutput(result)
		result = strings.TrimRight(result, "\n")
		if err != nil {
			return fmt.Sprintf("%s\n\nexit code: %s", result, err.Error()), nil
		}
		return result, nil
	}

	// Sandbox wrapping: if a sandbox mode is configured, wrap the command
	// with the platform sandbox (macOS Seatbelt, Linux unshare). We always
	// call WrapCommand — it fails closed (returns an error) when no backend
	// is available. The previous sandbox.Available() guard caused fail-open
	// behavior: when no backend was present the command ran unsandboxed on
	// the host, contradicting the documented "fail closed" promise.
	execName := "bash"
	execArgs := []string{"-c", p.Command}
	if sbMode := sandbox.ModeFromContext(ctx); sbMode != sandbox.ModeOff {
		workDir, _ := os.Getwd()
		// Network follows the mode (strict denies; workspace allows) instead
		// of being unconditionally on, so a sandboxed command can no longer
		// exfiltrate data in strict mode. See sandbox.ModeAllowsNetwork.
		cfg := sandbox.SandboxConfig{Mode: sbMode, WorkspaceDir: workDir, AllowNetwork: sandbox.ModeAllowsNetwork(sbMode)}
		// Map the legacy Mode to the corresponding Tier. ModeStrict
		// → TierStrict (deny all), ModeWorkspace → TierWorkspace
		// (allow workspace writes, deny process exec — the new safe
		// default), ModeOff is handled above so we never get here.
		switch sbMode {
		case sandbox.ModeStrict:
			cfg.Tier = sandbox.TierStrict
		case sandbox.ModeWorkspace:
			cfg.Tier = sandbox.TierWorkspace
		}
		var wrapErr error
		execName, execArgs, wrapErr = sandbox.WrapCommand(p.Command, cfg)
		if wrapErr != nil {
			return "", fmt.Errorf("sandbox unavailable (mode=%s): %w", sbMode, wrapErr)
		}
	}

	cmd := exec.CommandContext(ctx, execName, execArgs...) // #nosec G204 -- command parsed from tool-configured command string (lint/test command)
	// Put the child in its own process group so we can kill the whole tree
	// (including grandchildren spawned by the shell) via kill(-pgid).
	setCmdProcessGroup(cmd)
	if tc := GetToolContext(ctx); tc != nil && tc.WorkingDir != "" {
		cmd.Dir = tc.WorkingDir
	}
	// Use a limitedWriter to cap output at maxOutputBytes instead of
	// CombinedOutput, which buffers the entire output in memory. A command
	// like `yes` or `cat /dev/urandom` can produce GBs before the timeout
	// kills it; the limitedWriter keeps memory bounded while the command
	// continues to run (writes are silently discarded after the cap).
	var lw limitedWriter
	// Cap one byte above maxOutputBytes so that TruncateOutput's > branch
	// fires when the cap is reached. At exactly maxOutputBytes (no discard)
	// TruncateOutput returns unchanged, which is correct.
	lw.maxBytes = maxOutputBytes + 1
	cmd.Stdout = &lw
	cmd.Stderr = &lw
	err := cmd.Run()
	result := lw.buf.String()

	// Apply safety output truncation (50KB) — the limitedWriter may have
	// captured up to maxOutputBytes (500KB), so we still truncate for the
	// final result returned to the model.
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
