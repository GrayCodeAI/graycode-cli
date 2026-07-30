package config

import "testing"

func TestThinkingPrefForModel(t *testing.T) {
	t.Parallel()
	enabled := true
	s := Settings{ModelThinking: map[string]bool{
		"longcat/LongCat-2.0": true,
		"agnes-2.5-flash":     false,
	}}
	if pref := ThinkingPrefForModel(s, "longcat/LongCat-2.0"); pref == nil || !*pref {
		t.Fatalf("prefixed id pref = %v", pref)
	}
	if pref := ThinkingPrefForModel(s, "LongCat-2.0"); pref == nil || !*pref {
		t.Fatalf("bare id should resolve via alternate key, pref = %v", pref)
	}
	if pref := ThinkingPrefForModel(s, "agnes-2.5-flash"); pref == nil || *pref {
		t.Fatalf("agnes pref = %v, want false", pref)
	}
	if pref := ThinkingPrefForModel(s, "missing"); pref != nil {
		t.Fatalf("missing pref = %v, want nil", pref)
	}
	_ = enabled
}

func TestResolveThinkingForModel(t *testing.T) {
	t.Parallel()
	globalOn := true
	s := Settings{
		GLMThinkingEnabled: &globalOn,
		ModelThinking:      map[string]bool{"m1": false},
	}
	if pref := ResolveThinkingForModel(s, "m1", "longcat"); pref == nil || *pref {
		t.Fatalf("explicit map wins: %v", pref)
	}
	if pref := ResolveThinkingForModel(s, "other", "longcat"); pref == nil || *pref {
		t.Fatalf("longcat unset defaults off: %v", pref)
	}
	if pref := ResolveThinkingForModel(s, "other", "openai"); pref == nil || !*pref {
		t.Fatalf("non-longcat falls back to global: %v", pref)
	}
	s2 := Settings{}
	if pref := ResolveThinkingForModel(s2, "x", "agnes"); pref != nil {
		t.Fatalf("agnes unset should stay nil (provider default), got %v", pref)
	}
	if pref := ResolveGLMThinkingForModel(s, "other", "openai"); pref == nil || !*pref {
		t.Fatalf("deprecated alias: %v", pref)
	}
}

func TestFormatModelThinkingLabel(t *testing.T) {
	t.Parallel()
	on := true
	off := false
	if got := FormatModelThinkingLabel(false, &on, "longcat"); got != "—" {
		t.Fatalf("no capability = %q", got)
	}
	if got := FormatModelThinkingLabel(true, &on, "longcat"); got != "on" {
		t.Fatalf("on = %q", got)
	}
	if got := FormatModelThinkingLabel(true, &off, "longcat"); got != "off" {
		t.Fatalf("off = %q", got)
	}
	if got := FormatModelThinkingLabel(true, nil, "longcat"); got != "off" {
		t.Fatalf("unset longcat = %q", got)
	}
}

func TestModelCapabilitySupportsThinking(t *testing.T) {
	t.Parallel()
	if !ModelCapabilitySupportsThinking([]string{"tools", "reasoning"}) {
		t.Fatal("expected reasoning support")
	}
	if ModelCapabilitySupportsThinking([]string{"tools", "vision"}) {
		t.Fatal("did not expect thinking support")
	}
}
