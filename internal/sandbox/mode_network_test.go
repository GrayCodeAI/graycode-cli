package sandbox

import (
	"os"
	"testing"
)

func TestModeAllowsNetwork_Defaults(t *testing.T) {
	os.Unsetenv("GRAYCODE_SANDBOX_NETWORK")
	if ModeAllowsNetwork(ModeStrict) {
		t.Error("strict mode should deny network by default")
	}
	if !ModeAllowsNetwork(ModeWorkspace) {
		t.Error("workspace mode should allow network by default")
	}
	if !ModeAllowsNetwork(ModeOff) {
		t.Error("off mode should allow network by default")
	}
}

func TestModeAllowsNetwork_EnvOverride(t *testing.T) {
	t.Setenv("GRAYCODE_SANDBOX_NETWORK", "0")
	if ModeAllowsNetwork(ModeWorkspace) {
		t.Error("GRAYCODE_SANDBOX_NETWORK=0 should deny network even in workspace mode")
	}
	t.Setenv("GRAYCODE_SANDBOX_NETWORK", "1")
	if !ModeAllowsNetwork(ModeStrict) {
		t.Error("GRAYCODE_SANDBOX_NETWORK=1 should allow network even in strict mode")
	}
	t.Setenv("GRAYCODE_SANDBOX_NETWORK", "garbage")
	if ModeAllowsNetwork(ModeStrict) {
		t.Error("unrecognized value should fall back to per-mode default (strict denies)")
	}
}
