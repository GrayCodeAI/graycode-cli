package permissions_test

import (
	"strings"
	"testing"

	"github.com/GrayCodeAI/graycode-cli/internal/permissions"
)

func TestRiskString(t *testing.T) {
	tests := []struct {
		r    permissions.Risk
		want string
	}{
		{permissions.RiskLow, "low"},
		{permissions.RiskMedium, "medium"},
		{permissions.RiskHigh, "high"},
		{permissions.RiskBlocked, "blocked"},
		{permissions.Risk(99), "Risk(99)"},
	}
	for _, tc := range tests {
		if got := tc.r.String(); got != tc.want {
			t.Errorf("Risk(%d).String() = %q, want %q", tc.r, got, tc.want)
		}
	}
}

func TestParseRisk(t *testing.T) {
	tests := []struct {
		in      string
		want    permissions.Risk
		wantErr bool
	}{
		{"low", permissions.RiskLow, false},
		{"LOW", permissions.RiskLow, false},
		{"Low", permissions.RiskLow, false},
		{"medium", permissions.RiskMedium, false},
		{"MED", permissions.RiskMedium, false},
		{"high", permissions.RiskHigh, false},
		{"hi", permissions.RiskHigh, false},
		{"blocked", permissions.RiskBlocked, false},
		{"deny", permissions.RiskBlocked, false},
		{"forbidden", permissions.RiskBlocked, false},
		{"unknown", permissions.RiskMedium, true},
		{"", permissions.RiskMedium, true},
	}
	for _, tc := range tests {
		t.Run(tc.in, func(t *testing.T) {
			got, err := permissions.ParseRisk(tc.in)
			if tc.wantErr {
				if err == nil {
					t.Errorf("expected error for %q", tc.in)
				}
				return
			}
			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Errorf("ParseRisk(%q) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}

func TestAllow(t *testing.T) {
	v := permissions.Allow("read-only operation")
	if !v.Allowed {
		t.Error("expected Allowed=true")
	}
	if v.Risk != permissions.RiskLow {
		t.Errorf("expected RiskLow, got %v", v.Risk)
	}
	if v.Confidence != 1.0 {
		t.Errorf("expected Confidence=1.0, got %f", v.Confidence)
	}
	if v.Reason != "read-only operation" {
		t.Errorf("unexpected Reason: %q", v.Reason)
	}
}

func TestDeny(t *testing.T) {
	v := permissions.Deny("rm -rf not allowed", "boundary:destructive")
	if v.Allowed {
		t.Error("expected Allowed=false")
	}
	if v.Rule != "boundary:destructive" {
		t.Errorf("unexpected Rule: %q", v.Rule)
	}
	if v.Risk != permissions.RiskBlocked {
		t.Errorf("expected RiskBlocked, got %v", v.Risk)
	}
}

func TestRequireApproval(t *testing.T) {
	v := permissions.RequireApproval("uncertain", "guardian:rm-rf", permissions.RiskHigh)
	if v.Allowed {
		t.Error("expected Allowed=false (requires approval)")
	}
	if v.Risk != permissions.RiskHigh {
		t.Errorf("expected RiskHigh, got %v", v.Risk)
	}
	if v.Confidence != 0.5 {
		t.Errorf("expected Confidence=0.5, got %f", v.Confidence)
	}
	if v.Source != "guardian" {
		t.Errorf("expected Source=guardian, got %q", v.Source)
	}
}

func TestIsZero(t *testing.T) {
	if !(permissions.PermissionVerdict{}).IsZero() {
		t.Error("zero verdict should be zero")
	}
	v := permissions.Allow("ok")
	if v.IsZero() {
		t.Error("non-empty verdict should not be zero")
	}
}

func TestVerdictString(t *testing.T) {
	v := permissions.Deny("rm -rf is destructive", "boundary:rm-rf")
	s := v.String()
	if !strings.Contains(s, "DENY") {
		t.Errorf("expected DENY in summary, got %q", s)
	}
	if !strings.Contains(s, "boundary:rm-rf") {
		t.Errorf("expected rule in summary, got %q", s)
	}
	if !strings.Contains(s, "destructive") {
		t.Errorf("expected reason in summary, got %q", s)
	}
}

func TestVerdictString_WithRule(t *testing.T) {
	v := permissions.RequireApproval("uncertain", "guardian:rm-rf", permissions.RiskHigh)
	s := v.String()
	if !strings.Contains(s, "guard") {
		t.Errorf("expected source in summary, got %q", s)
	}
	if !strings.Contains(s, "guard") {
		t.Errorf("expected source in summary, got %q", s)
	}
}

func TestVerdictString_Allow(t *testing.T) {
	v := permissions.Allow("read-only")
	s := v.String()
	if !strings.Contains(s, "ALLOW") {
		t.Errorf("expected ALLOW in summary, got %q", s)
	}
}
