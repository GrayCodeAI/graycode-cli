package cmd

import (
	"strings"
	"testing"
)

func TestGenerateIssueTitle(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"", "Untitled report"},
		{"panic: nil pointer dereference\n\ngoroutine 1", "panic: nil pointer dereference"},
		{"# Crash on startup\n\nDetails here", "Crash on startup"},
		{"   \n\n\n", "Untitled report"},
		{" - flaky test in parser", "flaky test in parser"},
	}
	for _, c := range cases {
		if got := generateIssueTitle(c.in); got != c.want {
			t.Errorf("generateIssueTitle(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestGenerateIssueBody(t *testing.T) {
	if got := generateIssueBody(""); !strings.Contains(got, "No additional context") {
		t.Errorf("empty context body = %q, want 'No additional context'", got)
	}
	body := generateIssueBody("stack\nline 2")
	if !strings.Contains(body, "```") || !strings.Contains(body, "stack\nline 2") {
		t.Errorf("context body = %q, want fenced original text", body)
	}
}
