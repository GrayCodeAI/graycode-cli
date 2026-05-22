package engine

import "github.com/GrayCodeAI/hawk/internal/engine/diff"

// Types from diff sub-package.

type PendingChange = diff.PendingChange
type DiffSandbox = diff.DiffSandbox
type StagingArea = diff.StagingArea
type StagedChange = diff.StagedChange
type StagedHunk = diff.StagedHunk
type DiffPreview = diff.DiffPreview
type FileChange = diff.FileChange
type DiffHunk = diff.DiffHunk
type DiffLine = diff.DiffLine
type ChangeStats = diff.ChangeStats
type DiffSummary = diff.DiffSummary
type FileSummary = diff.FileSummary
type DiffSummarizer = diff.DiffSummarizer
type TestSelector = diff.TestSelector
type SelectedTests = diff.SelectedTests
type Diff3Result = diff.Diff3Result
type Diff3Conflict = diff.Diff3Conflict
type Diff3Stats = diff.Diff3Stats
type Diff3Region = diff.Diff3Region
type Edit = diff.Edit

// Short-name aliases.

type Preview = diff.DiffPreview
type Summarizer = diff.DiffSummarizer

// Functions.

var NewDiffSandbox = diff.NewDiffSandbox
var NewStagingArea = diff.NewStagingArea
var NewDiffPreview = diff.NewDiffPreview
var NewDiffSummarizer = diff.NewDiffSummarizer
var NewTestSelector = diff.NewTestSelector
var ComputeDiff = diff.ComputeDiff
var ComputeMyersDiff = diff.ComputeMyersDiff
var RenderUnified = diff.RenderUnified
var Merge3 = diff.Merge3
var MergeClean = diff.MergeClean
var FormatConflictMarkers = diff.FormatConflictMarkers
var LCS = diff.LCS
var EditScript = diff.EditScript
var BuildDependencyGraph = diff.BuildDependencyGraph
var GenerateTestCommand = diff.GenerateTestCommand
