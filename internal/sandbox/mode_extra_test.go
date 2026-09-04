package sandbox

import (
	"context"
	"testing"
)

func TestParseMode(t *testing.T) {
	tests := []struct {
		input    string
		expected Mode
	}{
		{"strict", ModeStrict},
		{"workspace", ModeWorkspace},
		{"off", ModeOff},
		{"", ModeStrict},
		{"unknown", ModeStrict},
	}
	for _, tt := range tests {
		result := ParseMode(tt.input)
		if result != tt.expected {
			t.Errorf("ParseMode(%q) = %v, want %v", tt.input, result, tt.expected)
		}
	}
}

func TestContextWithMode(t *testing.T) {
	ctx := context.Background()
	ctx = ContextWithMode(ctx, ModeWorkspace)
	mode := ModeFromContext(ctx)
	if mode != ModeWorkspace {
		t.Errorf("ModeFromContext = %v, want %v", mode, ModeWorkspace)
	}
}

func TestModeFromContext_NoMode(t *testing.T) {
	ctx := context.Background()
	mode := ModeFromContext(ctx)
	if mode != ModeOff {
		t.Errorf("ModeFromContext with no mode = %v, want %v", mode, ModeOff)
	}
}

func TestModeFromContext_Strict(t *testing.T) {
	ctx := context.Background()
	ctx = ContextWithMode(ctx, ModeStrict)
	mode := ModeFromContext(ctx)
	if mode != ModeStrict {
		t.Errorf("ModeFromContext = %v, want %v", mode, ModeStrict)
	}
}

func TestThreatLevel_String(t *testing.T) {
	tests := []struct {
		level    ThreatLevel
		expected string
	}{
		{ThreatLow, "LOW"},
		{ThreatMedium, "MEDIUM"},
		{ThreatHigh, "HIGH"},
		{ThreatCritical, "CRITICAL"},
	}
	for _, tt := range tests {
		result := tt.level.String()
		if result != tt.expected {
			t.Errorf("ThreatLevel(%d).String() = %q, want %q", tt.level, result, tt.expected)
		}
	}
}

func TestModeAllowsNetwork(t *testing.T) {
	// Set sandbox network to default behavior (empty env var)
	t.Setenv("GRAYCODE_SANDBOX_NETWORK", "")
	if !ModeAllowsNetwork(ModeWorkspace) {
		t.Error("expected workspace mode to allow network")
	}
	if ModeAllowsNetwork(ModeStrict) {
		t.Error("expected strict mode to NOT allow network")
	}
	if !ModeAllowsNetwork(ModeOff) {
		t.Error("expected off mode to allow network (default)")
	}
}
