// Package project is the Stage-1 namespace for project analysis, snapshots, impact analysis, dep updates, migrations, releases.
// See ../REFACTOR_PLAN.md.
package project

import "github.com/GrayCodeAI/hawk/engine"

type ProjectAnalysis = engine.ProjectAnalysis
type ModuleInfo = engine.ModuleInfo
type Pattern = engine.Pattern
type ProjectAnalyzer = engine.ProjectAnalyzer
type ProjectSnapshot = engine.ProjectSnapshot
type ProjectSnapshotCache = engine.ProjectSnapshotCache
type ImpactAnalysis = engine.ImpactAnalysis
type ImpactAnalyzer = engine.ImpactAnalyzer
type DependencyUpdater = engine.DependencyUpdater
type Dependency = engine.Dependency
type UpdatePlan = engine.UpdatePlan
type MigrationPlan = engine.MigrationPlan
type MigrationStep = engine.MigrationStep
type MigrationResult = engine.MigrationResult
type MigrationPlanner = engine.MigrationPlanner
type ReleaseManager = engine.ReleaseManager
type Release = engine.Release
type ChangeEntry = engine.ChangeEntry
type ReleaseStats = engine.ReleaseStats
type Convention = engine.Convention
type ConventionSet = engine.ConventionSet
type Violation = engine.Violation

var NewProjectAnalyzer = engine.NewProjectAnalyzer
var DetectArchitecture = engine.DetectArchitecture
var DetectPatterns = engine.DetectPatterns
var GenerateOnboardingDoc = engine.GenerateOnboardingDoc
var FormatAnalysis = engine.FormatAnalysis
var NewProjectSnapshotCache = engine.NewProjectSnapshotCache
var NewImpactAnalyzer = engine.NewImpactAnalyzer
var BuildImportGraph = engine.BuildImportGraph
var FormatImpact = engine.FormatImpact
var NewDependencyUpdater = engine.NewDependencyUpdater
var ClassifyUpdate = engine.ClassifyUpdate
var ParseSemver = engine.ParseSemver
var FormatOutdated = engine.FormatOutdated
var FormatPlan = engine.FormatPlan
var NewMigrationPlanner = engine.NewMigrationPlanner
var NewReleaseManager = engine.NewReleaseManager
var ParseConventionalCommit = engine.ParseConventionalCommit
var BumpVersion = engine.BumpVersion
var GenerateChangelog = engine.GenerateChangelog
var FormatReleaseNotes = engine.FormatReleaseNotes
var UpdateVersionFile = engine.UpdateVersionFile
var NewConventionSet = engine.NewConventionSet
var FormatViolations = engine.FormatViolations
