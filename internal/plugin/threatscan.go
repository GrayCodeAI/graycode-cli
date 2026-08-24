package plugin

import (
	"fmt"
	"regexp"
	"strings"
)

// Threat scanning for community skills, ported from Autohand Code CLI's
// SkillSecurityScanner (Apache-2.0, Copyright 2025 Autohand AI LLC).
// It is the second layer of defense after the Unicode audit in audit.go:
// auditContent catches homoglyph/prompt-injection characters, ScanThreats
// catches dangerous instructions in the skill body itself.

// ThreatCategory classifies the kind of dangerous content found.
type ThreatCategory string

const (
	ThreatCommandInjection ThreatCategory = "command-injection"
	ThreatDataExfiltration ThreatCategory = "data-exfiltration"
	ThreatEncodedPayload   ThreatCategory = "encoded-payload"
	ThreatNetworkCalls     ThreatCategory = "network-calls"
)

// Safe/Blocked thresholds on the 0-100 score (higher = safer).
const (
	threatSafeThreshold    = 50
	threatBlockedThreshold = 30
)

// ThreatMatch is a single threat-rule hit.
type ThreatMatch struct {
	Category ThreatCategory `json:"category"`
	Pattern  string         `json:"pattern"`
	Line     int            `json:"line"` // 1-indexed
	Context  string         `json:"context"`
}

// ThreatScanResult is the outcome of scanning one skill's content.
type ThreatScanResult struct {
	Score    int           `json:"score"` // 0-100, higher = safer
	Threats  []ThreatMatch `json:"threats,omitempty"`
	Blocked  bool          `json:"blocked"` // Score < threatBlockedThreshold: refuse install
	Warnings bool          `json:"warnings"`
}

// Safe reports whether the content passed without warnings.
func (r ThreatScanResult) Safe() bool { return r.Score >= threatSafeThreshold && len(r.Threats) == 0 }

type threatRule struct {
	category ThreatCategory
	pattern  *regexp.Regexp
	severity int // points deducted per match; higher = more severe
}

//nolint:lll // regex literals are unreadable when split
var threatRules = []threatRule{
	// Command injection
	{ThreatCommandInjection, regexp.MustCompile(`(?i)rm\s+-rf\b`), 15},
	{ThreatCommandInjection, regexp.MustCompile(`(?i)curl\s+.*\|\s*bash`), 20},
	{ThreatCommandInjection, regexp.MustCompile(`(?i)wget\s+.*\|\s*bash`), 20},
	{ThreatCommandInjection, regexp.MustCompile(`(?i)\beval\s*\(`), 15},
	{ThreatCommandInjection, regexp.MustCompile(`(?i)\bexec\s*\(`), 12},
	{ThreatCommandInjection, regexp.MustCompile(`(?i)\bsudo\b`), 15},
	{ThreatCommandInjection, regexp.MustCompile(`(?i)chmod\s+777\b`), 12},

	// Data exfiltration
	{ThreatDataExfiltration, regexp.MustCompile(`(?i)process\.env\b`), 10},
	{ThreatDataExfiltration, regexp.MustCompile(`(?i)\bcredentials?\b`), 8},
	{ThreatDataExfiltration, regexp.MustCompile(`(?i)\bapi[_-]?key\b`), 8},
	{ThreatDataExfiltration, regexp.MustCompile(`(?i)\bsecret[_-]?key\b`), 10},
	{ThreatDataExfiltration, regexp.MustCompile(`(?i)\.ssh/id_rsa\b`), 15},

	// Encoded payloads
	{ThreatEncodedPayload, regexp.MustCompile(`(?i)\batob\s*\(`), 12},
	{ThreatEncodedPayload, regexp.MustCompile(`(?i)Buffer\.from\s*\(`), 8},

	// Network calls
	{ThreatNetworkCalls, regexp.MustCompile("(?i)\\bfetch\\s*\\(\\s*[\"'`]https?://"), 10},
	{ThreatNetworkCalls, regexp.MustCompile(`(?i)\bwget\s+https?://`), 12},
	{ThreatNetworkCalls, regexp.MustCompile(`(?i)\bcurl\s+https?://`), 8},
	{ThreatNetworkCalls, regexp.MustCompile(`(?i)http\.request\s*\(`), 10},
	{ThreatNetworkCalls, regexp.MustCompile(`(?i)XMLHttpRequest`), 10},
}

// ScanThreats scans skill content for security threats and returns a
// safety score (0-100) with the matched rules. Empty content scores 100.
func ScanThreats(content string) ThreatScanResult {
	if strings.TrimSpace(content) == "" {
		return ThreatScanResult{Score: 100}
	}

	var threats []ThreatMatch
	deductions := 0
	for i, line := range strings.Split(content, "\n") {
		for _, rule := range threatRules {
			if rule.pattern.MatchString(line) {
				threats = append(threats, ThreatMatch{
					Category: rule.category,
					Pattern:  rule.pattern.String(),
					Line:     i + 1,
					Context:  strings.TrimSpace(line),
				})
				deductions += rule.severity
			}
		}
	}

	score := 100 - deductions
	if score < 0 {
		score = 0
	}
	return ThreatScanResult{
		Score:    score,
		Threats:  threats,
		Blocked:  score < threatBlockedThreshold,
		Warnings: score < threatSafeThreshold && score >= threatBlockedThreshold,
	}
}

// FormatThreatScan renders scan findings for CLI display.
func FormatThreatScan(name string, r ThreatScanResult) string {
	var b strings.Builder
	switch {
	case r.Blocked:
		fmt.Fprintf(&b, "  BLOCKED %s (score %d/100)\n", name, r.Score)
	case r.Warnings:
		fmt.Fprintf(&b, "  WARNING %s (score %d/100)\n", name, r.Score)
	default:
		return ""
	}
	for _, t := range r.Threats {
		fmt.Fprintf(&b, "    [%s] line %d: %s\n", t.Category, t.Line, t.Context)
	}
	return b.String()
}
