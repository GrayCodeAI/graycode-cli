// Package diff is the Stage-1 namespace for diff sandbox, staging, preview,
// summariser, test selector, and 3-way merge. See ../REFACTOR_PLAN.md.
package diff

import "github.com/GrayCodeAI/hawk/engine"

type PendingChange = engine.PendingChange
type DiffSandbox = engine.DiffSandbox
type StagingArea = engine.StagingArea
type StagedChange = engine.StagedChange
type StagedHunk = engine.StagedHunk
type Preview = engine.DiffPreview
type FileChange = engine.FileChange
type Hunk = engine.DiffHunk
type Line = engine.DiffLine
type ChangeStats = engine.ChangeStats
type Summary = engine.DiffSummary
type FileSummary = engine.FileSummary
type Summarizer = engine.DiffSummarizer
type TestSelector = engine.TestSelector
type SelectedTests = engine.SelectedTests
type Diff3Result = engine.Diff3Result
type Diff3Conflict = engine.Diff3Conflict
type Diff3Stats = engine.Diff3Stats
type Diff3Region = engine.Diff3Region
type Edit = engine.Edit

func NewDiffSandbox() *DiffSandbox                     { return engine.NewDiffSandbox() }
func NewStagingArea() *StagingArea                     { return engine.NewStagingArea() }
func NewDiffPreview() *Preview                         { return engine.NewDiffPreview() }
func NewSummarizer() *Summarizer                       { return engine.NewDiffSummarizer() }
func NewTestSelector(projectDir string) *TestSelector  { return engine.NewTestSelector(projectDir) }
func ComputeDiff(old, new string) []Hunk               { return engine.ComputeDiff(old, new) }
func ComputeMyersDiff(a, b []string) []Line            { return engine.ComputeMyersDiff(a, b) }
func RenderUnified(change *FileChange) string          { return engine.RenderUnified(change) }
func Merge3(base, ours, theirs string) *Diff3Result    { return engine.Merge3(base, ours, theirs) }
func MergeClean(base, ours, theirs string) (string, bool) { return engine.MergeClean(base, ours, theirs) }
func FormatConflictMarkers(c Diff3Conflict) string     { return engine.FormatConflictMarkers(c) }
func LCS(a, b []string) []string                       { return engine.LCS(a, b) }
func EditScript(from, to []string) []Edit              { return engine.EditScript(from, to) }
func BuildDependencyGraph(dir string) map[string][]string { return engine.BuildDependencyGraph(dir) }
func GenerateTestCommand(s *SelectedTests, lang string) string { return engine.GenerateTestCommand(s, lang) }
