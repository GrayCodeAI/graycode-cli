// Package types is a deprecated compatibility layer for shared Hawk ecosystem types.
// New cross-repo contracts belong in github.com/GrayCodeAI/hawk-core-contracts/types.
package types

import contracts "github.com/GrayCodeAI/hawk-core-contracts/types"

// Deprecated: use github.com/GrayCodeAI/hawk-core-contracts/types.Severity.
// Severity represents the impact level of a finding.
type Severity = contracts.Severity

const (
	SeverityInfo     = contracts.SeverityInfo
	SeverityLow      = contracts.SeverityLow
	SeverityMedium   = contracts.SeverityMedium
	SeverityHigh     = contracts.SeverityHigh
	SeverityCritical = contracts.SeverityCritical
)

// ParseSeverity converts a string to a Severity.
// Deprecated: use github.com/GrayCodeAI/hawk-core-contracts/types.ParseSeverity.
var ParseSeverity = contracts.ParseSeverity

// Deprecated: use github.com/GrayCodeAI/hawk-core-contracts/types.TokenSeverity.
// TokenSeverity defines rule severity for compression error patterns.
type TokenSeverity = contracts.TokenSeverity

const (
	TokenSeverityCritical = contracts.TokenSeverityCritical
	TokenSeverityHigh     = contracts.TokenSeverityHigh
	TokenSeverityMedium   = contracts.TokenSeverityMedium
	TokenSeverityLow      = contracts.TokenSeverityLow
)

// Deprecated: use github.com/GrayCodeAI/hawk-core-contracts/types.AuditSeverity.
// AuditSeverity indicates how dangerous a security audit finding is.
type AuditSeverity = contracts.AuditSeverity

const (
	AuditSeverityCritical = contracts.AuditSeverityCritical
	AuditSeverityWarning  = contracts.AuditSeverityWarning
	AuditSeverityInfo     = contracts.AuditSeverityInfo
)
