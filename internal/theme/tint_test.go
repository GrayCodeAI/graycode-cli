package theme

import (
	"strings"
	"testing"
)

func TestColorEnabled_RespectsEnv(t *testing.T) {
	t.Run("NO_COLOR disables", func(t *testing.T) {
		t.Setenv("NO_COLOR", "1")
		t.Setenv("FORCE_COLOR", "")
		if ColorEnabled() {
			t.Fatal("ColorEnabled() = true with NO_COLOR set")
		}
	})
	t.Run("FORCE_COLOR enables despite non-TTY", func(t *testing.T) {
		t.Setenv("NO_COLOR", "")
		t.Setenv("FORCE_COLOR", "1")
		if !ColorEnabled() {
			t.Fatal("ColorEnabled() = false with FORCE_COLOR set")
		}
	})
	t.Run("NO_COLOR wins over FORCE_COLOR", func(t *testing.T) {
		t.Setenv("NO_COLOR", "1")
		t.Setenv("FORCE_COLOR", "1")
		if ColorEnabled() {
			t.Fatal("ColorEnabled() = true when both NO_COLOR and FORCE_COLOR set")
		}
	})
}

func TestTint(t *testing.T) {
	t.Run("plain when disabled", func(t *testing.T) {
		t.Setenv("NO_COLOR", "1")
		if got := Tint("hello", ReportSuccess); got != "hello" {
			t.Fatalf("Tint = %q, want %q", got, "hello")
		}
	})
	t.Run("empty stays empty", func(t *testing.T) {
		t.Setenv("FORCE_COLOR", "1")
		if got := Tint("", ReportSuccess); got != "" {
			t.Fatalf("Tint(\"\") = %q, want empty", got)
		}
	})
	t.Run("wraps in ANSI when enabled", func(t *testing.T) {
		t.Setenv("NO_COLOR", "")
		t.Setenv("FORCE_COLOR", "1")
		got := Tint("ready", ReportSuccess)
		if !strings.Contains(got, "\x1b[") || !strings.Contains(got, "ready") {
			t.Fatalf("Tint = %q, want ANSI-wrapped text", got)
		}
	})
}
