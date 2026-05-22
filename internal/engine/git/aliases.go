// Package git provides git-context enrichment and remote-forge integration.
// See ../REFACTOR_PLAN.md.
package git

// Context wraps a local git repo and exposes file/commit/blame queries.
type Context = GitContext

// FileInfo summarises a tracked file's git metadata.
type FileInfo = GitFileInfo

// Provider talks to a remote forge (GitHub, GitLab, ...) for issues, PRs, CI.
type Provider = GitProvider

// Issue is a remote-forge issue record.
type Issue = GitIssue

// NewContext returns a git Context bound to the given working directory.
func NewContext(repoDir string) *Context {
	return NewGitContext(repoDir)
}

// NewProvider returns a forge provider client.
func NewProvider(providerType, token, owner, repo string) *Provider {
	return NewGitProvider(providerType, token, owner, repo)
}
