package permissions

import (
	"testing"
	"time"
)

func TestBypassGrant_Covers(t *testing.T) {
	// Empty scope covers everything.
	g := &BypassGrant{Enabled: true}
	if !g.Covers("bash") || !g.Covers("network") {
		t.Fatal("empty scope should cover all")
	}
	// Scoped grant covers only listed categories.
	g2 := &BypassGrant{Enabled: true, Scope: []string{"bash"}}
	if !g2.Covers("bash") {
		t.Fatal("should cover bash")
	}
	if g2.Covers("network") {
		t.Fatal("should not cover network")
	}
}

func TestBypassGrant_IsExpired(t *testing.T) {
	now := time.Now()
	// Session-long (zero ExpiresAt) never expires.
	g := &BypassGrant{Enabled: true}
	if g.IsExpired(now) {
		t.Fatal("session-long should not expire")
	}
	// Future expiry is not expired.
	later := now.Add(time.Hour)
	g2 := &BypassGrant{Enabled: true, ExpiresAt: later}
	if g2.IsExpired(now) {
		t.Fatal("future expiry should not be expired")
	}
	// Past expiry is expired.
	past := now.Add(-time.Hour)
	g3 := &BypassGrant{Enabled: true, ExpiresAt: past}
	if !g3.IsExpired(now) {
		t.Fatal("past expiry should be expired")
	}
}

func TestBypassKillswitch_Scoped(t *testing.T) {
	b := NewBypassKillswitch()
	if b.IsEnabled() {
		t.Fatal("should start disabled")
	}
	b.EnableScoped([]string{"bash"}, time.Now().Add(time.Hour), "debugging")
	if !b.IsEnabled() {
		t.Fatal("should be enabled after EnableScoped")
	}
	g := b.Grant()
	if g == nil || !g.Covers("bash") || g.Covers("network") {
		t.Fatal("grant should be scoped to bash")
	}
	if g.Reason != "debugging" {
		t.Fatal("reason should be preserved")
	}
	b.Disable()
	if b.IsEnabled() {
		t.Fatal("should be disabled after Disable")
	}
}

func TestToolCategory(t *testing.T) {
	cases := map[string]string{
		"Bash":     "bash",
		"WebFetch": "network",
		"Write":    "filesystem",
		"Edit":     "filesystem",
		"Read":     "other",
		"glob":     "other",
	}
	for tool, want := range cases {
		if got := ToolCategory(tool); got != want {
			t.Fatalf("ToolCategory(%q) = %q, want %q", tool, got, want)
		}
	}
}
