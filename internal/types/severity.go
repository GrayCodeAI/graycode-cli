// Package types provides Graycode-owned runtime types and shared compatibility aliases.
// Severity, TokenSeverity, and AuditSeverity are forwarded from internal/contracts/types.
// Provider-facing compatibility now lives in explicit adapters inside internal/types/client.go.
package types

import (
	contracts "github.com/GrayCodeAI/graycode-cli/internal/contracts/types"
)

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
var ParseSeverity = contracts.ParseSeverity

// TokenSeverity defines rule severity for compression error patterns.
type TokenSeverity = contracts.TokenSeverity

const (
	TokenSeverityCritical = contracts.TokenSeverityCritical
	TokenSeverityHigh     = contracts.TokenSeverityHigh
	TokenSeverityMedium   = contracts.TokenSeverityMedium
	TokenSeverityLow      = contracts.TokenSeverityLow
)

// AuditSeverity indicates how dangerous a security audit finding is.
type AuditSeverity = contracts.AuditSeverity

const (
	AuditSeverityCritical = contracts.AuditSeverityCritical
	AuditSeverityWarning  = contracts.AuditSeverityWarning
	AuditSeverityInfo     = contracts.AuditSeverityInfo
)
