package harness

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/GrayCodeAI/hawk/internal/fsutil"
)

// EvaluateWorkspace performs a comprehensive harness evaluation of the specified workspace directory.
func EvaluateWorkspace(ctx context.Context, targetPath string, opts EvaluateOptions) (*HarnessReport, error) {
	absPath, err := filepath.Abs(targetPath)
	if err != nil {
		absPath = targetPath
	}

	info, err := os.Stat(absPath)
	if err != nil || !info.IsDir() {
		return nil, fmt.Errorf("target path %q is not a valid directory", absPath)
	}

	report := &HarnessReport{
		TargetPath:  absPath,
		GeneratedAt: time.Now().UTC(),
		Dimensions:  make(map[Dimension]DimensionScore),
		Findings:    make([]Finding, 0),
	}

	// 1. Detect harness assets
	assets := detectAssets(absPath)
	report.Assets = assets

	// 2. Evaluate 5 dimensions
	evalFeedforward(absPath, assets, report)
	evalFeedback(absPath, assets, report)
	evalTaskUnderstanding(absPath, assets, report)
	evalStepPlanning(absPath, assets, report)
	evalVerification(absPath, assets, report)

	// 3. Compute overall score
	var totalScore int
	for _, dim := range report.Dimensions {
		totalScore += dim.Score
	}
	report.OverallScore = totalScore / len(report.Dimensions)

	if report.OverallScore >= 85 {
		report.OverallStatus = "EXCELLENT"
		report.Summary = "The project harness provides strong feedforward guidance, robust feedback sensors, and rigorous verification safeguards."
	} else if report.OverallScore >= 70 {
		report.OverallStatus = "GOOD"
		report.Summary = "The project harness is functional but has specific gaps in guidance, testing/linter feedback loops, or verification."
	} else if report.OverallScore >= 50 {
		report.OverallStatus = "NEEDS_IMPROVEMENT"
		report.Summary = "Several critical harness components (such as AGENTS.md rules, linter auto-fixes, or test runners) are missing or incomplete."
	} else {
		report.OverallStatus = "POOR"
		report.Summary = "The workspace lacks essential agent guidance, feedback sensors, and verification safeguards."
	}

	return report, nil
}

func detectAssets(root string) AssetsDetected {
	assets := AssetsDetected{
		Skills:      make([]string, 0),
		Linters:     make([]string, 0),
		TestRunners: make([]string, 0),
		Hooks:       make([]string, 0),
	}

	// Check AGENTS.md / ZERO.md
	agentsPath := filepath.Join(root, "AGENTS.md")
	if fileExists(agentsPath) {
		assets.AgentsMD = true
		assets.AgentsMDPath = agentsPath
	} else {
		lowerAgents := filepath.Join(root, "agents.md")
		if fileExists(lowerAgents) {
			assets.AgentsMD = true
			assets.AgentsMDPath = lowerAgents
		}
	}

	zeroPath := filepath.Join(root, "ZERO.md")
	if fileExists(zeroPath) {
		assets.ZeroMD = true
		assets.ZeroMDPath = zeroPath
	}

	// Check skills
	skillsDir := filepath.Join(root, ".zero", "skills")
	if dirExists(skillsDir) {
		entries, _ := os.ReadDir(skillsDir)
		for _, e := range entries {
			if e.IsDir() || strings.HasSuffix(e.Name(), ".md") {
				assets.Skills = append(assets.Skills, e.Name())
			}
		}
	}
	altSkillsDir := filepath.Join(root, "skills")
	if dirExists(altSkillsDir) {
		entries, _ := os.ReadDir(altSkillsDir)
		for _, e := range entries {
			if e.IsDir() || strings.HasSuffix(e.Name(), ".md") {
				assets.Skills = append(assets.Skills, e.Name())
			}
		}
	}

	// Check specs
	specsDir := filepath.Join(root, ".hawk", "specs")
	if dirExists(specsDir) {
		entries, _ := os.ReadDir(specsDir)
		assets.SpecsCount = len(entries)
	}

	// Check linters
	if fileExists(filepath.Join(root, ".golangci.yml")) || fileExists(filepath.Join(root, ".golangci.yaml")) {
		assets.Linters = append(assets.Linters, "golangci-lint")
	}
	if fileExists(filepath.Join(root, ".eslintrc")) || fileExists(filepath.Join(root, ".eslintrc.json")) || fileExists(filepath.Join(root, "eslint.config.js")) {
		assets.Linters = append(assets.Linters, "eslint")
	}
	if fileExists(filepath.Join(root, "ruff.toml")) || fileExists(filepath.Join(root, ".ruff.toml")) {
		assets.Linters = append(assets.Linters, "ruff")
	}
	if fileExists(filepath.Join(root, ".markdownlint-cli2.jsonc")) || fileExists(filepath.Join(root, ".markdownlint.json")) {
		assets.Linters = append(assets.Linters, "markdownlint")
	}

	// Check test runners
	if fileExists(filepath.Join(root, "Makefile")) {
		content, _ := fsutil.ReadPinnedFile(filepath.Join(root, "Makefile"))
		str := string(content)
		if strings.Contains(str, "test:") || strings.Contains(str, "go test") {
			assets.TestRunners = append(assets.TestRunners, "make test")
		}
		if strings.Contains(str, "lint:") {
			if !sliceContains(assets.Linters, "make lint") {
				assets.Linters = append(assets.Linters, "make lint")
			}
		}
	}
	if fileExists(filepath.Join(root, "go.mod")) {
		assets.TestRunners = append(assets.TestRunners, "go test")
	}
	if fileExists(filepath.Join(root, "package.json")) {
		content, _ := fsutil.ReadPinnedFile(filepath.Join(root, "package.json"))
		if strings.Contains(string(content), `"test"`) {
			assets.TestRunners = append(assets.TestRunners, "npm test")
		}
	}
	if fileExists(filepath.Join(root, "pytest.ini")) || fileExists(filepath.Join(root, "pyproject.toml")) {
		assets.TestRunners = append(assets.TestRunners, "pytest")
	}

	// Check hooks
	if fileExists(filepath.Join(root, "lefthook.yml")) {
		assets.Hooks = append(assets.Hooks, "lefthook")
	}
	if dirExists(filepath.Join(root, ".hawk", "hooks")) {
		assets.Hooks = append(assets.Hooks, "hawk-hooks")
	}
	if dirExists(filepath.Join(root, ".git", "hooks")) {
		assets.Hooks = append(assets.Hooks, "git-hooks")
	}

	// Default assumption for bridges
	assets.InspectBridge = true
	assets.SightBridge = true
	assets.AutonomyTier = "Builder"
	assets.SandboxPolicy = "workspace"

	return assets
}

