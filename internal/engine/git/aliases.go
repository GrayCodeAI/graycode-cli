// Package git is the Stage-1 namespace for git-related types and functions
// in package engine. See ../REFACTOR_PLAN.md.
package git

import "github.com/GrayCodeAI/hawk/engine"

// Context wraps a local git repo and exposes file/commit/blame queries.
type Context = engine.GitContext

// FileInfo summarises a tracked file's git metadata.
type FileInfo = engine.GitFileInfo

// CommitInfo describes a single commit (SHA, author, message, etc.).
type CommitInfo = engine.CommitInfo

// BlameLine is one line of git blame output.
type BlameLine = engine.BlameLine

// Provider talks to a remote forge (GitHub, GitLab, ...) for issues, PRs, CI.
type Provider = engine.GitProvider

// Issue is a remote-forge issue record.
type Issue = engine.GitIssue

// PullRequest is a remote-forge PR record.
type PullRequest = engine.PullRequest

// CIStatus is the aggregated CI state for a PR or commit.
type CIStatus = engine.CIStatus

// CICheck is a single check within a CIStatus.
type CICheck = engine.CICheck

// NewContext returns a git Context bound to the given working directory.
func NewContext(repoDir string) *Context {
	return engine.NewGitContext(repoDir)
}

// NewProvider returns a forge provider client.
func NewProvider(providerType, token, owner, repo string) *Provider {
	return engine.NewGitProvider(providerType, token, owner, repo)
}

// FormatIssues renders a slice of issues for terminal display.
func FormatIssues(issues []Issue) string {
	return engine.FormatIssues(issues)
}

// FormatPRs renders a slice of pull requests for terminal display.
func FormatPRs(prs []PullRequest) string {
	return engine.FormatPRs(prs)
}
