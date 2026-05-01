package tool

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// ──────────────────────────────────────────────────────────────────────────────
// 1. Per-tool timeout configuration
// ──────────────────────────────────────────────────────────────────────────────

// ToolTimeout returns the default timeout for a given tool name.
// Callers may still override with an explicit per-invocation value.
func ToolTimeout(toolName string) time.Duration {
	switch toolName {
	case "Bash", "bash":
		return 120 * time.Second
	case "WebFetch", "web_fetch":
		return 30 * time.Second
	case "Grep", "grep":
		return 60 * time.Second
	default:
		return 60 * time.Second
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// 2. Output size limiting
// ──────────────────────────────────────────────────────────────────────────────

const maxOutputBytes = 50_000 // 50 KB

// TruncateOutput trims output to maxOutputBytes and appends an indicator.
func TruncateOutput(s string) string {
	if len(s) <= maxOutputBytes {
		return s
	}
	return s[:maxOutputBytes] + "\n[output truncated — showing first 50KB]"
}

// ──────────────────────────────────────────────────────────────────────────────
// 3. Destructive command detection (Bash)
// ──────────────────────────────────────────────────────────────────────────────

// destructivePatterns are additional patterns (beyond the existing
// dangerousSubstrings/suspiciousPatterns in bash.go) that the safety layer
// flags as destructive.  We purposefully keep these separate so the two lists
// are independently testable.
var destructivePatterns = []string{
	"rm -rf",
	"git reset --hard",
	"git push --force",
	"drop table",
	"truncate",
	"> /dev/sda",
	"dd if=",
	"mkfs",
	":(){ :|:& };:",
}

// IsDestructiveCommand returns true when the command contains a pattern that
// is considered destructive.  This is a superset intended for pre-execution
// gating — it catches broader patterns than bash.go's dangerousSubstrings
// (e.g. "rm -rf ." is already caught; this also catches bare "rm -rf").
func IsDestructiveCommand(command string) bool {
	lower := strings.ToLower(command)
	for _, pat := range destructivePatterns {
		if strings.Contains(lower, strings.ToLower(pat)) {
			return true
		}
	}
	return false
}

// ──────────────────────────────────────────────────────────────────────────────
// 4. Credential / secret detection (Write / Edit content)
// ──────────────────────────────────────────────────────────────────────────────

// credentialPatterns is a compiled set of regexes that match common secret
// formats.  All patterns are case-insensitive where appropriate.
var credentialPatterns = []*regexp.Regexp{
	// OpenAI-style secret keys
	regexp.MustCompile(`sk-[A-Za-z0-9]{20,}`),
	// AWS access key IDs
	regexp.MustCompile(`AKIA[0-9A-Z]{16}`),
	// GitHub personal access tokens (classic & fine-grained)
	regexp.MustCompile(`ghp_[A-Za-z0-9]{36,}`),
	// GitHub OAuth tokens
	regexp.MustCompile(`gho_[A-Za-z0-9]{36,}`),
	// PEM private keys
	regexp.MustCompile(`-----BEGIN (RSA |EC |OPENSSH )?PRIVATE KEY-----`),
	// Passwords embedded in connection strings (e.g. postgres://user:pass@...)
	regexp.MustCompile(`://[^:]+:[^@\s]+@`),
}

// DetectCredentials returns a non-empty description of the first credential
// pattern found in content, or "" if none match.
func DetectCredentials(content string) string {
	labels := []string{
		"OpenAI/secret key (sk-...)",
		"AWS access key (AKIA...)",
		"GitHub personal access token (ghp_...)",
		"GitHub OAuth token (gho_...)",
		"PEM private key",
		"password in connection string",
	}
	for i, re := range credentialPatterns {
		if re.MatchString(content) {
			return labels[i]
		}
	}
	return ""
}

// ──────────────────────────────────────────────────────────────────────────────
// 5. Sensitive-path blocking (Read / Write / Edit)
// ──────────────────────────────────────────────────────────────────────────────

// blockedPathSuffixes are path suffixes that should never be read or written.
var blockedPathSuffixes = []string{
	"/.ssh/id_rsa",
	"/.ssh/id_ed25519",
	"/.ssh/id_ecdsa",
	"/.ssh/id_dsa",
	"/.ssh/config",
	"/.ssh/known_hosts",
	"/.ssh/authorized_keys",
	"/.aws/credentials",
}

// blockedBasenames are file basenames that are blocked regardless of directory.
var blockedBasenames = []string{
	".env",
	"credentials.json",
}

// IsSensitivePath returns a non-empty reason when path points to a file
// that should be blocked for security.  The path is cleaned and, when
// possible, resolved through symlinks before checking.
func IsSensitivePath(path string) string {
	// Resolve to absolute + follow symlinks when possible.
	resolved := path
	if abs, err := filepath.Abs(path); err == nil {
		resolved = abs
	}
	if evaled, err := filepath.EvalSymlinks(resolved); err == nil {
		resolved = evaled
	}
	clean := filepath.Clean(resolved)

	home, _ := os.UserHomeDir()

	// Check suffix-based blocks (e.g. ~/.ssh/*)
	for _, suffix := range blockedPathSuffixes {
		blocked := filepath.Join(home, suffix[1:]) // strip leading /
		if clean == blocked {
			return fmt.Sprintf("access to %s is blocked for security", suffix)
		}
	}

	// ~/.ssh/* catch-all — block everything inside ~/.ssh
	if home != "" {
		sshDir := filepath.Join(home, ".ssh")
		if strings.HasPrefix(clean, sshDir+string(filepath.Separator)) || clean == sshDir {
			return "access to ~/.ssh is blocked for security"
		}
	}

	// ~/.env
	if home != "" && clean == filepath.Join(home, ".env") {
		return "access to ~/.env is blocked for security"
	}

	// Basename checks — blocks */.env and */credentials.json everywhere.
	base := filepath.Base(clean)
	for _, b := range blockedBasenames {
		if base == b {
			return fmt.Sprintf("access to %s files is blocked for security", b)
		}
	}

	return ""
}

// ──────────────────────────────────────────────────────────────────────────────
// 6. Symlink resolution (used by IsSensitivePath and callers)
// ──────────────────────────────────────────────────────────────────────────────

// ResolvePath returns the absolute, symlink-resolved path.
// If resolution fails it falls back to filepath.Abs.
func ResolvePath(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		// If the file does not exist yet (Write), resolve the parent.
		dir := filepath.Dir(abs)
		base := filepath.Base(abs)
		if rdir, err2 := filepath.EvalSymlinks(dir); err2 == nil {
			return filepath.Join(rdir, base), nil
		}
		return abs, nil
	}
	return resolved, nil
}

// ──────────────────────────────────────────────────────────────────────────────
// 7. Binary file detection (Read)
// ──────────────────────────────────────────────────────────────────────────────

const binaryProbeSize = 8192

// BinaryIndicator is the message returned instead of binary content.
const BinaryIndicator = "[binary file — not displaying]"

// IsBinaryContent returns true when data contains at least one null byte
// in the first binaryProbeSize bytes, indicating likely binary content.
func IsBinaryContent(data []byte) bool {
	end := len(data)
	if end > binaryProbeSize {
		end = binaryProbeSize
	}
	for i := 0; i < end; i++ {
		if data[i] == 0 {
			return true
		}
	}
	return false
}
