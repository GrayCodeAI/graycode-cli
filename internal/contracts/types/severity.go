// Vendored from github.com/GrayCodeAI/eagle/types at v0.0.0-20260902153929-5877bed17503 (MIT, Copyright (c) 2026 GrayCode AI).
// The upstream repository no longer exists; this copy is owned by Graycode as its contract surface.
package types

import (
	"fmt"
	"strings"
)

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
	if int(s) >= 0 && int(s) < len(severityNames) {
		return severityNames[s]
	}
	return "unknown"
}

// ParseSeverity converts a string to a Severity.
//
// Deprecated: ParseSeverity fails open — unknown input (typos such as
// "critcal", empty strings, arbitrary text) silently maps to SeverityInfo,
// so a malformed value is indistinguishable from a legitimate "info".
// Callers handling untrusted input should use ParseSeverityStrict, which
// reports unknown values as errors instead.
func ParseSeverity(s string) Severity {
	sev, _ := ParseSeverityStrict(s)
	return sev
}

// ParseSeverityStrict converts a string to a Severity, reporting unknown
// values as errors instead of failing open to SeverityInfo. Matching is
// case-insensitive and ignores surrounding whitespace, exactly like
// ParseSeverity; the two accept the same set of valid names.
func ParseSeverityStrict(s string) (Severity, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "critical":
		return SeverityCritical, nil
	case "high":
		return SeverityHigh, nil
	case "medium":
		return SeverityMedium, nil
	case "low":
		return SeverityLow, nil
	case "info":
		return SeverityInfo, nil
	default:
		return SeverityInfo, fmt.Errorf("unknown severity %q (want one of info, low, medium, high, critical)", s)
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
