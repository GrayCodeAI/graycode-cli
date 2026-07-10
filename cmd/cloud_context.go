package cmd

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

type gitContext struct {
	Repository string
	Provider   string
	Branch     string
	Commit     string
}

var runCloudGit = func(ctx context.Context, args ...string) (string, error) {
	out, err := exec.CommandContext(ctx, "git", args...).Output() // #nosec G204 -- fixed git binary and fixed args
	return strings.TrimSpace(string(out)), err
}

func detectGitContext(ctx context.Context) (gitContext, error) {
	remote, err := runCloudGit(ctx, "config", "--get", "remote.origin.url")
	if err != nil || remote == "" {
		return gitContext{}, fmt.Errorf("could not detect the git origin; pass --repository")
	}
	branch, _ := runCloudGit(ctx, "branch", "--show-current")
	commit, _ := runCloudGit(ctx, "rev-parse", "HEAD")
	return gitContext{Repository: repositoryName(remote), Provider: repositoryProvider(remote), Branch: branch, Commit: commit}, nil
}

func repositoryName(remote string) string {
	value := strings.TrimSuffix(strings.TrimSpace(remote), ".git")
	if index := strings.LastIndex(value, ":"); index >= 0 && !strings.Contains(value[index+1:], "/") {
		return value[index+1:]
	}
	if index := strings.Index(value, ":"); index >= 0 && strings.Contains(value[index+1:], "/") && !strings.Contains(value, "://") {
		return value[index+1:]
	}
	parts := strings.Split(value, "/")
	if len(parts) >= 2 {
		return strings.Join(parts[len(parts)-2:], "/")
	}
	return value
}

func repositoryProvider(remote string) string {
	value := strings.ToLower(remote)
	switch {
	case strings.Contains(value, "github.com"):
		return "github"
	case strings.Contains(value, "gitlab.com"):
		return "gitlab"
	case strings.Contains(value, "bitbucket.org"):
		return "bitbucket"
	default:
		return "git"
	}
}
