// Package swift builds a private diagnostic report (ported from fx's `/swift`
// slash command): a one-command snapshot of session context, logs, permissions,
// and recent activity rendered as a redactable markdown document.
package swift

import (
	"regexp"
	"strings"
)

// maskSecrets redacts "obvious" secrets from a line: bearer tokens, common
// secret-bearing key=value or JSON pairs, known token prefixes, and long
// hex/base64 runs. It is deliberately conservative — over-redaction is fine
// for a private diagnostic that will be shared after a human review.
func maskSecrets(s string) string {
	out := s
	for _, rule := range secretRules {
		out = rule.re.ReplaceAllString(out, rule.repl)
	}
	return out
}

type secretRule struct {
	re   *regexp.Regexp
	repl string
}

var secretRules = []secretRule{
	// "Bearer <token>"
	{regexp.MustCompile(`(?i)\b(bearer\s+)[A-Za-z0-9._~+/=-]{12,}`), `${1}*****`},
	// "token"/"secret"/"password"/"api_key"/"auth" =/<value>
	{regexp.MustCompile(`(?i)("?(?:api[_-]?key|secret|password|passwd|token|access[_-]?token|auth[_-]?key|auth)["' ]*\s*[:=]\s*["']?)[A-Za-z0-9._~+/=:-]{8,}`), `${1}*****`},
	// Well-known token/credential prefixes.
	{regexp.MustCompile(`(?i)\b(sk-ant-[A-Za-z0-9_-]{8,}|sk-[A-Za-z0-9_-]{8,}|xox[bap]-[A-Za-z0-9-]{8,}|ghp_[A-Za-z0-9]{8,}|github_pat_[A-Za-z0-9_]{8,}|glpat-[A-Za-z0-9_-]{8,}|AKIA[0-9A-Z]{16}|ai-[A-Za-z0-9_-]{8,})`), `*****`},
	// Long hex runs (>= 40 chars) — likely key material.
	{regexp.MustCompile(`\b[A-Fa-f0-9]{40,}\b`), `*****`},
	// Long base64 runs (>= 48 chars).
	{regexp.MustCompile(`\b[A-Za-z0-9+/]{48,}\b`), `*****`},
}

// stripANSI removes ANSI escape sequences (SGR/CSI/OSC) from a string, leaving
// only the visible text. Mirrors fx renderTimelineEntryBody's ANSI stripping.
func stripANSI(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	i := 0
	for i < len(s) {
		c := s[i]
		if c == 0x1b { // ESC
			if i+1 < len(s) {
				switch s[i+1] {
				case '[': // CSI
					i += 2
					for i < len(s) {
						cc := s[i]
						i++
						if cc >= 0x40 && cc <= 0x7e { // final byte
							break
						}
					}
					continue
				case ']': // OSC — skip to BEL or ST
					i += 2
					for i < len(s) {
						if s[i] == 0x07 {
							i++
							break
						}
						if i+1 < len(s) && s[i] == 0x1b && s[i+1] == '\\' {
							i += 2
							break
						}
						i++
					}
					continue
				default: // standalone ESC — drop it
					i++
					continue
				}
			}
			i++
			continue
		}
		b.WriteByte(c)
		i++
	}
	return b.String()
}
