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
// Source: github.com/JuliusBrussee/caveman, hooks/permission.js
// (PermissionVerdict). Ported to native Go.
package permissions

import "fmt"

// Risk is the severity of a permission verdict.
type Risk int

const (
	// RiskLow: read-only, side-effect-free operations.
	RiskLow Risk = iota
	// RiskMedium: local writes, network reads, subprocess spawns.
	RiskMedium
	// RiskHigh: network writes, credential access, system changes.
	RiskHigh
	// RiskBlocked: definitely should not run; security or
	// compliance violation.
	RiskBlocked
)

// String returns a human-readable risk name.
func (r Risk) String() string {
	switch r {
	case RiskLow:
		return "low"
	case RiskMedium:
		return "medium"
	case RiskHigh:
		return "high"
	case RiskBlocked:
		return "blocked"
	default:
		return fmt.Sprintf("Risk(%d)", int(r))
	}
}

// ParseRisk parses a risk name (case-insensitive) into a Risk value.
// Returns RiskMedium and a non-nil error if the name is unknown.
func ParseRisk(s string) (Risk, error) {
	switch lower(s) {
	case "low":
		return RiskLow, nil
	case "medium", "med", "moderate":
		return RiskMedium, nil
	case "high", "hi":
		return RiskHigh, nil
	case "blocked", "block", "deny", "denied", "forbidden":
		return RiskBlocked, nil
	default:
		return RiskMedium, fmt.Errorf("permissions: unknown risk %q", s)
	}
}

// PermissionVerdict is the unified outcome type for any permission
// subsystem. It is constructed by helpers (Allow, Deny, RequireApproval)
// and consumed by the tool dispatcher.
type PermissionVerdict struct {
	// Allowed is the final accept/reject decision.
	Allowed bool
	// Reason is a human-readable explanation.
	Reason string
	// Rule is the identifier of the rule or check that fired
	// (e.g. "boundary:write-outside-workspace", "guardian:rm-rf").
	// Empty when no specific rule matched.
	Rule string
	// Risk is the severity assessment. Even if Allowed=true, a
	// non-low risk should be surfaced in logs/UI.
	Risk Risk
	// Confidence is 0.0-1.0; only meaningful for LLM-derived
	// verdicts. Static rule verdicts should set this to 1.0.
	Confidence float64
	// Source identifies the subsystem that produced the verdict
	// (e.g. "rules", "guardian", "boundary", "hook", "default").
	Source string
}

// Allow returns a permissive verdict.
func Allow(reason string) PermissionVerdict {
	return PermissionVerdict{
		Allowed:    true,
		Reason:     reason,
		Risk:       RiskLow,
		Confidence: 1.0,
		Source:     "default",
	}
}

// Deny returns a reject verdict with the given reason and rule.
func Deny(reason, rule string) PermissionVerdict {
	return PermissionVerdict{
		Allowed:    false,
		Reason:     reason,
		Rule:       rule,
		Risk:       RiskBlocked,
		Confidence: 1.0,
		Source:     "rules",
	}
}

// RequireApproval returns a "needs human approval" verdict. Allowed
// is false; the caller can use Confidence < 1.0 to indicate the
// request is plausible but uncertain.
func RequireApproval(reason, rule string, risk Risk) PermissionVerdict {
	return PermissionVerdict{
		Allowed:    false,
		Reason:     reason,
		Rule:       rule,
		Risk:       risk,
		Confidence: 0.5,
		Source:     "guardian",
	}
}

// IsZero reports whether v is the zero value. Useful for
// detecting "no verdict produced" cases.
func (v PermissionVerdict) IsZero() bool {
	return v.Allowed == false && v.Reason == "" && v.Rule == "" &&
		v.Risk == 0 && v.Confidence == 0 && v.Source == ""
}

// String returns a one-line summary for logs.
func (v PermissionVerdict) String() string {
	action := "DENY"
	if v.Allowed {
		action = "ALLOW"
	}
	if v.Rule != "" {
		return fmt.Sprintf("[%s] %s (%s, risk=%s, conf=%.2f): %s",
			v.Source, action, v.Rule, v.Risk, v.Confidence, v.Reason)
	}
	return fmt.Sprintf("[%s] %s (risk=%s, conf=%.2f): %s",
		v.Source, action, v.Risk, v.Confidence, v.Reason)
}

// lower is a small ASCII-only strings.ToLower shim to keep the
// package's import list tight.
func lower(s string) string {
	b := make([]byte, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		b[i] = c
	}
	return string(b)
}
