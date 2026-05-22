// This file re-exports symbols from the git sub-package so that existing
// callers of engine.GitContext, engine.NewGitContext, etc. keep compiling
// during the Stage 2 migration. See REFACTOR_PLAN.md.
package engine

import "github.com/GrayCodeAI/hawk/internal/engine/git"

type (
	GitContext  = git.GitContext
	GitFileInfo = git.GitFileInfo
	CommitInfo  = git.CommitInfo
	BlameLine   = git.BlameLine
	GitProvider = git.GitProvider
	GitIssue    = git.GitIssue
	PullRequest = git.PullRequest
	CIStatus    = git.CIStatus
	CICheck     = git.CICheck
)

var (
	NewGitContext  = git.NewGitContext
	NewGitProvider = git.NewGitProvider
	FormatIssues   = git.FormatIssues
	FormatPRs      = git.FormatPRs
	FormatCIStatus = git.FormatCIStatus
	DetectProvider = git.DetectProvider
)
