package cmd

import "testing"

func TestGhostText_Suggest(t *testing.T) {
	g := NewGhostText()
	g.Suggest("I fixed the failing test in auth_test.go")
	got := g.Get()
	if got != "go test ./..." {
		t.Errorf("expected 'go test ./...', got %q", got)
	}
}

func TestGhostText_Accept(t *testing.T) {
	g := NewGhostText()
	g.SuggestExplicit("git push")
	s := g.Accept()
	if s != "git push" {
		t.Errorf("Accept() = %q, want 'git push'", s)
	}
	if g.Active() {
		t.Error("expected inactive after accept")
	}
}

func TestGhostText_Clear(t *testing.T) {
	g := NewGhostText()
	g.SuggestExplicit("ls")
	g.Clear()
	if g.Active() {
		t.Error("expected inactive after clear")
	}
	if g.Get() != "" {
		t.Error("expected empty after clear")
	}
}

func TestGhostText_NoMatch(t *testing.T) {
	g := NewGhostText()
	g.Suggest("Here is some general information about Go.")
	if g.Active() {
		t.Error("expected no suggestion for unmatched response")
	}
}
