package safety

import "testing"

func TestToolPolicyForReturnsDefensiveCapabilities(t *testing.T) {
	policy := ToolPolicyFor("Write")
	if policy.Name != "Write" || policy.DefaultRisk != RiskMedium {
		t.Fatalf("unexpected Write policy: %#v", policy)
	}
	policy.Capabilities[0] = CapabilityUnknown
	if ToolPolicyFor("Write").Capabilities[0] != CapabilityFilesystemWrite {
		t.Fatal("ToolPolicyFor returned mutable registry state")
	}
}

func TestToolPolicyForUnknownFailsClosed(t *testing.T) {
	policy := ToolPolicyFor("plugin_future_tool")
	if policy.DefaultRisk != RiskHigh || len(policy.Capabilities) != 1 || policy.Capabilities[0] != CapabilityUnknown {
		t.Fatalf("unexpected unknown policy: %#v", policy)
	}
}

func TestToolPolicyForCanonicalAliases(t *testing.T) {
	if got := ToolPolicyFor("bash"); got.Name != "Bash" || got.Capabilities[0] != CapabilityProcessExecute {
		t.Fatalf("bash alias did not resolve: %#v", got)
	}
}
