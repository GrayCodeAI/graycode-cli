package tool

import (
	"strings"
	"testing"
)

func TestGitHubArgsReadOnlyActions(t *testing.T) {
	args, err := githubArgs("pr_view", "42", 20)
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "pr view 42") || !strings.Contains(joined, "--json") {
		t.Fatalf("args = %v", args)
	}
}

func TestGitHubArgsRejectsMutatingActions(t *testing.T) {
	for _, action := range []string{"pr_create", "pr_merge", "issue_comment", "push"} {
		if _, err := githubArgs(action, "", 20); err == nil {
			t.Fatalf("action %q unexpectedly accepted", action)
		}
	}
}
