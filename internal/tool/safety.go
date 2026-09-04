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

	"github.com/GrayCodeAI/graycode-cli/internal/env"
	"github.com/GrayCodeAI/graycode-cli/internal/home"
	"github.com/GrayCodeAI/graycode-cli/internal/storage"
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
	// find -delete and find -exec rm are rm-equivalent and must be hard-blocked
	// because they bypass the dangerousSubstrings check (no literal "rm" in the
	// command). Caught by IsDestructiveCommand so background tasks (which
	// skip the IsSuspicious permission prompt) are still blocked.
	//
	// The trailing-word form (e.g. "find -delete", "find -exec rm") below
	// matches the canonical forms. The "find ... -delete" mid-command form
	// is caught separately by findDeleteFlagRe below.
	"find -delete",
	"find -exec rm",
	"find -execdir rm",
}

// findDeleteFlagRe matches the `-delete` flag in any position of a find
// command (e.g. "find /tmp -type f -name '*.log' -delete"). The -delete
// flag is rm-equivalent and must be hard-blocked even when it appears
// mid-command.
var findDeleteFlagRe = regexp.MustCompile(`(?:^|\s)find\b[^\n;&|]*-delete\b`)

// findExecRmRe matches "find ... -exec rm" / "-execdir rm" patterns with
// any number of intervening flags. The `-exec rm` form is rm-equivalent.
var findExecRmRe = regexp.MustCompile(`(?:^|\s)find\b[^\n;&|]*-exec(?:dir)?\s+rm\b`)

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
	// find -delete / find -exec rm with intervening flags (e.g.
	// "find /tmp -type f -name '*.log' -delete" or
	// "find . -name '*.tmp' -exec rm {} +")
	if findDeleteFlagRe.MatchString(command) {
		return true
	}
	if findExecRmRe.MatchString(command) {
		return true
	}
	// Also check each segment independently
	for _, seg := range SegmentCommand(command) {
		segLower := strings.ToLower(seg)
		for _, pat := range destructivePatterns {
			if strings.Contains(segLower, strings.ToLower(pat)) {
				return true
			}
		}
		if findDeleteFlagRe.MatchString(seg) {
			return true
		}
		if findExecRmRe.MatchString(seg) {
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
	"secrets.txt",
	"secrets.yaml",
	"secrets.yml",
	"secrets.json",
	".git-credentials",
	".htpasswd",
	"id_rsa",
	"id_ed25519",
	"id_ecdsa",
	"id_dsa",
}

func matchesResolvedPath(cleanPath, candidate string) bool {
	resolved := candidate
	if canonical, err := ResolvePath(candidate); err == nil {
		resolved = canonical
	}
	return cleanPath == filepath.Clean(resolved)
}

// IsSensitivePath returns a non-empty reason when path points to a file
// that should be blocked for security.  The path is cleaned and, when
// possible, resolved through symlinks before checking.
func IsSensitivePath(path string) string {
	// Resolve to absolute + follow symlinks when possible, including a
	// symlinked parent for a file that does not exist yet (the Write case).
	resolved := path
	if canonical, err := ResolvePath(path); err == nil {
		resolved = canonical
	}
	clean := filepath.Clean(resolved)

	homeDir := home.MustDir()

	if homeDir != "" {
		graycodeProv := filepath.Join(homeDir, ".graycode", "provider.json")
		if clean == graycodeProv {
			return "access to ~/.graycode/provider.json is blocked for security (API credentials)"
		}
		graycodeEnv := filepath.Join(homeDir, ".graycode", "env")
		if clean == graycodeEnv {
			return "access to ~/.graycode/env is blocked for security (API keys)"
		}
		graycodeDotEnv := filepath.Join(homeDir, ".graycode", ".env")
		if clean == graycodeDotEnv {
			return "access to ~/.graycode/.env is blocked for security (API keys)"
		}
	}

	if matchesResolvedPath(clean, storage.ProviderConfigPath()) {
		return "access to provider.json is blocked for security (API credentials)"
	}

	if cfgDir := strings.TrimSpace(env.Getenv("GRAYCODE_CONFIG_DIR")); cfgDir != "" {
		customProv := filepath.Join(cfgDir, "provider.json")
		if matchesResolvedPath(clean, customProv) {
			return "access to provider.json is blocked for security (API credentials)"
		}
		customEnv := filepath.Join(cfgDir, "env")
		if matchesResolvedPath(clean, customEnv) {
			return "access to graycode env file is blocked for security (API keys)"
		}
		customDotEnv := filepath.Join(cfgDir, ".env")
		if matchesResolvedPath(clean, customDotEnv) {
			return "access to graycode .env is blocked for security (API keys)"
		}
	}

	// Check suffix-based blocks (e.g. ~/.ssh/*)
	for _, suffix := range blockedPathSuffixes {
		blocked := filepath.Join(homeDir, suffix[1:]) // strip leading /
		if clean == blocked {
			return fmt.Sprintf("access to %s is blocked for security", suffix)
		}
	}

	// ~/.ssh/* catch-all — block everything inside ~/.ssh
	if homeDir != "" {
		sshDir := filepath.Join(homeDir, ".ssh")
		if strings.HasPrefix(clean, sshDir+string(filepath.Separator)) || clean == sshDir {
			return "access to ~/.ssh is blocked for security"
		}
	}

	// ~/.env
	if homeDir != "" && clean == filepath.Join(homeDir, ".env") {
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

// commandPathSeparators splits a shell command into path-like tokens.
func commandPathSeparators(r rune) bool {
	switch r {
	case ' ', '\t', '\n', '\r', ';', '|', '&', '<', '>', '(', ')', '"', '\'', '`', '=':
		return true
	}
	return false
}

func expandCommandPathVariables(command string) string {
	command = strings.ReplaceAll(command, `\ `, " ")
	for _, item := range []struct {
		name  string
		value string
	}{
		{name: "HOME", value: home.MustDir()},
		{name: "GRAYCODE_CONFIG_DIR", value: strings.TrimSpace(env.Getenv("GRAYCODE_CONFIG_DIR"))},
		{name: "GRAYCODE_ROUTER_CONFIG_DIR", value: strings.TrimSpace(env.Getenv("GRAYCODE_ROUTER_CONFIG_DIR"))},
	} {
		if item.value == "" {
			continue
		}
		command = strings.ReplaceAll(command, "${"+item.name+"}", item.value)
		command = strings.ReplaceAll(command, "$"+item.name, item.value)
	}
	return strings.NewReplacer(`"`, "", `'`, "").Replace(command)
}

// CommandReferencesSensitivePath returns a non-empty reason when a shell
// command string references a credential file that the file tools already
// block via IsSensitivePath (SSH keys, ~/.aws/credentials, .env, provider
// configs, …). Without this, Bash is a trivial bypass of that protection
// ("cat ~/.ssh/id_rsa"). Suffix/basename checks cover common forms, while
// IsSensitivePath keeps configured and symlinked provider paths aligned with
// the file tools.
func CommandReferencesSensitivePath(command string) string {
	// Do not try to emulate every shell parameter-expansion form here. An
	// GRAYCODE_ROUTER_CONFIG_DIR reference is itself sensitive because the variable
	// identifies the credential-bearing provider-state directory. This also
	// closes modifier forms such as ${GRAYCODE_ROUTER_CONFIG_DIR%/}/provider.json.
	if strings.Contains(command, "GRAYCODE_ROUTER_CONFIG_DIR") {
		return "command references GRAYCODE_ROUTER_CONFIG_DIR, blocked for security"
	}
	if !strings.ContainsAny(command, "/.") {
		return ""
	}
	command = expandCommandPathVariables(command)
	configuredPaths := []string{storage.ProviderConfigPath()}
	if cfgDir := strings.TrimSpace(env.Getenv("GRAYCODE_CONFIG_DIR")); cfgDir != "" {
		configuredPaths = append(configuredPaths, filepath.Join(cfgDir, "provider.json"), filepath.Join(cfgDir, "env"), filepath.Join(cfgDir, ".env"))
	}
	for _, candidate := range configuredPaths {
		if candidate != "" && strings.Contains(command, candidate) {
			return "command references a configured credential path, blocked for security"
		}
	}
	homeDir := home.MustDir()
	for _, tok := range strings.FieldsFunc(command, commandPathSeparators) {
		tok = strings.Trim(tok, ",:")
		if tok == "" || !strings.ContainsAny(tok, "/.") {
			continue
		}
		// Expand a leading tilde so ~/.ssh/id_rsa and $HOME-relative forms
		// normalize to the same shape as absolute paths.
		if homeDir != "" {
			if strings.HasPrefix(tok, "~/") {
				tok = homeDir + tok[1:]
			} else if strings.HasPrefix(tok, "$HOME/") {
				tok = homeDir + tok[len("$HOME"):]
			}
		}
		// Keep Bash aligned with the Read/Edit/Write path policy, including a
		// provider.json rooted at GRAYCODE_ROUTER_CONFIG_DIR. ResolvePath also handles
		// relative custom config directories and symlinked parents.
		if reason := IsSensitivePath(tok); reason != "" {
			return "command references a sensitive path: " + reason
		}
		for _, suffix := range blockedPathSuffixes {
			if strings.HasSuffix(tok, suffix) || tok == suffix[1:] {
				return fmt.Sprintf("command references %s, blocked for security", suffix)
			}
		}
		if strings.Contains(tok, "/.ssh/") || strings.HasSuffix(tok, "/.ssh") {
			return "command references ~/.ssh, blocked for security"
		}
		if strings.HasSuffix(tok, ".graycode/provider.json") {
			return "command references provider.json, blocked for security (API credentials)"
		}
		base := tok
		if i := strings.LastIndexByte(base, '/'); i >= 0 {
			base = base[i+1:]
		}
		for _, b := range blockedBasenames {
			if base == b {
				return fmt.Sprintf("command references %s, blocked for security", b)
			}
		}
		if strings.HasPrefix(base, ".env") && base != ".envrc" {
			return fmt.Sprintf("command references %s, blocked for security", base)
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
		"0.0.0.0/8",      // "this network" (RFC 1122)
		"10.0.0.0/8",     // private
		"172.16.0.0/12",  // private
		"192.168.0.0/16", // private
		"169.254.0.0/16", // link-local / cloud metadata
		"100.64.0.0/10",  // CGN (RFC 6598)
		"198.18.0.0/15",  // benchmark testing (RFC 2544)
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
// Returns the validated URL with the resolved IP pinned as the host (preventing
// DNS rebinding) and the original hostname (so callers can preserve the Host
// header for virtual-host routing).
func validateURLPublic(ctx context.Context, rawURL string) (pinnedURL, originalHost string, err error) {
	if ctx.Value(ssrfSkipKey{}) != nil {
		return rawURL, "", nil
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", "", fmt.Errorf("invalid URL: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", "", fmt.Errorf("blocked: only http/https URLs are allowed")
	}

	host := u.Hostname()
	if host == "" {
		return "", "", fmt.Errorf("blocked: URL has no host")
	}

	// Resolve the hostname to check against private ranges.
	addrs, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		// DNS failure — block the request rather than allowing potential SSRF bypass.
		return "", "", fmt.Errorf("blocked: DNS resolution failed for %q: %w", host, err)
	}
	var safeIP string
	for _, addr := range addrs {
		ip := addr.IP
		// net.IPNet.Contains calls ip.To4() internally, so IPv4-mapped IPv6
		// addresses (::ffff:a.b.c.d) are correctly checked against IPv4 CIDR
		// blocks — no separate handling needed.
		for _, block := range privateIPBlocks {
			if block.Contains(ip) {
				return "", "", fmt.Errorf("blocked: URL %q resolves to private IP %s", rawURL, ip)
			}
		}
		if safeIP == "" {
			safeIP = ip.String()
		}
	}
	if safeIP == "" {
		return "", "", fmt.Errorf("blocked: URL %q resolved to no addresses", rawURL)
	}

	// Pin the IP to prevent DNS rebinding: replace host with the validated IP.
	// The caller should set req.Host to originalHost to preserve virtual-host
	// routing (most web servers route by Host header, not by IP).
	if u.Port() != "" {
		u.Host = net.JoinHostPort(safeIP, u.Port())
	} else {
		u.Host = safeIP
	}
	return u.String(), host, nil
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
			pinned, origHost, err := validateURLPublic(ctx, req.URL.String())
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
			// Preserve the original Host header so virtual-host routing
			// works correctly on the redirect target.
			if origHost != "" {
				req.Host = origHost
			}
			return nil
		},
	}
}