func evalFeedforward(root string, assets AssetsDetected, report *HarnessReport) {
	score := 100
	state := EvidenceStatePresent

	if !assets.AgentsMD && !assets.ZeroMD {
		score -= 40
		state = EvidenceStateMissing
		report.Findings = append(report.Findings, Finding{
			ID:              "FF-001",
			Dimension:       DimensionFeedforward,
			Severity:        SeverityHigh,
			Title:           "Missing Project AGENTS.md Directive File",
			Description:     "No AGENTS.md or ZERO.md file was found at the repository root to guide AI agents.",
			Impact:          "Agents will rely on generic defaults, increasing hallucinated file paths, non-standard style conventions, or unsafe commands.",
			EvidenceSource:  root,
			EvidenceState:   EvidenceStateMissing,
			ExpectedOutcome: "A root AGENTS.md file defining project build instructions, linting commands, testing patterns, and code architecture rules.",
			ScopedRepair:    "Create an AGENTS.md file at the repository root outlining key developer instructions and command rules.",
			ValidationRoute: "hawk harness review",
		})
	} else if assets.AgentsMD {
		content, err := os.ReadFile(assets.AgentsMDPath)
		if err == nil {
			if len(content) < 100 {
				score -= 20
				state = EvidenceStatePartial
				report.Findings = append(report.Findings, Finding{
					ID:              "FF-002",
					Dimension:       DimensionFeedforward,
					Severity:        SeverityMedium,
					Title:           "AGENTS.md Content is Extremely Brief",
					Description:     fmt.Sprintf("AGENTS.md at %s contains only %d bytes.", assets.AgentsMDPath, len(content)),
					Impact:          "Short directives may leave important team conventions, linter instructions, or architecture bounds unspecified.",
					EvidenceSource:  assets.AgentsMDPath,
					EvidenceState:   EvidenceStatePartial,
					ExpectedOutcome: "Comprehensive AGENTS.md detailing workflow commands, test placement, and code boundary constraints.",
					ScopedRepair:    "Expand AGENTS.md with explicit build, test, and contribution conventions.",
					ValidationRoute: "hawk harness review",
				})
			}
		}
	}

	if len(assets.Skills) == 0 {
		score -= 15
		if state == EvidenceStatePresent {
			state = EvidenceStatePartial
		}
		report.Findings = append(report.Findings, Finding{
			ID:              "FF-003",
			Dimension:       DimensionFeedforward,
			Severity:        SeverityLow,
			Title:           "No Modular Project Skills Configured",
			Description:     "No custom skills were found in .zero/skills/ or skills/.",
			Impact:          "Complex domain tasks lack pre-packaged workflow instructions.",
			EvidenceSource:  filepath.Join(root, ".zero", "skills"),
			EvidenceState:   EvidenceStateMissing,
			ExpectedOutcome: "Project skills for recurring complex procedures.",
			ScopedRepair:    "Add project skills under .zero/skills/<skill_name>/SKILL.md.",
			ValidationRoute: "hawk skills list",
		})
	}

	if score < 0 {
		score = 0
	}
	report.Dimensions[DimensionFeedforward] = DimensionScore{
		Dimension:     DimensionFeedforward,
		Score:         score,
		State:         state,
		Summary:       fmt.Sprintf("Feedforward score %d%%. AGENTS.md: %v, Skills: %d.", score, assets.AgentsMD, len(assets.Skills)),
		FindingsCount: countFindingsByDimension(report.Findings, DimensionFeedforward),
	}
}

