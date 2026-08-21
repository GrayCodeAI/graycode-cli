package trace

import (
	"testing"
)

func TestMaskSecrets(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name, in, want string
	}{
		{"bearer token", "Authorization: Bearer abc123XYZ._-def456  !", "Authorization: Bearer *****  !"},
		{"sk- prefixed", "key sk-ant-api03-e1f3abcdefGHI", "key *****"},
		{"github token", "token ghp_abcdef12345678", "token *****"},
		{"akid", "AKIAIOSFODNN7EXAMPLE", "*****"},
		{"json pair", `{"api_key": "super-secret-value"}`, `{"api_key": "*****"}`},
		{"assignment", "API_SECRET=abcdef12345678", "API_SECRET=*****"},
		{"long hex", "sha 9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08", "sha *****"},
		{"plain text untouched", "git status --short", "git status --short"},
		{"short run untouched", "key abc", "key abc"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := maskSecrets(c.in)
			if got != c.want {
				t.Errorf("maskSecrets(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

func TestStripANSI(t *testing.T) {
	t.Parallel()
	cases := []struct{ in, want string }{
		{"\x1b[31mred\x1b[0m", "red"},
		{"\x1b[1;3mBold Italic\x1b[0m", "Bold Italic"},
		{"plain", "plain"},
		{"\x1b]0;title\x07visible", "visible"},
		{"\x1b[38;2;255;0;0mcolored", "colored"},
	}
	for _, c := range cases {
		got := stripANSI(c.in)
		if got != c.want {
			t.Errorf("stripANSI(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestMaskSecretsIdempotent(t *testing.T) {
	t.Parallel()
	in := "token ghp_abcdef12345678 and sk-ant-api03-e1f3abc"
	once := maskSecrets(in)
	twice := maskSecrets(once)
	if once != twice {
		t.Errorf("maskSecrets not idempotent: %q -> %q -> %q", in, once, twice)
	}
}
