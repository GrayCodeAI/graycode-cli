// This file re-exports symbols from the git sub-package so that existing
// callers of engine.GitContext, engine.NewGitContext, etc. keep compiling
// during the Stage 2 migration. See REFACTOR_PLAN.md.
package engine

import "github.com/GrayCodeAI/hawk/internal/engine/git"

type GitContext = git.GitContext
type GitFileInfo = git.GitFileInfo
type CommitInfo = git.CommitInfo
type BlameLine = git.BlameLine
type GitProvider = git.GitProvider
type GitIssue = git.GitIssue
type PullRequest = git.PullRequest
type CIStatus = git.CIStatus
type CICheck = git.CICheck

var NewGitContext = git.NewGitContext
var NewGitProvider = git.NewGitProvider
var FormatIssues = git.FormatIssues
var FormatPRs = git.FormatPRs
var FormatCIStatus = git.FormatCIStatus
var DetectProvider = git.DetectProvider
