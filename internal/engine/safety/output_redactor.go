package safety

import (
	"fmt"
	"os"
	"regexp"
	"strings"
	"sync"
)

// RedactPattern defines a regex pattern used to detect and redact sensitive information.
type RedactPattern struct {
	Name        string
	Pattern     *regexp.Regexp
	Replacement string
	Category    string // "api_key", "token", "password", "cert", "connection_string"
}

// RedactStats tracks cumulative redaction statistics.
type RedactStats struct {
	TotalRedacted int
	ByCategory    map[string]int
	BytesSaved    int
}

// secretEnvNames are environment variable names whose values are treated as
// secrets. RegisterEnvSecrets and RedactEnvVars both key off this list so the
// set stays in one place.
var secretEnvNames = []string{
	"AWS_SECRET_ACCESS_KEY",
	"AWS_SESSION_TOKEN",
	"GITHUB_TOKEN",
	"GH_TOKEN",
	"OPENAI_API_KEY",
	"ANTHROPIC_API_KEY",
	"DATABASE_URL",
	"REDIS_URL",
	"SECRET_KEY",
	"API_KEY",
	"AUTH_TOKEN",
	"ACCESS_TOKEN",
	"PRIVATE_KEY",
	"NPM_TOKEN",
	"SLACK_TOKEN",
	"STRIPE_SECRET_KEY",
	"SENDGRID_API_KEY",
	"TWILIO_AUTH_TOKEN",
	"HEROKU_API_KEY",
	"DOCKER_PASSWORD",
}

// OutputRedactor strips sensitive information from tool outputs before they reach the LLM.
type OutputRedactor struct {
	Patterns     []*RedactPattern
	KnownSecrets map[string]string
	Stats        RedactStats
	mu           sync.RWMutex
}

// NewOutputRedactor creates an OutputRedactor pre-loaded with 25+ built-in patterns
// covering common secret formats.
func NewOutputRedactor() *OutputRedactor {
	r := &OutputRedactor{
		KnownSecrets: make(map[string]string),
		Stats: RedactStats{
			ByCategory: make(map[string]int),
		},
	}

	// AWS access key IDs
	r.addBuiltin("aws_access_key", `AKIA[0-9A-Z]{16}`, "api_key")
	// AWS secret access keys (40-char base64 after common prefixes)
	r.addBuiltin("aws_secret_key", `(?i)(?:aws_secret_access_key|aws_secret)\s*[=:]\s*[A-Za-z0-9/+=]{40}`, "api_key")
	// AWS session tokens
	r.addBuiltin("aws_session_token", `(?i)(?:aws_session_token)\s*[=:]\s*[A-Za-z0-9/+=]{100,}`, "token")

	// GitHub tokens
	r.addBuiltin("github_pat", `ghp_[A-Za-z0-9]{36,}`, "token")
	r.addBuiltin("github_oauth", `gho_[A-Za-z0-9]{36,}`, "token")
	r.addBuiltin("github_app", `ghs_[A-Za-z0-9]{36,}`, "token")
	r.addBuiltin("github_refresh", `ghr_[A-Za-z0-9]{36,}`, "token")

	// Generic API keys with sk- prefix (OpenAI, Stripe, etc.)
	r.addBuiltin("sk_api_key", `sk-[A-Za-z0-9]{20,}`, "api_key")
	// Anthropic API keys (sk-ant-apiNN-...); must precede the generic sk- rule
	// to keep the hyphens matched.
	r.addBuiltin("anthropic_sk", `sk-ant-api\d{2}-[A-Za-z0-9_-]{20,}`, "api_key")
	// Generic API keys with key- prefix
	r.addBuiltin("key_prefix_api_key", `key-[A-Za-z0-9]{20,}`, "api_key")

	// Slack tokens
	r.addBuiltin("slack_token", `xox[baprs]-[A-Za-z0-9\-]{10,}`, "token")

	// Stripe keys
	r.addBuiltin("stripe_secret", `sk_live_[A-Za-z0-9]{20,}`, "api_key")
	r.addBuiltin("stripe_restricted", `rk_live_[A-Za-z0-9]{20,}`, "api_key")

	// Passwords in URLs (://user:pass@host)
	r.addBuiltin("password_in_url", `://[^:@\s]+:([^@\s]{3,})@`, "password")

	// Bearer tokens in headers
	r.addBuiltin("bearer_token", `(?i)bearer\s+[A-Za-z0-9\-._~+/]+=*`, "token")

	// Authorization headers with Basic auth
	r.addBuiltin("basic_auth", `(?i)basic\s+[A-Za-z0-9+/]+=*`, "token")

	// Private keys (PEM format)
	r.addBuiltin("rsa_private_key", `-----BEGIN RSA PRIVATE KEY-----[\s\S]*?-----END RSA PRIVATE KEY-----`, "cert")
	r.addBuiltin("ec_private_key", `-----BEGIN EC PRIVATE KEY-----[\s\S]*?-----END EC PRIVATE KEY-----`, "cert")
	r.addBuiltin("private_key_generic", `-----BEGIN PRIVATE KEY-----[\s\S]*?-----END PRIVATE KEY-----`, "cert")

	// Connection strings with passwords
	r.addBuiltin("postgres_conn", `(?i)postgres(?:ql)?://[^:@\s]+:([^@\s]+)@[^\s]+`, "connection_string")
	r.addBuiltin("mysql_conn", `(?i)mysql://[^:@\s]+:([^@\s]+)@[^\s]+`, "connection_string")
	r.addBuiltin("mongodb_conn", `(?i)mongodb(?:\+srv)?://[^:@\s]+:([^@\s]+)@[^\s]+`, "connection_string")
	r.addBuiltin("redis_conn", `(?i)redis://:[^@\s]+@[^\s]+`, "connection_string")

	// JWTs (three base64url segments separated by dots)
	r.addBuiltin("jwt", `eyJ[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}`, "token")

	// Heroku API key
	r.addBuiltin("heroku_api_key", `(?i)heroku[_\s]*api[_\s]*key\s*[=:]\s*[A-Fa-f0-9\-]{36,}`, "api_key")

	// npm tokens
	r.addBuiltin("npm_token", `(?i)npm_[A-Za-z0-9]{36,}`, "token")

	// SendGrid API key
	r.addBuiltin("sendgrid_key", `SG\.[A-Za-z0-9_\-]{22}\.[A-Za-z0-9_\-]{43}`, "api_key")

	// Generic secret assignment patterns (PASSWORD=..., SECRET=..., etc.)
	// Must be last so more specific patterns above match first.
	r.addBuiltin("env_secret_assignment", `(?i)\b(?:PASSWORD|SECRET|API_KEY|APIKEY|ACCESS_TOKEN|AUTH_TOKEN)\s*[=:]\s*["']?[^\s"'\[]{8,}["']?`, "password")

	return r
}

