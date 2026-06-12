package tool

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/GrayCodeAI/hawk/internal/env"
	"github.com/GrayCodeAI/hawk/internal/home"
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

const maxOutputBytes = 500_000 // 500 KB — tune this if your tool outputs are routinely larger

// TruncateOutput trims output to maxOutputBytes and appends an indicator.
func TruncateOutput(s string) string {
	if len(s) <= maxOutputBytes {
		return s
	}
	// Truncate at rune boundary to avoid splitting multi-byte UTF-8 characters.
	truncated := s[:maxOutputBytes]
	for i := len(truncated) - 1; i >= 0; i-- {
		b := truncated[i]
		if b&0xC0 != 0x80 {
			// Found the start byte of a UTF-8 sequence.
			// Determine how many continuation bytes follow it.
			seqLen := 1
			if b&0xE0 == 0xC0 {
				seqLen = 2
			} else if b&0xF0 == 0xE0 {
				seqLen = 3
			} else if b&0xF8 == 0xF0 {
				seqLen = 4
			}
			// Include the full sequence if it fits, otherwise drop the whole character.
			if i+seqLen <= len(truncated) {
				truncated = truncated[:i+seqLen]
			} else {
				truncated = truncated[:i]
			}
			break
		}
	}
	return truncated + "\n[output truncated — showing first 500KB]"
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
	// Check full command first (catches multi-char patterns like fork bombs)
	lower := strings.ToLower(command)
	for _, pat := range destructivePatterns {
		if strings.Contains(lower, strings.ToLower(pat)) {
			return true
		}
	}
	// Also check each segment independently
	for _, seg := range SegmentCommand(command) {
		segLower := strings.ToLower(seg)
		for _, pat := range destructivePatterns {
			if strings.Contains(segLower, strings.ToLower(pat)) {
				return true
			}
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
	// Anthropic API keys (must precede generic sk- pattern)
	regexp.MustCompile(`sk-ant-api\d{2}-[A-Za-z0-9_-]{20,}`),
	// OpenAI-style secret keys
	regexp.MustCompile(`sk-[A-Za-z0-9]{20,}`),
	// AWS access key IDs
	regexp.MustCompile(`AKIA[0-9A-Z]{16}`),
	// GitHub personal access tokens (classic & fine-grained)
	regexp.MustCompile(`ghp_[A-Za-z0-9]{36,}`),
	// GitHub OAuth tokens
	regexp.MustCompile(`gho_[A-Za-z0-9]{36,}`),
	// GitLab personal access tokens
	regexp.MustCompile(`glpat-[A-Za-z0-9_-]{20,}`),
	// Slack bot/user tokens
	regexp.MustCompile(`xox[bpsar]-[A-Za-z0-9-]{10,}`),
	// Stripe secret keys (live and test)
	regexp.MustCompile(`[sr]k_(live|test)_[A-Za-z0-9]{20,}`),
	// PEM private keys
	regexp.MustCompile(`-----BEGIN (RSA |EC |OPENSSH )?PRIVATE KEY-----`),
	// Passwords embedded in connection strings (e.g. postgres://user:pass@host/...)
	// Requires a password component (non-empty between : and @) and a host component after @.
	regexp.MustCompile(`://[^:]+:[^@\s]+@[^/\s]+`),
}

// DetectCredentials returns a non-empty description of the first credential
// pattern found in content, or "" if none match.
func DetectCredentials(content string) string {
	labels := []string{
		"Anthropic API key (sk-ant-...)",
		"OpenAI/secret key (sk-...)",
		"AWS access key (AKIA...)",
		"GitHub personal access token (ghp_...)",
		"GitHub OAuth token (gho_...)",
		"GitLab personal access token (glpat-...)",
		"Slack token (xox...)",
		"Stripe secret key",
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
	".npmrc",
	".netrc",
	".pgpass",
	"kubeconfig",
	"token.json",
	"service-account.json",
	"credentials.yaml",
	"credentials.yml",
	"credentials.xml",
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

	home := home.Dir()

	if home != "" {
		hawkProv := filepath.Join(home, ".hawk", "provider.json")
		if clean == hawkProv {
			return "access to ~/.hawk/provider.json is blocked for security (API credentials)"
		}
		hawkEnv := filepath.Join(home, ".hawk", "env")
		if clean == hawkEnv {
			return "access to ~/.hawk/env is blocked for security (API keys)"
		}
		hawkDotEnv := filepath.Join(home, ".hawk", ".env")
		if clean == hawkDotEnv {
			return "access to ~/.hawk/.env is blocked for security (API keys)"
		}
	}

	if cfgDir := strings.TrimSpace(env.Getenv("HAWK_CONFIG_DIR")); cfgDir != "" {
		customProv := filepath.Clean(filepath.Join(cfgDir, "provider.json"))
		if clean == customProv {
			return "access to provider.json is blocked for security (API credentials)"
		}
		customEnv := filepath.Clean(filepath.Join(cfgDir, "env"))
		if clean == customEnv {
			return "access to hawk env file is blocked for security (API keys)"
		}
		customDotEnv := filepath.Clean(filepath.Join(cfgDir, ".env"))
		if clean == customDotEnv {
			return "access to hawk .env is blocked for security (API keys)"
		}
	}

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

	// Basename checks — blocks */.env and */credentials.json everywhere,
	// plus common .env variants (.env.local, .env.production, .env.backup, etc.)
	base := filepath.Base(clean)
	for _, b := range blockedBasenames {
		if base == b {
			return fmt.Sprintf("access to %s files is blocked for security", b)
		}
	}
	// Block any file starting with ".env" (catches .env.local, .env.production, .env.backup, etc.)
	if strings.HasPrefix(base, ".env") && base != ".envrc" {
		return fmt.Sprintf("access to %s files is blocked for security", base)
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

// IsBinaryContent returns true when data appears to be binary, detected by:
// - Null bytes in the first binaryProbeSize bytes (classic binary indicator)
// - High ratio of non-text bytes (control characters other than \n, \r, \t)
func IsBinaryContent(data []byte) bool {
	end := len(data)
	if end > binaryProbeSize {
		end = binaryProbeSize
	}
	nonText := 0
	for i := 0; i < end; i++ {
		b := data[i]
		if b == 0 {
			return true // null byte is a definitive binary indicator
		}
		// Count control characters other than common text ones (tab, newline, carriage return)
		if b < 0x20 && b != '\t' && b != '\n' && b != '\r' {
			nonText++
		}
	}
	// If more than 30% of the probed bytes are non-text control chars, treat as binary.
	return end > 0 && float64(nonText)/float64(end) > 0.3
}

// ──────────────────────────────────────────────────────────────────────────────
// 8. SSRF protection (WebFetch / Download)
// ──────────────────────────────────────────────────────────────────────────────

// privateIPBlocks are CIDR ranges that should never be fetched by external tools.
var privateIPBlocks []*net.IPNet

func init() {
	for _, cidr := range []string{
		"127.0.0.0/8",    // loopback
		"10.0.0.0/8",     // private
		"172.16.0.0/12",  // private
		"192.168.0.0/16", // private
		"169.254.0.0/16", // link-local / cloud metadata
		"::1/128",        // IPv6 loopback
		"fc00::/7",       // IPv6 unique local
		"fe80::/10",      // IPv6 link-local
	} {
		_, block, _ := net.ParseCIDR(cidr)
		if block != nil {
			privateIPBlocks = append(privateIPBlocks, block)
		}
	}
}

// ssrfSkipKey is a context key that, when set, disables SSRF validation.
// Used by tests that run httptest servers on localhost.
type ssrfSkipKey struct{}

// WithSSRFSkip returns a context that skips SSRF URL validation.
func WithSSRFSkip(ctx context.Context) context.Context {
	return context.WithValue(ctx, ssrfSkipKey{}, true)
}

// validateURLPublic rejects URLs that resolve to private/link-local IP ranges
// to prevent SSRF attacks (e.g., fetching AWS metadata at 169.254.169.254).
// Returns the validated URL with the resolved IP pinned as the host, preventing
// DNS rebinding attacks where the second resolution returns a private IP.
func validateURLPublic(ctx context.Context, rawURL string) (string, error) {
	if ctx.Value(ssrfSkipKey{}) != nil {
		return rawURL, nil
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", fmt.Errorf("invalid URL: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", fmt.Errorf("blocked: only http/https URLs are allowed")
	}

	host := u.Hostname()
	if host == "" {
		return "", fmt.Errorf("blocked: URL has no host")
	}

	// Resolve the hostname to check against private ranges.
	addrs, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		// DNS failure — block the request rather than allowing potential SSRF bypass.
		return "", fmt.Errorf("blocked: DNS resolution failed for %q: %w", host, err)
	}
	var safeIP string
	for _, addr := range addrs {
		ip := addr.IP
		for _, block := range privateIPBlocks {
			if block.Contains(ip) {
				return "", fmt.Errorf("blocked: URL %q resolves to private IP %s", rawURL, ip)
			}
		}
		if safeIP == "" {
			safeIP = ip.String()
		}
	}
	if safeIP == "" {
		return "", fmt.Errorf("blocked: URL %q resolved to no addresses", rawURL)
	}

	// Pin the IP to prevent DNS rebinding: replace host with the validated IP.
	// Preserve the original Host header via a separate mechanism if needed.
	if u.Port() != "" {
		u.Host = net.JoinHostPort(safeIP, u.Port())
	} else {
		u.Host = safeIP
	}
	return u.String(), nil
}

// ssrfSafeClient returns an http.Client that validates redirect targets
// against private IP ranges, preventing SSRF via redirect chains.
// The redirect validator pins the resolved IP so that subsequent connections
// in the chain cannot be rebinding to a different (private) address.
func ssrfSafeClient(ctx context.Context, timeout time.Duration) *http.Client {
	return &http.Client{
		Timeout: timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return fmt.Errorf("too many redirects")
			}
			if ctx.Value(ssrfSkipKey{}) != nil {
				return nil
			}
			pinned, err := validateURLPublic(ctx, req.URL.String())
			if err != nil {
				return err
			}
			// Replace the redirect URL with the IP-pinned version so the
			// transport dials the validated address instead of resolving DNS again.
			parsed, parseErr := url.Parse(pinned)
			if parseErr != nil {
				return parseErr
			}
			req.URL = parsed
			return nil
		},
	}
}
