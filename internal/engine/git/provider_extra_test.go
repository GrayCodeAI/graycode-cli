package git

import (
	"testing"
)

func TestParseGHRepoView_Github(t *testing.T) {
	jsonStr := `{"owner":{"login":"GrayCodeAI"},"name":"hawk","url":"https://github.com/GrayCodeAI/hawk"}`
	owner, repo, provider := parseGHRepoView(jsonStr)
	if owner != "GrayCodeAI" {
		t.Errorf("owner = %q, want %q", owner, "GrayCodeAI")
	}
	if repo != "hawk" {
		t.Errorf("repo = %q, want %q", repo, "hawk")
	}
	if provider != "github" {
		t.Errorf("provider = %q, want %q", provider, "github")
	}
}

func TestParseGHRepoView_Gitlab(t *testing.T) {
	jsonStr := `{"owner":{"login":"gitlab-user"},"name":"gitlab-repo","url":"https://gitlab.com/gitlab-user/gitlab-repo"}`
	_, _, provider := parseGHRepoView(jsonStr)
	if provider != "gitlab" {
		t.Errorf("provider = %q, want %q", provider, "gitlab")
	}
}

func TestParseGHRepoView_Bitbucket(t *testing.T) {
	jsonStr := `{"owner":{"login":"bb-user"},"name":"bb-repo","url":"https://bitbucket.org/bb-user/bb-repo"}`
	_, _, provider := parseGHRepoView(jsonStr)
	if provider != "bitbucket" {
		t.Errorf("provider = %q, want %q", provider, "bitbucket")
	}
}

func TestDetectProvider_NonExistent(t *testing.T) {
	provider, owner, repo := DetectProvider("/nonexistent/directory")
	if provider != "" || owner != "" || repo != "" {
		t.Errorf("expected empty results for non-existent dir, got provider=%q owner=%q repo=%q", provider, owner, repo)
	}
}
