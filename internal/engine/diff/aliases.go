// Package diff is the Stage-1 namespace for diff sandbox, staging, preview,
// summariser, test selector, and 3-way merge. See ../REFACTOR_PLAN.md.
package diff

import "github.com/GrayCodeAI/hawk/internal/engine"

type (
	PendingChange = engine.PendingChange
	DiffSandbox   = engine.DiffSandbox
	StagingArea   = engine.StagingArea
	StagedChange  = engine.StagedChange
	StagedHunk    = engine.StagedHunk
	Preview       = engine.DiffPreview
	FileChange    = engine.FileChange
	Hunk          = engine.DiffHunk
	Line          = engine.DiffLine
	ChangeStats   = engine.ChangeStats
	Summary       = engine.DiffSummary
	FileSummary   = engine.FileSummary
	Summarizer    = engine.DiffSummarizer
	TestSelector  = engine.TestSelector
	SelectedTests = engine.SelectedTests
	Diff3Result   = engine.Diff3Result
	Diff3Conflict = engine.Diff3Conflict
	Diff3Stats    = engine.Diff3Stats
	Diff3Region   = engine.Diff3Region
	Edit          = engine.Edit
)

func NewDiffSandbox() *DiffSandbox                    { return engine.NewDiffSandbox() }
func NewStagingArea() *StagingArea                    { return engine.NewStagingArea() }
func NewDiffPreview() *Preview                        { return engine.NewDiffPreview() }
func NewSummarizer() *Summarizer                      { return engine.NewDiffSummarizer() }
func NewTestSelector(projectDir string) *TestSelector { return engine.NewTestSelector(projectDir) }
func ComputeDiff(old, new string) []Hunk              { return engine.ComputeDiff(old, new) }
func ComputeMyersDiff(a, b []string) []Line           { return engine.ComputeMyersDiff(a, b) }
func RenderUnified(change *FileChange) string         { return engine.RenderUnified(change) }
func Merge3(base, ours, theirs string) *Diff3Result   { return engine.Merge3(base, ours, theirs) }
func MergeClean(base, ours, theirs string) (string, bool) {
	return engine.MergeClean(base, ours, theirs)
}
func FormatConflictMarkers(c Diff3Conflict) string        { return engine.FormatConflictMarkers(c) }
func LCS(a, b []string) []string                          { return engine.LCS(a, b) }
func EditScript(from, to []string) []Edit                 { return engine.EditScript(from, to) }
func BuildDependencyGraph(dir string) map[string][]string { return engine.BuildDependencyGraph(dir) }

func GenerateTestCommand(s *SelectedTests, lang string) string {
	return engine.GenerateTestCommand(s, lang)
}
