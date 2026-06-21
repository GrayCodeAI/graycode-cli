package types

import "strings"

// Severity represents the impact level of a finding.
type Severity int

const (
	SeverityInfo Severity = iota
	SeverityLow
	SeverityMedium
	SeverityHigh
	SeverityCritical
)

var severityNames = [...]string{"info", "low", "medium", "high", "critical"}

func (s Severity) String() string {
	if int(s) < len(severityNames) {
		return severityNames[s]
	}
	return "unknown"
}

// ParseSeverity converts a string to a Severity.
func ParseSeverity(s string) Severity {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "critical":
		return SeverityCritical
	case "high":
		return SeverityHigh
	case "medium":
		return SeverityMedium
	case "low":
		return SeverityLow
	default:
		return SeverityInfo
	}
}

// AtLeast returns true if s >= threshold.
func (s Severity) AtLeast(threshold Severity) bool {
	return s >= threshold
}

// TokenSeverity defines rule severity for compression error patterns.
type TokenSeverity string

const (
	TokenSeverityCritical TokenSeverity = "critical"
	TokenSeverityHigh     TokenSeverity = "high"
	TokenSeverityMedium   TokenSeverity = "medium"
	TokenSeverityLow      TokenSeverity = "low"
)

// AuditSeverity indicates how dangerous a security audit finding is.
type AuditSeverity string

const (
	AuditSeverityCritical AuditSeverity = "CRITICAL"
	AuditSeverityWarning  AuditSeverity = "WARNING"
	AuditSeverityInfo     AuditSeverity = "INFO"
)
