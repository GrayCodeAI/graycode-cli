// verdict.go: a unified PermissionVerdict type that consolidates the
// outcome shape across all permission subsystems (guardian, rules
// DSL, boundary checker, tool dispatcher, hooks, etc.).
//
// The existing GuardianDecision struct has the same shape but is
// tied to the LLM-based guardian. PermissionVerdict is more
// general: it can be returned by any subsystem and carries a Risk
// level (low/medium/high/blocked) and a Rule string identifying
// which check fired.
//
// GrayCode native implementation. hooks/permission.js
// (PermissionVerdict). Ported to native Go.
package permissions

import contracts "github.com/GrayCodeAI/graycode-cli/internal/contracts/policy"

// Risk is the severity of a permission verdict.
type Risk = contracts.Risk

const (
	RiskLow     = contracts.RiskLow
	RiskMedium  = contracts.RiskMedium
	RiskHigh    = contracts.RiskHigh
	RiskBlocked = contracts.RiskBlocked
)

// ParseRisk parses a risk name (case-insensitive) into a Risk value.
var ParseRisk = contracts.ParseRisk

// PermissionVerdict is the unified outcome type for any permission
// subsystem. It is constructed by helpers (Allow, Deny, RequireApproval)
// and consumed by the tool dispatcher.
type PermissionVerdict = contracts.PermissionVerdict

// Allow returns a permissive verdict.
var Allow = contracts.Allow

// Deny returns a reject verdict with the given reason and rule.
var Deny = contracts.Deny

// RequireApproval returns a "needs human approval" verdict. Allowed
// is false; the caller can use Confidence < 1.0 to indicate the
// request is plausible but uncertain.
var RequireApproval = contracts.RequireApproval
