package engine

import "github.com/GrayCodeAI/hawk/internal/engine/diff"

// Types from diff sub-package.

type (
	PendingChange  = diff.PendingChange
	DiffSandbox    = diff.DiffSandbox
	StagingArea    = diff.StagingArea
	StagedChange   = diff.StagedChange
	StagedHunk     = diff.StagedHunk
	DiffPreview    = diff.DiffPreview
	FileChange     = diff.FileChange
	DiffHunk       = diff.DiffHunk
	DiffLine       = diff.DiffLine
	ChangeStats    = diff.ChangeStats
	DiffSummary    = diff.DiffSummary
	FileSummary    = diff.FileSummary
	DiffSummarizer = diff.DiffSummarizer
	TestSelector   = diff.TestSelector
	SelectedTests  = diff.SelectedTests
	Diff3Result    = diff.Diff3Result
	Diff3Conflict  = diff.Diff3Conflict
	Diff3Stats     = diff.Diff3Stats
	Diff3Region    = diff.Diff3Region
	Edit           = diff.Edit
)

// Short-name aliases.

type (
	Preview    = diff.DiffPreview
	Summarizer = diff.DiffSummarizer
)

// Functions.

var (
	NewDiffSandbox        = diff.NewDiffSandbox
	NewStagingArea        = diff.NewStagingArea
	NewDiffPreview        = diff.NewDiffPreview
	NewDiffSummarizer     = diff.NewDiffSummarizer
	NewTestSelector       = diff.NewTestSelector
	ComputeDiff           = diff.ComputeDiff
	ComputeMyersDiff      = diff.ComputeMyersDiff
	RenderUnified         = diff.RenderUnified
	Merge3                = diff.Merge3
	MergeClean            = diff.MergeClean
	FormatConflictMarkers = diff.FormatConflictMarkers
	LCS                   = diff.LCS
	EditScript            = diff.EditScript
	BuildDependencyGraph  = diff.BuildDependencyGraph
	GenerateTestCommand   = diff.GenerateTestCommand
)
