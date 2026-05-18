// Package types defines stable shared types for GrayCodeAI libraries (sight, inspect, tok, …).
// Implementations live in github.com/GrayCodeAI/hawk/internal/types; this package forwards them
// so consumers do not import internal paths from another module.
package types

import hawkinternal "github.com/GrayCodeAI/hawk/internal/types"

type (
	Severity      = hawkinternal.Severity
	TokenSeverity = hawkinternal.TokenSeverity
	AuditSeverity = hawkinternal.AuditSeverity
	ToolCall      = hawkinternal.ToolCall
	ToolResult    = hawkinternal.ToolResult
)

const (
	SeverityInfo     = hawkinternal.SeverityInfo
	SeverityLow      = hawkinternal.SeverityLow
	SeverityMedium   = hawkinternal.SeverityMedium
	SeverityHigh     = hawkinternal.SeverityHigh
	SeverityCritical = hawkinternal.SeverityCritical
)

const (
	TokenSeverityCritical = hawkinternal.TokenSeverityCritical
	TokenSeverityHigh     = hawkinternal.TokenSeverityHigh
	TokenSeverityMedium   = hawkinternal.TokenSeverityMedium
	TokenSeverityLow      = hawkinternal.TokenSeverityLow
)

const (
	AuditSeverityCritical = hawkinternal.AuditSeverityCritical
	AuditSeverityWarning  = hawkinternal.AuditSeverityWarning
	AuditSeverityInfo     = hawkinternal.AuditSeverityInfo
)

// ParseSeverity converts a string to a Severity (delegates to hawk internal/types).
var ParseSeverity = hawkinternal.ParseSeverity
