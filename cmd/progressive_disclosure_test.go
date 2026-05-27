package cmd

import (
	"strings"
	"testing"
)

func TestParseUserLevel(t *testing.T) {
	t.Parallel()
	tests := []struct {
		input string
		want  UserLevel
		ok    bool
	}{
		{"beginner", LevelBeginner, true},
		{"new", LevelBeginner, true},
		{"novice", LevelBeginner, true},
		{"intermediate", LevelIntermediate, true},
		{"standard", LevelIntermediate, true},
		{"advanced", LevelAdvanced, true},
		{"expert", LevelAdvanced, true},
		{"unknown", LevelIntermediate, false},
		{"", LevelIntermediate, false},
	}

	for _, tt := range tests {
		level, ok := ParseUserLevel(tt.input)
		if ok != tt.ok {
			t.Errorf("ParseUserLevel(%q) ok = %v, want %v", tt.input, ok, tt.ok)
		}
		if ok && level != tt.want {
			t.Errorf("ParseUserLevel(%q) = %v, want %v", tt.input, level, tt.want)
		}
	}
}

func TestUserLevel_String(t *testing.T) {
	t.Parallel()
	if LevelBeginner.String() != "beginner" {
		t.Errorf("expected 'beginner', got %q", LevelBeginner.String())
	}
	if LevelAdvanced.String() != "advanced" {
		t.Errorf("expected 'advanced', got %q", LevelAdvanced.String())
	}
}

func TestBeginnerHelp(t *testing.T) {
	t.Parallel()
	help := BeginnerHelp()
	if help == "" {
		t.Error("beginner help should not be empty")
	}
	if !strings.Contains(help, "Getting Started") {
		t.Error("beginner help should have intro")
	}
	if !strings.Contains(help, "/help") {
		t.Error("beginner help should mention /help")
	}
}

func TestIntermediateHelp(t *testing.T) {
	t.Parallel()
	help := IntermediateHelp()
	if help == "" {
		t.Error("intermediate help should not be empty")
	}
	if !strings.Contains(help, "/test") {
		t.Error("intermediate help should mention /test")
	}
}

func TestAdvancedHelp(t *testing.T) {
	t.Parallel()
	help := AdvancedHelp()
	if help == "" {
		t.Error("advanced help should not be empty")
	}
	// Advanced should have more commands than beginner
	beginner := BeginnerHelp()
	if len(help) <= len(beginner) {
		t.Error("advanced help should be longer than beginner help")
	}
	if !strings.Contains(help, "/research") {
		t.Error("advanced help should mention /research")
	}
	if !strings.Contains(help, "/council") {
		t.Error("advanced help should mention /council")
	}
}

func TestGetHelpForLevel(t *testing.T) {
	t.Parallel()
	// Each level should return different content
	beginner := GetHelpForLevel(LevelBeginner)
	intermediate := GetHelpForLevel(LevelIntermediate)
	advanced := GetHelpForLevel(LevelAdvanced)

	if beginner == intermediate {
		t.Error("beginner and intermediate help should differ")
	}
	if intermediate == advanced {
		t.Error("intermediate and advanced help should differ")
	}
}

func TestDefaultDisclosureConfig(t *testing.T) {
	t.Parallel()
	cfg := DefaultDisclosureConfig()
	if cfg.Level != LevelIntermediate {
		t.Errorf("default level should be intermediate, got %v", cfg.Level)
	}
}