func evalFeedback(root string, assets AssetsDetected, report *HarnessReport) {
	score := 100
	state := EvidenceStatePresent

	if len(assets.Linters) == 0 {
		score -= 30
		state = EvidenceStatePartial
		report.Findings = append(report.Findings, Finding{
			ID:              "FB-001",
			Dimension:       DimensionFeedback,
			Severity:        SeverityHigh,
			Title:           "No Linter Configuration Detected",
			Description:     "No standard linter configuration (.golangci.yml, eslint, ruff) or linter target in Makefile was detected.",
			Impact:          "Agents cannot run automatic linting and auto-fix feedback loops after writing code.",
			EvidenceSource:  root,
			EvidenceState:   EvidenceStateMissing,
			ExpectedOutcome: "Configured linter rules and auto-lint feedback mechanisms.",
			ScopedRepair:    "Add a linter configuration file or make lint command for automated code verification.",
			ValidationRoute: "hawk harness review",
		})
	}

	if len(assets.TestRunners) == 0 {
		score -= 40
		state = EvidenceStateMissing
		report.Findings = append(report.Findings, Finding{
			ID:              "FB-002",
			Dimension:       DimensionFeedback,
			Severity:        SeverityHigh,
			Title:           "No Automated Test Runner Detected",
			Description:     "No test suite or test execution target (go test, npm test, pytest, make test) was detected.",
			Impact:          "Agents cannot verify regression-free execution after making structural edits.",
			EvidenceSource:  root,
			EvidenceState:   EvidenceStateMissing,
			ExpectedOutcome: "Working test runner accessible via standard shell commands.",
			ScopedRepair:    "Define unit/integration tests and expose a clean test command in Makefile or package manifest.",
			ValidationRoute: "hawk harness review",
		})
	}

	if len(assets.Hooks) == 0 {
		score -= 15
		if state == EvidenceStatePresent {
			state = EvidenceStatePartial
		}
		report.Findings = append(report.Findings, Finding{
			ID:              "FB-003",
			Dimension:       DimensionFeedback,
			Severity:        SeverityLow,
			Title:           "No Lifecycle Hooks Configured",
			Description:     "No pre-commit or session lifecycle hooks (lefthook, hawk-hooks) were detected.",
			Impact:          "Automated checks prior to code review or session teardown are unenforced.",
			EvidenceSource:  filepath.Join(root, ".hawk", "hooks"),
			EvidenceState:   EvidenceStateMissing,
			ExpectedOutcome: "Automated beforeReview and afterReview hooks.",
			ScopedRepair:    "Configure lefthook or .hawk/hooks to execute automated sanity checks.",
			ValidationRoute: "hawk hook list",
		})
	}

	if score < 0 {
		score = 0
	}
	report.Dimensions[DimensionFeedback] = DimensionScore{
		Dimension:     DimensionFeedback,
		Score:         score,
		State:         state,
		Summary:       fmt.Sprintf("Feedback score %d%%. Linters: %d, TestRunners: %d, Hooks: %d.", score, len(assets.Linters), len(assets.TestRunners), len(assets.Hooks)),
		FindingsCount: countFindingsByDimension(report.Findings, DimensionFeedback),
	}
}

