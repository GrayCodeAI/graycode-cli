package sandbox

import (
	"strings"
	"testing"
)

func TestSanitizeEnvRemovesDisallowed(t *testing.T) {
	env := []string{
		"PATH=/tmp/malicious",
		"LD_PRELOAD=/tmp/evil.so",
		"PYTHONPATH=/tmp/py",
		"NODE_OPTIONS=--require=/tmp/evil.js",
		"GOROOT=/tmp/fake",
		"BENIGN=ok",
	}
	s := SanitizeEnv(env)
	for _, key := range []string{"PATH", "LD_PRELOAD", "PYTHONPATH", "NODE_OPTIONS", "GOROOT"} {
		for _, kv := range s.Env {
			if strings.HasPrefix(kv, key+"=") {
				t.Fatalf("disallowed key %q leaked into child env: %q", key, kv)
			}
		}
	}
	// Benign keys must survive.
	foundBenign := false
	for _, kv := range s.Env {
		if kv == "BENIGN=ok" {
			foundBenign = true
		}
	}
	if !foundBenign {
		t.Fatal("benign key must survive sanitization")
	}
	// Removed list must name the disallowed keys (no values).
	if len(s.Removed) != 5 {
		t.Fatalf("removed count = %d, want 5; got %v", len(s.Removed), s.Removed)
	}
	for _, key := range []string{"PATH", "LD_PRELOAD", "PYTHONPATH", "NODE_OPTIONS", "GOROOT"} {
		if !listHasStr(s.Removed, key) {
			t.Fatalf("removed list missing %q: %v", key, s.Removed)
		}
	}
}

func TestSanitizeEnvEmptyAndClean(t *testing.T) {
	if s := SanitizeEnv(nil); len(s.Env) != 0 || len(s.Removed) != 0 {
		t.Fatalf("nil env must yield empty result, got %+v", s)
	}
	s := SanitizeEnv([]string{"A=1", "B=2"})
	if len(s.Removed) != 0 || len(s.Env) != 2 {
		t.Fatalf("clean env must pass through, got %+v", s)
	}
}

func TestSanitizeEnvDedupKeepsLast(t *testing.T) {
	s := SanitizeEnv([]string{"KEEP=first", "KEEP=last"})
	count := 0
	for _, kv := range s.Env {
		if strings.HasPrefix(kv, "KEEP=") {
			count++
			if kv != "KEEP=last" {
				t.Fatalf("expected last value to win, got %q", kv)
			}
		}
	}
	if count != 1 {
		t.Fatalf("expected one KEEP entry, got %d", count)
	}
}

func listHasStr(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}
