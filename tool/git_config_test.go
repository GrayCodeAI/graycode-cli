package tool

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseGitConfig_Basic(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config")
	content := `[core]
	repositoryformatversion = 0
	filemode = true
	bare = false
[remote "origin"]
	url = https://github.com/example/repo.git
	fetch = +refs/heads/*:refs/remotes/origin/*
[branch "main"]
	remote = origin
	merge = refs/heads/main
# This is a comment
; This is also a comment
`
	os.WriteFile(configPath, []byte(content), 0o644)

	config, err := ParseGitConfig(configPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if config["core"]["bare"] != "false" {
		t.Fatalf("expected core.bare = false, got %q", config["core"]["bare"])
	}
	if config["remote.origin"]["url"] != "https://github.com/example/repo.git" {
		t.Fatalf("expected remote.origin.url, got %q", config["remote.origin"]["url"])
	}
	if config["branch.main"]["remote"] != "origin" {
		t.Fatalf("expected branch.main.remote = origin, got %q", config["branch.main"]["remote"])
	}
}

func TestParseGitConfig_Comments(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config")
	content := `# Global comment
[user]
	name = Test User
	email = test@example.com # inline comment
`
	os.WriteFile(configPath, []byte(content), 0o644)

	config, err := ParseGitConfig(configPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if config["user"]["name"] != "Test User" {
		t.Fatalf("expected name = Test User, got %q", config["user"]["name"])
	}
	if config["user"]["email"] != "test@example.com" {
		t.Fatalf("expected email without comment, got %q", config["user"]["email"])
	}
}

func TestParseGitConfig_NonExistent(t *testing.T) {
	_, err := ParseGitConfig("/nonexistent/path/config")
	if err == nil {
		t.Fatal("expected error for non-existent file")
	}
}

func TestParseGitConfig_QuotedValues(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config")
	content := `[core]
	autocrlf = "input"
	editor = "vim"
`
	os.WriteFile(configPath, []byte(content), 0o644)

	config, err := ParseGitConfig(configPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if config["core"]["autocrlf"] != "input" {
		t.Fatalf("expected input without quotes, got %q", config["core"]["autocrlf"])
	}
}
