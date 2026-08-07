package permissions

import "testing"

func TestAutoMode_SemanticMatch(t *testing.T) {
	a := NewAutoModeState()

	// User allowed "go test *" — should match "go test ./foo"
	a.allowList["Bash:go test *"] = true

	tests := []struct {
		cmd       string
		wantOK    bool
		wantAllow bool
	}{
		{"go test ./foo", true, true},    // prefix match
		{"go test ./...", true, true},    // prefix match
		{"go test -v ./pkg", true, true}, // prefix match
		{"go build ./foo", false, false}, // different command
		{"npm test", false, false},       // not learned
		{"git status", false, false},     // not learned
	}

	for _, tt := range tests {
		gotOK, gotAllow := a.ShouldAutoAllow("Bash", tt.cmd)
		if gotOK != tt.wantOK || gotAllow != tt.wantAllow {
			t.Errorf("ShouldAutoAllow(Bash, %q) = (%v, %v), want (%v, %v)",
				tt.cmd, gotOK, gotAllow, tt.wantOK, tt.wantAllow)
		}
	}
}

func TestAutoMode_DenyBeatsAllow_Semantic(t *testing.T) {
	a := NewAutoModeState()

	// Allow all "go *" but deny "go test ./secret"
	a.allowList["Bash:go *"] = true
	a.denyList["Bash:go test ./secret"] = true

	// "go test ./secret" should be denied (deny beats allow).
	// Return semantics: (allowed, found). Deny = (false, true).
	gotAllow, gotFound := a.ShouldAutoAllow("Bash", "go test ./secret")
	if !gotFound || gotAllow {
		t.Errorf("expected deny for 'go test ./secret', got (allowed=%v, found=%v)", gotAllow, gotFound)
	}

	// "go test ./foo" should be allowed. Allow = (true, true).
	gotAllow, gotFound = a.ShouldAutoAllow("Bash", "go test ./foo")
	if !gotFound || !gotAllow {
		t.Errorf("expected allow for 'go test ./foo', got (allowed=%v, found=%v)", gotAllow, gotFound)
	}
}

func TestCommandPrefix(t *testing.T) {
	cases := map[string]string{
		"go test ./foo":     "go test",
		"go build ./...":    "go build",
		"npm install foo":   "npm install",
		"git status":        "git status",
		"ls -la":            "ls",
		"pytest -xvs":       "pytest",
		"docker build -t":   "docker build",
		"cargo test":        "cargo test",
		"bundle exec rspec": "bundle exec",
		"":                  "",
	}
	for input, want := range cases {
		if got := commandPrefix(input); got != want {
			t.Errorf("commandPrefix(%q) = %q, want %q", input, got, want)
		}
	}
}
