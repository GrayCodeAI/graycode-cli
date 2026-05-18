// Package types provides shared type definitions for backward compatibility.
// Deprecated: Use github.com/GrayCodeAI/hawk/internal/types instead.
package types

import "github.com/GrayCodeAI/hawk/internal/types"

// Re-export all types from internal/types for backward compatibility.
type Severity = types.Severity
type TokenSeverity = types.TokenSeverity
type AuditSeverity = types.AuditSeverity
type ToolCall = types.ToolCall
type ToolResult = types.ToolResult

// Severity constants
const (
	SeverityInfo     = types.SeverityInfo
	SeverityLow      = types.SeverityLow
	SeverityMedium   = types.SeverityMedium
	SeverityHigh     = types.SeverityHigh
	SeverityCritical = types.SeverityCritical
)

// TokenSeverity constants
const (
	TokenSeverityCritical = types.TokenSeverityCritical
	TokenSeverityHigh     = types.TokenSeverityHigh
	TokenSeverityMedium   = types.TokenSeverityMedium
	TokenSeverityLow      = types.TokenSeverityLow
)

// AuditSeverity constants
const (
	AuditSeverityCritical = types.AuditSeverityCritical
	AuditSeverityWarning  = types.AuditSeverityWarning
	AuditSeverityInfo     = types.AuditSeverityInfo
)
