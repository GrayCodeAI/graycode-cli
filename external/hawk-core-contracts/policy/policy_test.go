package policy_test

import (
	"strings"
	"testing"

	"github.com/GrayCodeAI/hawk-core-contracts/policy"
)

func TestParseRisk(t *testing.T) {
	tests := []struct {
		in   string
		want policy.Risk
	}{
		{in: "low", want: policy.RiskLow},
		{in: "MED", want: policy.RiskMedium},
		{in: "moderate", want: policy.RiskMedium},
		{in: "hi", want: policy.RiskHigh},
		{in: "forbidden", want: policy.RiskBlocked},
	}

	for _, tt := range tests {
		got, err := policy.ParseRisk(tt.in)
		if err != nil {
			t.Fatalf("ParseRisk(%q) unexpected error = %v", tt.in, err)
		}
		if got != tt.want {
			t.Fatalf("ParseRisk(%q) = %v, want %v", tt.in, got, tt.want)
		}
	}
}

func TestParseRiskUnknown(t *testing.T) {
	got, err := policy.ParseRisk("mystery")
	if err == nil {
		t.Fatal("ParseRisk() error = nil, want non-nil")
	}
	if got != policy.RiskMedium {
		t.Fatalf("ParseRisk() risk = %v, want %v", got, policy.RiskMedium)
	}
}

func TestPermissionVerdictHelpers(t *testing.T) {
	allow := policy.Allow("safe")
	if !allow.Allowed || allow.Risk != policy.RiskLow || allow.Source != "default" {
		t.Fatalf("Allow() = %#v", allow)
	}

	deny := policy.Deny("blocked", "rule.exec")
	if deny.Allowed || deny.Risk != policy.RiskBlocked || deny.Rule != "rule.exec" {
		t.Fatalf("Deny() = %#v", deny)
	}

	approval := policy.RequireApproval("needs review", "rule.write", policy.RiskHigh)
	if approval.Allowed || approval.Risk != policy.RiskHigh || approval.Source != "guardian" {
		t.Fatalf("RequireApproval() = %#v", approval)
	}
	if approval.IsZero() {
		t.Fatalf("RequireApproval() should not be zero: %#v", approval)
	}
	if !(policy.PermissionVerdict{}).IsZero() {
		t.Fatal("zero PermissionVerdict should report IsZero() = true")
	}
}

func TestPermissionVerdictString(t *testing.T) {
	got := policy.Deny("dangerous command", "rule.shell").String()
	if !strings.Contains(got, "DENY") {
		t.Fatalf("String() = %q, want DENY marker", got)
	}
	if !strings.Contains(got, "rule.shell") {
		t.Fatalf("String() = %q, want rule name", got)
	}
	if !strings.Contains(got, "risk=blocked") {
		t.Fatalf("String() = %q, want blocked risk", got)
	}
}