// addBuiltin registers a pattern during construction (no locking needed).
func (r *OutputRedactor) addBuiltin(name, pattern, category string) {
	re := regexp.MustCompile(pattern)
	r.Patterns = append(r.Patterns, &RedactPattern{
		Name:        name,
		Pattern:     re,
		Replacement: "[REDACTED:" + category + "]",
		Category:    category,
	})
}

// Redact applies all patterns and known secrets to the output, replacing
// matches with [REDACTED:<category>] placeholders. Returns the sanitized string.
func (r *OutputRedactor) Redact(output string) string {
	r.mu.Lock()
	defer r.mu.Unlock()

	result := output
	originalLen := len(output)

	// Apply regex patterns.
	for _, p := range r.Patterns {
		matches := p.Pattern.FindAllStringIndex(result, -1)
		if len(matches) > 0 {
			r.Stats.TotalRedacted += len(matches)
			r.Stats.ByCategory[p.Category] += len(matches)
			result = p.Pattern.ReplaceAllString(result, p.Replacement)
		}
	}

	// Apply known secrets.
	for name, secret := range r.KnownSecrets {
		if secret == "" {
			continue
		}
		count := strings.Count(result, secret)
		if count > 0 {
			r.Stats.TotalRedacted += count
			r.Stats.ByCategory["known_secret"] += count
			replacement := "[REDACTED:" + name + "]"
			result = strings.ReplaceAll(result, secret, replacement)
		}
	}

	saved := originalLen - len(result)
	if saved > 0 {
		r.Stats.BytesSaved += saved
	}

	return result
}

// AddKnownSecret registers a specific value to always redact from outputs.
// The name is used in the replacement placeholder.
func (r *OutputRedactor) AddKnownSecret(name, value string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.KnownSecrets[name] = value
}

// RegisterEnvSecrets imports the values of secret-named environment variables
// (see secretEnvNames) into the known-secrets table so tool output that echoes
// them is redacted before it reaches the model. Values shorter than 8 bytes are
// skipped to avoid mangling short, non-secret values. Safe to call repeatedly.
func (r *OutputRedactor) RegisterEnvSecrets() {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, envName := range secretEnvNames {
		if val := strings.TrimSpace(os.Getenv(envName)); len(val) >= 8 {
			r.KnownSecrets["env:"+envName] = val
		}
	}
}

