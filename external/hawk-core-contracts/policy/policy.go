package policy

import "fmt"

// Risk is the severity of a permission or policy verdict.
type Risk int

const (
	RiskLow Risk = iota
	RiskMedium
	RiskHigh
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
		return RiskMedium, fmt.Errorf("policy: unknown risk %q", s)
	}
}

// PermissionVerdict is the unified outcome type for permission subsystems.
type PermissionVerdict struct {
	Allowed    bool    `json:"allowed"`
	Reason     string  `json:"reason,omitempty"`
	Rule       string  `json:"rule,omitempty"`
	Risk       Risk    `json:"risk"`
	Confidence float64 `json:"confidence,omitempty"`
	Source     string  `json:"source,omitempty"`
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

// RequireApproval returns a "needs human approval" verdict.
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

// IsZero reports whether v is the zero value.
func (v PermissionVerdict) IsZero() bool {
	return !v.Allowed && v.Reason == "" && v.Rule == "" &&
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

// GuardianDecision is a provider-neutral automatic permission review response.
type GuardianDecision struct {
	Allowed    bool    `json:"allowed"`
	Reason     string  `json:"reason"`
	Confidence float64 `json:"confidence"`
}

// PermissionRequest represents a user-facing approval request.
type PermissionRequest struct {
	ToolName string `json:"tool_name"`
	ToolID   string `json:"tool_id,omitempty"`
	Summary  string `json:"summary,omitempty"`
}

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
