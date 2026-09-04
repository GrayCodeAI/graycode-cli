package git

import (
	"testing"
)

func TestParseGHRepoView_Github(t *testing.T) {
	jsonStr := `{"owner":{"login":"GrayCodeAI"},"name":"graycode-cli","url":"https://github.com/GrayCodeAI/graycode-cli"}`
	owner, repo, provider := parseGHRepoView(jsonStr)
	if owner != "GrayCodeAI" {
		t.Errorf("owner = %q, want %q", owner, "GrayCodeAI")
	}
	if repo != "graycode-cli" {
		t.Errorf("repo = %q, want %q", repo, "graycode-cli")
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

func TestParseGitConfig_Github(t *testing.T) {
	content := `[core]
	repositoryformatversion = 0
	filemode = true
	bare = false
	logallrefupdates = true
	ignorecase = true
	precomposeunicode = true
[remote "origin"]
	url = git@github.com:GrayCodeAI/graycode-cli.git
	fetch = +refs/heads/*:refs/remotes/origin/*
[branch "main"]
	remote = origin
	merge = refs/heads/main`
	provider, owner, repo := parseGitConfig(content)
	if owner != "GrayCodeAI" {
		t.Errorf("owner = %q, want %q", owner, "GrayCodeAI")
	}
	if repo != "graycode-cli" {
		t.Errorf("repo = %q, want %q", repo, "graycode-cli")
	}
	if provider != "github" {
		t.Errorf("provider = %q, want %q", provider, "github")
	}
}

func TestParseGitConfig_Gitlab(t *testing.T) {
	content := `[remote "origin"]
	url = https://gitlab.com/gitlab-user/gitlab-repo.git`
	provider, owner, repo := parseGitConfig(content)
	if owner != "gitlab-user" {
		t.Errorf("owner = %q, want %q", owner, "gitlab-user")
	}
	if repo != "gitlab-repo" {
		t.Errorf("repo = %q, want %q", repo, "gitlab-repo")
	}
	if provider != "gitlab" {
		t.Errorf("provider = %q, want %q", provider, "gitlab")
	}
}

func TestParseGitConfig_Bitbucket(t *testing.T) {
	content := `[remote "origin"]
	url = https://bitbucket.org/bb-user/bb-repo.git`
	provider, owner, repo := parseGitConfig(content)
	if owner != "bb-user" {
		t.Errorf("owner = %q, want %q", owner, "bb-user")
	}
	if repo != "bb-repo" {
		t.Errorf("repo = %q, want %q", repo, "bb-repo")
	}
	if provider != "bitbucket" {
		t.Errorf("provider = %q, want %q", provider, "bitbucket")
	}
}
