package cmd

import (
	"context"
	"fmt"
	"testing"
)

func TestDetectGitContext(t *testing.T) {
	original := runCloudGit
	t.Cleanup(func() { runCloudGit = original })
	runCloudGit = func(_ context.Context, args ...string) (string, error) {
		switch args[0] {
		case "config":
			return "git@github.com:GrayCodeAI/graycode-cli.git", nil
		case "branch":
			return "main", nil
		case "rev-parse":
			return "abc123", nil
		default:
			return "", fmt.Errorf("unexpected git command %v", args)
		}
	}
	got, err := detectGitContext(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got.Repository != "GrayCodeAI/graycode-cli" || got.Provider != "github" || got.Branch != "main" || got.Commit != "abc123" {
		t.Fatalf("context = %+v", got)
	}
}

func TestRepositoryNameHandlesHTTPSRemote(t *testing.T) {
	if got := repositoryName("https://gitlab.com/acme/platform.git"); got != "acme/platform" {
		t.Fatalf("repository = %q", got)
	}
}

func TestFirstValue(t *testing.T) {
	if got := firstValue("", "workflow", "later"); got != "workflow" {
		t.Fatalf("value = %q", got)
	}
}