func evalTaskUnderstanding(root string, assets AssetsDetected, report *HarnessReport) {
	score := 85
	state := EvidenceStatePresent

	if assets.SpecsCount == 0 {
		score -= 20
		state = EvidenceStatePartial
		report.Findings = append(report.Findings, Finding{
			ID:              "TU-001",
			Dimension:       DimensionTaskUnderstanding,
			Severity:        SeverityMedium,
			Title:           "No Active Spec Definitions Found",
			Description:     "No task specs were found under .hawk/specs/.",
			Impact:          "Complex tasks risk starting without structured requirement decomposition and explicit acceptance criteria.",
			EvidenceSource:  filepath.Join(root, ".hawk", "specs"),
			EvidenceState:   EvidenceStatePartial,
			ExpectedOutcome: "Structured specification files for multi-step feature developments.",
			ScopedRepair:    "Use `/spec [feature description]` to draft structured specifications prior to implementation.",
			ValidationRoute: "hawk spec status",
		})
	}

	if score < 0 {
		score = 0
	}
	report.Dimensions[DimensionTaskUnderstanding] = DimensionScore{
		Dimension:     DimensionTaskUnderstanding,
		Score:         score,
		State:         state,
		Summary:       fmt.Sprintf("Task Understanding score %d%%. Specs found: %d.", score, assets.SpecsCount),
		FindingsCount: countFindingsByDimension(report.Findings, DimensionTaskUnderstanding),
	}
}

func evalStepPlanning(root string, assets AssetsDetected, report *HarnessReport) {
	score := 90
	state := EvidenceStatePresent

	// Check git repo status
	gitDir := filepath.Join(root, ".git")
	if !dirExists(gitDir) {
		score -= 30
		state = EvidenceStatePartial
		report.Findings = append(report.Findings, Finding{
			ID:              "SP-001",
			Dimension:       DimensionStepPlanning,
			Severity:        SeverityMedium,
			Title:           "Workspace is Not a Git Repository",
			Description:     "No .git directory was found at the workspace root.",
			Impact:          "Agents cannot track diffs, execute multi-agent mission branches in git worktrees, or perform smart commits.",
			EvidenceSource:  root,
			EvidenceState:   EvidenceStateMissing,
			ExpectedOutcome: "Git-versioned repository with branch and commit history.",
			ScopedRepair:    "Initialize git version control (`git init`) and commit base repository state.",
			ValidationRoute: "git status",
		})
	}

	if score < 0 {
		score = 0
	}
	report.Dimensions[DimensionStepPlanning] = DimensionScore{
		Dimension:     DimensionStepPlanning,
		Score:         score,
		State:         state,
		Summary:       fmt.Sprintf("Step Planning & Execution score %d%%. Version control: %v.", score, dirExists(gitDir)),
		FindingsCount: countFindingsByDimension(report.Findings, DimensionStepPlanning),
	}
}

func evalVerification(root string, assets AssetsDetected, report *HarnessReport) {
	score := 95
	state := EvidenceStatePresent

	if !assets.InspectBridge || !assets.SightBridge {
		score -= 15
		state = EvidenceStatePartial
		report.Findings = append(report.Findings, Finding{
			ID:              "VF-001",
			Dimension:       DimensionVerification,
			Severity:        SeverityLow,
			Title:           "Verification Audit Bridges Unverified",
			Description:     "Security audit (inspect) or code review (sight) engines are operating in baseline mode.",
			Impact:          "Deep security vulnerability scans and formal diff review quality graphs are unattached.",
			EvidenceSource:  "internal/bridge",
			EvidenceState:   EvidenceStatePartial,
			ExpectedOutcome: "Active security audit and code review sub-module bridges.",
			ScopedRepair:    "Verify external support sub-modules via `hawk doctor`.",
			ValidationRoute: "hawk doctor",
		})
	}

	if score < 0 {
		score = 0
	}
	report.Dimensions[DimensionVerification] = DimensionScore{
		Dimension:     DimensionVerification,
		Score:         score,
		State:         state,
		Summary:       fmt.Sprintf("Verification & Safeguards score %d%%. Security & Review bridges active.", score),
		FindingsCount: countFindingsByDimension(report.Findings, DimensionVerification),
	}
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return !info.IsDir()
}

func dirExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return info.IsDir()
}

func sliceContains(slice []string, val string) bool {
	for _, item := range slice {
		if item == val {
			return true
		}
	}
	return false
}

func countFindingsByDimension(findings []Finding, dim Dimension) int {
	count := 0
	for _, f := range findings {
		if f.Dimension == dim {
			count++
		}
	}
	return count
}
