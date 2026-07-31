package permissions

import (
	"path/filepath"
	"regexp"
	"strings"
)

// Canonicalizer normalizes shell commands for stable approval caching.
// It is stateless and safe for concurrent use.
type Canonicalizer struct{}

// BannedPrefixes lists patterns that should NEVER be saved as approved commands.
var BannedPrefixes = []string{
	// Shell interpreters
	"bash",
	"sh",
	"zsh",
	"fish",
	"dash",
	// Script runners
	"python",
	"node",
	"ruby",
	"perl",
	"lua",
	// Eval-like
	"eval",
	"exec",
	"source",
}

// cosmetic flags that don't affect safety
var cosmeticFlags = map[string]bool{
	"--color":    true,
	"--no-color": true,
	"-v":         true,
	"--verbose":  true,
	"--quiet":    true,
	"-q":         true,
}

// regex to match env variable prefixes like ENV_VAR=value
var envPrefixRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*=\S*$`)

// regex to match redirections
var redirectionRe = regexp.MustCompile(`\s*(?:[0-9]*>[>&]*\s*\S+|[0-9]*<\s*\S+|>&\s*\S+)`)

// regex to collapse multiple spaces
var multiSpaceRe = regexp.MustCompile(`\s+`)

// NewCanonicalizer creates a new Canonicalizer instance.
func NewCanonicalizer() *Canonicalizer {
	return &Canonicalizer{}
}

// Canonicalize normalizes a shell command for consistent matching.
func (c *Canonicalizer) Canonicalize(command string) string {
	// Strip leading/trailing whitespace
	cmd := strings.TrimSpace(command)
	if cmd == "" {
		return ""
	}

	// Remove shell wrappers: bash -c "...", sh -c '...'
	cmd = unwrapShell(cmd)

	// Handle && chains: canonicalize each part individually
	if strings.Contains(cmd, "&&") {
		parts := strings.Split(cmd, "&&")
		var canonParts []string
		for _, part := range parts {
			p := strings.TrimSpace(part)
			if p != "" {
				canonParts = append(canonParts, c.canonicalizeSingle(p))
			}
		}
		return strings.Join(canonParts, " && ")
	}

	// Handle pipes: canonicalize each part
	if strings.Contains(cmd, "|") && !strings.Contains(cmd, "||") {
		parts := splitPipes(cmd)
		var canonParts []string
		for _, part := range parts {
			p := strings.TrimSpace(part)
			if p != "" {
				canonParts = append(canonParts, c.canonicalizeSingle(p))
			}
		}
		return strings.Join(canonParts, " | ")
	}

	return c.canonicalizeSingle(cmd)
}

// canonicalizeSingle normalizes a single command (no chains or pipes).
func (c *Canonicalizer) canonicalizeSingle(cmd string) string {
	// Strip redirections
	cmd = stripRedirections(cmd)

	// Collapse multiple spaces
	cmd = multiSpaceRe.ReplaceAllString(cmd, " ")
	cmd = strings.TrimSpace(cmd)

	if cmd == "" {
		return ""
	}

	// Tokenize
	tokens := tokenize(cmd)
	if len(tokens) == 0 {
		return ""
	}

	// Remove env prefixes
	startIdx := 0
	for startIdx < len(tokens) && envPrefixRe.MatchString(tokens[startIdx]) {
		startIdx++
	}
	if startIdx >= len(tokens) {
		return ""
	}
	tokens = tokens[startIdx:]

	// Normalize path prefix on the command binary
	tokens[0] = normalizePath(tokens[0])

	// Handle `env` as a transparent prefix (e.g., /usr/bin/env python3 → python3)
	if tokens[0] == "env" && len(tokens) > 1 {
		tokens = tokens[1:]
		// Skip any env-style VAR=val after env
		for len(tokens) > 0 && envPrefixRe.MatchString(tokens[0]) {
			tokens = tokens[1:]
		}
		if len(tokens) == 0 {
			return ""
		}
		tokens[0] = normalizePath(tokens[0])
	}

	// Strip cosmetic flags
	var filtered []string
	for _, tok := range tokens {
		if !cosmeticFlags[tok] {
			filtered = append(filtered, tok)
		}
	}

	// Normalize quotes on arguments (for matching only)
	for i := range filtered {
		filtered[i] = stripQuotes(filtered[i])
	}

	return strings.Join(filtered, " ")
}

// ExtractBaseCommand returns just the binary name from a command.
func (c *Canonicalizer) ExtractBaseCommand(command string) string {
	cmd := strings.TrimSpace(command)
	if cmd == "" {
		return ""
	}

	// Remove shell wrappers
	cmd = unwrapShell(cmd)

	// Tokenize
	tokens := tokenize(cmd)
	if len(tokens) == 0 {
		return ""
	}

	// Skip env prefixes
	startIdx := 0
	for startIdx < len(tokens) && envPrefixRe.MatchString(tokens[startIdx]) {
		startIdx++
	}
	if startIdx >= len(tokens) {
		return ""
	}

	base := normalizePath(tokens[startIdx])

	// Handle `env` as a transparent prefix
	if base == "env" && startIdx+1 < len(tokens) {
		startIdx++
		// Skip any VAR=val after env
		for startIdx < len(tokens) && envPrefixRe.MatchString(tokens[startIdx]) {
			startIdx++
		}
		if startIdx >= len(tokens) {
			return "env"
		}
		return normalizePath(tokens[startIdx])
	}

	return base
}

// ExtractSubcommand returns the binary name plus its first non-flag argument.
func (c *Canonicalizer) ExtractSubcommand(command string) string {
	cmd := strings.TrimSpace(command)
	if cmd == "" {
		return ""
	}

	// Remove shell wrappers
	cmd = unwrapShell(cmd)

	// Tokenize
	tokens := tokenize(cmd)
	if len(tokens) == 0 {
		return ""
	}

	// Skip env prefixes
	startIdx := 0
	for startIdx < len(tokens) && envPrefixRe.MatchString(tokens[startIdx]) {
		startIdx++
	}
	if startIdx >= len(tokens) {
		return ""
	}

	baseName := normalizePath(tokens[startIdx])

	// Find the first non-flag argument after the command. For git, skip global
	// options that take a path/value argument so `git -C /repo status` yields
	// "git status" rather than "git /repo".
	for i := startIdx + 1; i < len(tokens); i++ {
		tok := stripQuotes(tokens[i])
		if baseName == "git" {
			switch {
			case tok == "-C" || tok == "-c" || tok == "--git-dir" || tok == "--work-tree":
				i++ // consume the following argument
				continue
			case strings.HasPrefix(tok, "--git-dir=") || strings.HasPrefix(tok, "--work-tree="):
				continue
			}
		}
		if !strings.HasPrefix(tok, "-") && !envPrefixRe.MatchString(tok) {
			return baseName + " " + tok
		}
	}

	return baseName
}

// IsEquivalent checks if two commands are semantically the same for permission purposes.
func (c *Canonicalizer) IsEquivalent(cmd1, cmd2 string) bool {
	canon1 := c.Canonicalize(cmd1)
	canon2 := c.Canonicalize(cmd2)

	if canon1 == canon2 {
		return true
	}

	// Check if base command + subcommand match and targets are the same
	base1 := c.ExtractBaseCommand(cmd1)
	base2 := c.ExtractBaseCommand(cmd2)
	if base1 != base2 {
		return false
	}

	sub1 := c.ExtractSubcommand(cmd1)
	sub2 := c.ExtractSubcommand(cmd2)
	if sub1 != sub2 {
		return false
	}

	// Extract targets (non-flag arguments after subcommand)
	targets1 := extractTargets(cmd1)
	targets2 := extractTargets(cmd2)

	if len(targets1) != len(targets2) {
		return false
	}
	for i := range targets1 {
		if targets1[i] != targets2[i] {
			return false
		}
	}

	return true
}

// GeneratePattern creates a glob pattern that would match this command and similar ones.
func (c *Canonicalizer) GeneratePattern(command string) string {
	cmd := strings.TrimSpace(command)
	if cmd == "" {
		return "*"
	}

	// Work with a lightly processed version that keeps flags intact
	processed := unwrapShell(cmd)
	processed = multiSpaceRe.ReplaceAllString(processed, " ")
	processed = strings.TrimSpace(processed)

	tokens := tokenize(processed)
	if len(tokens) == 0 {
		return "*"
	}

	// Skip env prefixes
	startIdx := 0
	for startIdx < len(tokens) && envPrefixRe.MatchString(tokens[startIdx]) {
		startIdx++
	}
	if startIdx >= len(tokens) {
		return "*"
	}
	tokens = tokens[startIdx:]
	tokens[0] = normalizePath(tokens[0])

	// Handle `env` transparent prefix
	if tokens[0] == "env" && len(tokens) > 1 {
		tokens = tokens[1:]
		for len(tokens) > 0 && envPrefixRe.MatchString(tokens[0]) {
			tokens = tokens[1:]
		}
		if len(tokens) == 0 {
			return "*"
		}
		tokens[0] = normalizePath(tokens[0])
	}

	// Collect the base command and all flags up to the first non-flag argument
	var prefix []string
	prefix = append(prefix, tokens[0])
	argIdx := 1

	// Include flags that appear before the first positional argument
	for argIdx < len(tokens) {
		tok := tokens[argIdx]
		if strings.HasPrefix(tok, "-") {
			// Skip cosmetic flags from the pattern
			if !cosmeticFlags[tok] {
				prefix = append(prefix, tok)
			}
			argIdx++
		} else {
			break
		}
	}

	// Look for a path-like argument to make a directory wildcard
	for i := argIdx; i < len(tokens); i++ {
		tok := stripQuotes(tokens[i])
		if strings.Contains(tok, "/") {
			dir := filepath.Dir(tok)
			if dir != "." && dir != "" {
				return strings.Join(prefix, " ") + " " + dir + "/*"
			}
		}
	}

	// If there's a subcommand (first positional arg), use that + wildcard
	if argIdx < len(tokens) {
		subCmd := stripQuotes(tokens[argIdx])
		if !strings.HasPrefix(subCmd, "-") {
			return strings.Join(prefix, " ") + " " + subCmd + "*"
		}
	}

	return strings.Join(prefix, " ") + "*"
}

// IsBannedPrefix checks if a command starts with a banned prefix.
func (c *Canonicalizer) IsBannedPrefix(command string) bool {
	cmd := strings.TrimSpace(command)
	if cmd == "" {
		return false
	}

	// First, check the raw first token (before unwrapping shell wrappers).
	// This catches "sh -c ..." and "bash script.sh" etc.
	rawBase := extractRawBaseCommand(cmd)
	if isBannedBase(rawBase) {
		return true
	}

	// Also check after unwrapping (for commands like ENV=val python3 script.py)
	base := c.ExtractBaseCommand(cmd)
	if isBannedBase(base) {
		return true
	}

	// Check for piped network fetchers: curl | sh, wget | bash
	if strings.Contains(cmd, "|") {
		parts := splitPipes(cmd)
		if len(parts) >= 2 {
			firstBase := extractRawBaseCommand(strings.TrimSpace(parts[0]))
			lastBase := extractRawBaseCommand(strings.TrimSpace(parts[len(parts)-1]))

			networkFetchers := map[string]bool{"curl": true, "wget": true}
			shellInterps := map[string]bool{"bash": true, "sh": true, "zsh": true, "fish": true, "dash": true}

			if networkFetchers[firstBase] && shellInterps[lastBase] {
				return true
			}
		}
	}

	return false
}

// isBannedBase checks if a base command name matches any banned prefix.
func isBannedBase(base string) bool {
	for _, banned := range BannedPrefixes {
		if base == banned {
			return true
		}
		// Also match versioned variants (python3, node18, etc.)
		if strings.HasPrefix(base, banned) {
			suffix := base[len(banned):]
			if suffix == "" || isVersionSuffix(suffix) {
				return true
			}
		}
	}
	return false
}

// extractRawBaseCommand gets the first token of the command (normalized path only),
// without unwrapping shell wrappers or removing env prefixes.
func extractRawBaseCommand(cmd string) string {
	tokens := tokenize(cmd)
	if len(tokens) == 0 {
		return ""
	}
	return normalizePath(tokens[0])
}

// --- Helper functions ---

// unwrapShell removes shell wrappers like `bash -c "..."` and `sh -c '...'`.
func unwrapShell(cmd string) string {
	shells := []string{"bash", "sh", "zsh", "fish", "dash"}
	for _, shell := range shells {
		prefix := shell + " -c "
		if strings.HasPrefix(cmd, prefix) {
			inner := cmd[len(prefix):]
			inner = strings.TrimSpace(inner)
			inner = stripQuotes(inner)
			return strings.TrimSpace(inner)
		}
	}
	return cmd
}

// stripQuotes removes surrounding quotes from a string.
func stripQuotes(s string) string {
	if len(s) >= 2 {
		if (s[0] == '"' && s[len(s)-1] == '"') || (s[0] == '\'' && s[len(s)-1] == '\'') {
			return s[1 : len(s)-1]
		}
	}
	return s
}

// normalizePath converts absolute paths to just the binary name.
func normalizePath(cmd string) string {
	if strings.Contains(cmd, "/") {
		return filepath.Base(cmd)
	}
	return cmd
}

// stripRedirections removes output/input redirections from a command string.
func stripRedirections(cmd string) string {
	result := redirectionRe.ReplaceAllString(cmd, "")
	return strings.TrimSpace(result)
}

// tokenize splits a command string into tokens, respecting quotes.
func tokenize(cmd string) []string {
	var tokens []string
	var current strings.Builder
	inSingleQuote := false
	inDoubleQuote := false

	for i := 0; i < len(cmd); i++ {
		ch := cmd[i]

		switch {
		case ch == '\'' && !inDoubleQuote:
			inSingleQuote = !inSingleQuote
			current.WriteByte(ch)
		case ch == '"' && !inSingleQuote:
			inDoubleQuote = !inDoubleQuote
			current.WriteByte(ch)
		case ch == ' ' && !inSingleQuote && !inDoubleQuote:
			if current.Len() > 0 {
				tokens = append(tokens, current.String())
				current.Reset()
			}
		default:
			current.WriteByte(ch)
		}
	}
	if current.Len() > 0 {
		tokens = append(tokens, current.String())
	}

	return tokens
}

// splitPipes splits a command by pipe characters, respecting quotes.
func splitPipes(cmd string) []string {
	var parts []string
	var current strings.Builder
	inSingleQuote := false
	inDoubleQuote := false

	for i := 0; i < len(cmd); i++ {
		ch := cmd[i]

		switch {
		case ch == '\'' && !inDoubleQuote:
			inSingleQuote = !inSingleQuote
			current.WriteByte(ch)
		case ch == '"' && !inSingleQuote:
			inDoubleQuote = !inDoubleQuote
			current.WriteByte(ch)
		case ch == '|' && !inSingleQuote && !inDoubleQuote:
			// Make sure it's not || (logical OR)
			if i+1 < len(cmd) && cmd[i+1] == '|' {
				current.WriteByte(ch)
				current.WriteByte(cmd[i+1])
				i++
			} else {
				parts = append(parts, current.String())
				current.Reset()
			}
		default:
			current.WriteByte(ch)
		}
	}
	if current.Len() > 0 {
		parts = append(parts, current.String())
	}

	return parts
}

// extractTargets returns non-flag arguments from a command, after the subcommand.
func extractTargets(command string) []string {
	cmd := strings.TrimSpace(command)
	cmd = unwrapShell(cmd)

	tokens := tokenize(cmd)
	if len(tokens) == 0 {
		return nil
	}

	// Skip env prefixes
	startIdx := 0
	for startIdx < len(tokens) && envPrefixRe.MatchString(tokens[startIdx]) {
		startIdx++
	}
	if startIdx >= len(tokens) {
		return nil
	}

	// Skip the base command
	startIdx++

	// Skip the first non-flag token (subcommand)
	for i := startIdx; i < len(tokens); i++ {
		tok := stripQuotes(tokens[i])
		if !strings.HasPrefix(tok, "-") {
			startIdx = i + 1
			break
		}
	}

	// Collect remaining non-flag tokens as targets
	var targets []string
	for i := startIdx; i < len(tokens); i++ {
		tok := stripQuotes(tokens[i])
		if !strings.HasPrefix(tok, "-") && !redirectionRe.MatchString(tok) {
			targets = append(targets, tok)
		}
	}

	return targets
}

// isVersionSuffix checks if a string looks like a version suffix (e.g., "3", "3.9", "18").
func isVersionSuffix(s string) bool {
	if len(s) == 0 {
		return false
	}
	for _, ch := range s {
		if ch != '.' && (ch < '0' || ch > '9') {
			return false
		}
	}
	return true
}