// RedactEnvVars scans the output for values of known environment variables
// whose names suggest they contain secrets, and replaces them.
func (r *OutputRedactor) RedactEnvVars(output string) string {
	r.mu.Lock()
	defer r.mu.Unlock()

	result := output
	originalLen := len(output)

	for _, envName := range secretEnvNames {
		val, ok := r.KnownSecrets["env:"+envName]
		if !ok {
			continue
		}
		if val == "" {
			continue
		}
		count := strings.Count(result, val)
		if count > 0 {
			r.Stats.TotalRedacted += count
			r.Stats.ByCategory["env_var"] += count
			result = strings.ReplaceAll(result, val, "[REDACTED:env:"+envName+"]")
		}
	}

	saved := originalLen - len(result)
	if saved > 0 {
		r.Stats.BytesSaved += saved
	}

	return result
}

// RedactPaths replaces the user's home directory in output with ~/ to avoid
// leaking the full filesystem path structure.
func (r *OutputRedactor) RedactPaths(output string, homedir string) string {
	if homedir == "" {
		return output
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	// Ensure homedir does not have a trailing slash for consistent replacement.
	homedir = strings.TrimSuffix(homedir, "/")
	if homedir == "" {
		return output
	}

	count := strings.Count(output, homedir)
	if count > 0 {
		r.Stats.TotalRedacted += count
		r.Stats.ByCategory["path"] += count
		originalLen := len(output)
		result := strings.ReplaceAll(output, homedir, "~")
		saved := originalLen - len(result)
		if saved > 0 {
			r.Stats.BytesSaved += saved
		}
		return result
	}

	return output
}

// IsClean performs a quick check to determine whether the output contains any
// detectable secrets. Returns true if no secrets are found.
func (r *OutputRedactor) IsClean(output string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, p := range r.Patterns {
		if p.Pattern.MatchString(output) {
			return false
		}
	}

	for _, secret := range r.KnownSecrets {
		if secret != "" && strings.Contains(output, secret) {
			return false
		}
	}

	return true
}

// FormatStats returns a human-readable summary of redaction statistics.
func (r *OutputRedactor) FormatStats() string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if r.Stats.TotalRedacted == 0 {
		return "Redaction Stats:\nTotal: 0 secrets redacted\n"
	}

	var sb strings.Builder
	sb.WriteString("Redaction Stats:\n")
	sb.WriteString(fmt.Sprintf("Total: %d secrets redacted\n", r.Stats.TotalRedacted))

	if len(r.Stats.ByCategory) > 0 {
		sb.WriteString("By type: ")
		first := true
		for cat, count := range r.Stats.ByCategory {
			if !first {
				sb.WriteString(", ")
			}
			sb.WriteString(fmt.Sprintf("%s (%d)", cat, count))
			first = false
		}
		sb.WriteString("\n")
	}

	sb.WriteString(fmt.Sprintf("Bytes saved: %s\n", formatBytes(r.Stats.BytesSaved)))

	return sb.String()
}

// AddPattern registers a new redaction pattern at runtime. Returns an error
// if the pattern cannot be compiled.
func (r *OutputRedactor) AddPattern(name string, pattern string, category string) error {
	re, err := regexp.Compile(pattern)
	if err != nil {
		return fmt.Errorf("invalid redaction pattern %q: %w", name, err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	newPattern := &RedactPattern{
		Name:        name,
		Pattern:     re,
		Replacement: "[REDACTED:" + category + "]",
		Category:    category,
	}

	// Insert before the last pattern (generic catch-all) so custom patterns
	// take priority over env_secret_assignment.
	if len(r.Patterns) > 0 {
		last := r.Patterns[len(r.Patterns)-1]
		r.Patterns[len(r.Patterns)-1] = newPattern
		r.Patterns = append(r.Patterns, last)
	} else {
		r.Patterns = append(r.Patterns, newPattern)
	}

	return nil
}

// formatBytes renders a byte count with comma separators for readability.
func formatBytes(n int) string {
	if n < 0 {
		return "0"
	}
	s := fmt.Sprintf("%d", n)
	if len(s) <= 3 {
		return s
	}

	var result strings.Builder
	remainder := len(s) % 3
	if remainder > 0 {
		result.WriteString(s[:remainder])
		if len(s) > remainder {
			result.WriteString(",")
		}
	}
	for i := remainder; i < len(s); i += 3 {
		if i > remainder {
			result.WriteString(",")
		}
		result.WriteString(s[i : i+3])
	}
	return result.String()
}
