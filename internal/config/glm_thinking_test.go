package config

import "testing"

// TestSettingValue_GLMThinking verifies the glmthinking setting key renders the
// three states (default/true/false) and accepts both the short and long key forms.
func TestSettingValue_GLMThinking(t *testing.T) {
	enabled := true
	disabled := false
	tests := []struct {
		name string
		s    Settings
		key  string
		want string
	}{
		{name: "nil renders default", s: Settings{}, key: "glmthinking", want: "default"},
		{name: "true renders true", s: Settings{GLMThinkingEnabled: &enabled}, key: "glmthinking", want: "true"},
		{name: "false renders false", s: Settings{GLMThinkingEnabled: &disabled}, key: "glmthinking", want: "false"},
		{name: "long key form", s: Settings{GLMThinkingEnabled: &enabled}, key: "glm_thinking_enabled", want: "true"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := SettingValue(tt.s, tt.key)
			if !ok {
				t.Fatalf("SettingValue(%q) returned ok=false", tt.key)
			}
			if got != tt.want {
				t.Errorf("SettingValue(%q) = %q, want %q", tt.key, got, tt.want)
			}
		})
	}
}
