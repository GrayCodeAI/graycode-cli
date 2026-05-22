// This file re-exports symbols from the project sub-package so that existing
// callers of engine.ProjectAnalysis, engine.NewProjectAnalyzer, etc. keep compiling
// during the Stage 2 migration. See REFACTOR_PLAN.md.
package engine

import "github.com/GrayCodeAI/hawk/internal/engine/project"

type ProjectAnalysis = project.ProjectAnalysis
type ModuleInfo = project.ModuleInfo
type Pattern = project.Pattern
type ProjectAnalyzer = project.ProjectAnalyzer
type ProjectContext = project.ProjectContext
type ProjectSnapshot = project.ProjectSnapshot
type ProjectSnapshotCache = project.ProjectSnapshotCache
type ImpactAnalysis = project.ImpactAnalysis
type ImpactAnalyzer = project.ImpactAnalyzer
type DependencyUpdater = project.DependencyUpdater
type Dependency = project.Dependency
type UpdatePlan = project.UpdatePlan
type MigrationPlan = project.MigrationPlan
type MigrationStep = project.MigrationStep
type MigrationResult = project.MigrationResult
type MigrationPlanner = project.MigrationPlanner
type ReleaseManager = project.ReleaseManager
type Release = project.Release
type ChangeEntry = project.ChangeEntry
type ReleaseStats = project.ReleaseStats
type Convention = project.Convention
type ConventionSet = project.ConventionSet
type Violation = project.Violation

var NewProjectAnalyzer = project.NewProjectAnalyzer
var DetectArchitecture = project.DetectArchitecture
var DetectPatterns = project.DetectPatterns
var GenerateOnboardingDoc = project.GenerateOnboardingDoc
var FormatAnalysis = project.FormatAnalysis
var NewProjectContext = project.NewProjectContext
var NewProjectSnapshotCache = project.NewProjectSnapshotCache
var NewImpactAnalyzer = project.NewImpactAnalyzer
var BuildImportGraph = project.BuildImportGraph
var FormatImpact = project.FormatImpact
var NewDependencyUpdater = project.NewDependencyUpdater
var ClassifyUpdate = project.ClassifyUpdate
var ParseSemver = project.ParseSemver
var FormatOutdated = project.FormatOutdated
var FormatPlan = project.FormatPlan
var NewMigrationPlanner = project.NewMigrationPlanner
var NewReleaseManager = project.NewReleaseManager
var ParseConventionalCommit = project.ParseConventionalCommit
var BumpVersion = project.BumpVersion
var GenerateChangelog = project.GenerateChangelog
var FormatReleaseNotes = project.FormatReleaseNotes
var UpdateVersionFile = project.UpdateVersionFile
var NewConventionSet = project.NewConventionSet
var FormatViolations = project.FormatViolations
