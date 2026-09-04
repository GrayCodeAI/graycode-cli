// This file re-exports symbols from the project sub-package so that existing
// callers of engine.ProjectAnalysis, engine.NewProjectAnalyzer, etc. keep compiling
// during the Stage 2 migration. See docs/plans/engine-refactor-plan.md.
package engine

import "github.com/GrayCodeAI/graycode-cli/internal/engine/project"

type (
	ProjectAnalysis      = project.ProjectAnalysis
	ModuleInfo           = project.ModuleInfo
	Pattern              = project.Pattern
	ProjectAnalyzer      = project.ProjectAnalyzer
	ProjectContext       = project.ProjectContext
	ProjectSnapshot      = project.ProjectSnapshot
	ProjectSnapshotCache = project.ProjectSnapshotCache
	ImpactAnalysis       = project.ImpactAnalysis
	ImpactAnalyzer       = project.ImpactAnalyzer
	DependencyUpdater    = project.DependencyUpdater
	Dependency           = project.Dependency
	UpdatePlan           = project.UpdatePlan
	MigrationPlan        = project.MigrationPlan
	MigrationStep        = project.MigrationStep
	MigrationResult      = project.MigrationResult
	MigrationPlanner     = project.MigrationPlanner
	ReleaseManager       = project.ReleaseManager
	Release              = project.Release
	ChangeEntry          = project.ChangeEntry
	ReleaseStats         = project.ReleaseStats
	Convention           = project.Convention
	ConventionSet        = project.ConventionSet
	Violation            = project.Violation
)

var (
	NewProjectAnalyzer      = project.NewProjectAnalyzer
	DetectArchitecture      = project.DetectArchitecture
	DetectPatterns          = project.DetectPatterns
	GenerateOnboardingDoc   = project.GenerateOnboardingDoc
	FormatAnalysis          = project.FormatAnalysis
	NewProjectContext       = project.NewProjectContext
	NewProjectSnapshotCache = project.NewProjectSnapshotCache
	NewImpactAnalyzer       = project.NewImpactAnalyzer
	BuildImportGraph        = project.BuildImportGraph
	FormatImpact            = project.FormatImpact
	NewDependencyUpdater    = project.NewDependencyUpdater
	ClassifyUpdate          = project.ClassifyUpdate
	ParseSemver             = project.ParseSemver
	FormatOutdated          = project.FormatOutdated
	FormatPlan              = project.FormatPlan
	NewMigrationPlanner     = project.NewMigrationPlanner
	NewReleaseManager       = project.NewReleaseManager
	ParseConventionalCommit = project.ParseConventionalCommit
	BumpVersion             = project.BumpVersion
	GenerateChangelog       = project.GenerateChangelog
	FormatReleaseNotes      = project.FormatReleaseNotes
	UpdateVersionFile       = project.UpdateVersionFile
	NewConventionSet        = project.NewConventionSet
	FormatViolations        = project.FormatViolations
)
