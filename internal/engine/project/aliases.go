// Package project is the Stage-1 namespace for project analysis, snapshots, impact analysis, dep updates, migrations, releases.
// See ../REFACTOR_PLAN.md.
package project

import "github.com/GrayCodeAI/hawk/engine"

type (
	ProjectAnalysis      = engine.ProjectAnalysis
	ModuleInfo           = engine.ModuleInfo
	Pattern              = engine.Pattern
	ProjectAnalyzer      = engine.ProjectAnalyzer
	ProjectSnapshot      = engine.ProjectSnapshot
	ProjectSnapshotCache = engine.ProjectSnapshotCache
	ImpactAnalysis       = engine.ImpactAnalysis
	ImpactAnalyzer       = engine.ImpactAnalyzer
	DependencyUpdater    = engine.DependencyUpdater
	Dependency           = engine.Dependency
	UpdatePlan           = engine.UpdatePlan
	MigrationPlan        = engine.MigrationPlan
	MigrationStep        = engine.MigrationStep
	MigrationResult      = engine.MigrationResult
	MigrationPlanner     = engine.MigrationPlanner
	ReleaseManager       = engine.ReleaseManager
	Release              = engine.Release
	ChangeEntry          = engine.ChangeEntry
	ReleaseStats         = engine.ReleaseStats
	Convention           = engine.Convention
	ConventionSet        = engine.ConventionSet
	Violation            = engine.Violation
)

var (
	NewProjectAnalyzer      = engine.NewProjectAnalyzer
	DetectArchitecture      = engine.DetectArchitecture
	DetectPatterns          = engine.DetectPatterns
	GenerateOnboardingDoc   = engine.GenerateOnboardingDoc
	FormatAnalysis          = engine.FormatAnalysis
	NewProjectSnapshotCache = engine.NewProjectSnapshotCache
	NewImpactAnalyzer       = engine.NewImpactAnalyzer
	BuildImportGraph        = engine.BuildImportGraph
	FormatImpact            = engine.FormatImpact
	NewDependencyUpdater    = engine.NewDependencyUpdater
	ClassifyUpdate          = engine.ClassifyUpdate
	ParseSemver             = engine.ParseSemver
	FormatOutdated          = engine.FormatOutdated
	FormatPlan              = engine.FormatPlan
	NewMigrationPlanner     = engine.NewMigrationPlanner
	NewReleaseManager       = engine.NewReleaseManager
	ParseConventionalCommit = engine.ParseConventionalCommit
	BumpVersion             = engine.BumpVersion
	GenerateChangelog       = engine.GenerateChangelog
	FormatReleaseNotes      = engine.FormatReleaseNotes
	UpdateVersionFile       = engine.UpdateVersionFile
	NewConventionSet        = engine.NewConventionSet
	FormatViolations        = engine.FormatViolations
)
