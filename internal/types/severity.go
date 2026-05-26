// Package types provides shared types used across GrayCodeAI hawk-related modules.
// Severity, TokenSeverity, and AuditSeverity are forwarded from the shared package.
// ToolCall and ToolResult come from eyrie/client.
package types

import (
	"github.com/GrayCodeAI/eyrie/client"
	"github.com/GrayCodeAI/hawk/shared/types"
)

// Severity represents the impact level of a finding.
type Severity = types.Severity

const (
	SeverityInfo     = types.SeverityInfo
	SeverityLow      = types.SeverityLow
	SeverityMedium   = types.SeverityMedium
	SeverityHigh     = types.SeverityHigh
	SeverityCritical = types.SeverityCritical
)

// ParseSeverity converts a string to a Severity.
var ParseSeverity = types.ParseSeverity

// TokenSeverity defines rule severity for compression error patterns.
type TokenSeverity = types.TokenSeverity

const (
	TokenSeverityCritical = types.TokenSeverityCritical
	TokenSeverityHigh     = types.TokenSeverityHigh
	TokenSeverityMedium   = types.TokenSeverityMedium
	TokenSeverityLow      = types.TokenSeverityLow
)

// AuditSeverity indicates how dangerous a security audit finding is.
type AuditSeverity = types.AuditSeverity

const (
	AuditSeverityCritical = types.AuditSeverityCritical
	AuditSeverityWarning  = types.AuditSeverityWarning
	AuditSeverityInfo     = types.AuditSeverityInfo
)

// ToolCall represents a tool invocation requested by the model.
type ToolCall = client.ToolCall

// ToolResult represents the result of a tool execution.
type ToolResult = client.ToolResult
