package engine

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
)

// DependencyUpdater detects outdated packages and helps update them safely.
type DependencyUpdater struct {
	ProjectDir string
	Language   string
	mu         sync.Mutex
}

// Dependency represents a single package dependency with version info.
type Dependency struct {
	Name               string
	CurrentVersion     string
	LatestVersion      string
	UpdateType         string // "major", "minor", "patch"
	IsDirectDep        bool
	HasBreakingChanges bool
	SecurityFix        bool
	Changelog          string
}

// UpdatePlan represents a structured plan for updating dependencies.
type UpdatePlan struct {
	Dependencies      []Dependency
	RiskLevel         string // "low", "medium", "high"
	TestCommand       string
	RollbackCommand   string
	EstimatedBreaking int
}

// NewDependencyUpdater creates a new DependencyUpdater for the given project directory.
func NewDependencyUpdater(projectDir string) *DependencyUpdater {
	du := &DependencyUpdater{
		ProjectDir: projectDir,
	}
	du.Language = du.DetectLanguage()
	return du
}

// DetectLanguage determines the project language by checking for known manifest files.
func (du *DependencyUpdater) DetectLanguage() string {
	checks := []struct {
		file string
		lang string
	}{
		{"go.mod", "go"},
		{"package.json", "javascript"},
		{"requirements.txt", "python"},
		{"Cargo.toml", "rust"},
	}

	for _, c := range checks {
		path := filepath.Join(du.ProjectDir, c.file)
		if _, err := os.Stat(path); err == nil {
			return c.lang
		}
	}
	return "unknown"
}

// ListOutdated returns the list of outdated dependencies for the detected language.
func (du *DependencyUpdater) ListOutdated() ([]Dependency, error) {
	du.mu.Lock()
	defer du.mu.Unlock()

	switch du.Language {
	case "go":
		return du.listOutdatedGo()
	case "javascript":
		return du.listOutdatedJS()
	case "python":
		return du.listOutdatedPython()
	case "rust":
		return du.listOutdatedRust()
	default:
		return nil, fmt.Errorf("unsupported language: %s", du.Language)
	}
}

func (du *DependencyUpdater) listOutdatedGo() ([]Dependency, error) {
	cmd := exec.Command("go", "list", "-m", "-u", "all")
	cmd.Dir = du.ProjectDir
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("go list failed: %w", err)
	}

	var deps []Dependency
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	for _, line := range lines {
		// Format: module version [version]
		// The bracketed version is the latest available
		if !strings.Contains(line, "[") {
			continue
		}

		parts := strings.Fields(line)
		if len(parts) < 3 {
			continue
		}

		name := parts[0]
		current := parts[1]
		latest := strings.Trim(parts[2], "[]")

		updateType := ClassifyUpdate(current, latest)
		dep := Dependency{
			Name:               name,
			CurrentVersion:     current,
			LatestVersion:      latest,
			UpdateType:         updateType,
			IsDirectDep:        true,
			HasBreakingChanges: updateType == "major",
		}
		deps = append(deps, dep)
	}
	return deps, nil
}

func (du *DependencyUpdater) listOutdatedJS() ([]Dependency, error) {
	cmd := exec.Command("npm", "outdated", "--json")
	cmd.Dir = du.ProjectDir
	output, err := cmd.Output()
	// npm outdated exits with code 1 when there are outdated packages
	if err != nil && len(output) == 0 {
		return nil, fmt.Errorf("npm outdated failed: %w", err)
	}

	var result map[string]struct {
		Current string `json:"current"`
		Latest  string `json:"latest"`
		Type    string `json:"type"`
	}
	if err := json.Unmarshal(output, &result); err != nil {
		return nil, fmt.Errorf("failed to parse npm output: %w", err)
	}

	var deps []Dependency
	for name, info := range result {
		updateType := ClassifyUpdate(info.Current, info.Latest)
		dep := Dependency{
			Name:               name,
			CurrentVersion:     info.Current,
			LatestVersion:      info.Latest,
			UpdateType:         updateType,
			IsDirectDep:        info.Type == "dependencies",
			HasBreakingChanges: updateType == "major",
		}
		deps = append(deps, dep)
	}
	return deps, nil
}

func (du *DependencyUpdater) listOutdatedPython() ([]Dependency, error) {
	cmd := exec.Command("pip", "list", "--outdated", "--format=json")
	cmd.Dir = du.ProjectDir
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("pip list failed: %w", err)
	}

	var result []struct {
		Name           string `json:"name"`
		CurrentVersion string `json:"version"`
		LatestVersion  string `json:"latest_version"`
	}
	if err := json.Unmarshal(output, &result); err != nil {
		return nil, fmt.Errorf("failed to parse pip output: %w", err)
	}

	var deps []Dependency
	for _, item := range result {
		updateType := ClassifyUpdate(item.CurrentVersion, item.LatestVersion)
		dep := Dependency{
			Name:               item.Name,
			CurrentVersion:     item.CurrentVersion,
			LatestVersion:      item.LatestVersion,
			UpdateType:         updateType,
			IsDirectDep:        true,
			HasBreakingChanges: updateType == "major",
		}
		deps = append(deps, dep)
	}
	return deps, nil
}

func (du *DependencyUpdater) listOutdatedRust() ([]Dependency, error) {
	cmd := exec.Command("cargo", "outdated", "--format=json")
	cmd.Dir = du.ProjectDir
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("cargo outdated failed: %w", err)
	}

	var result struct {
		Dependencies []struct {
			Name    string `json:"name"`
			Current string `json:"project"`
			Latest  string `json:"latest"`
			Kind    string `json:"kind"`
		} `json:"dependencies"`
	}
	if err := json.Unmarshal(output, &result); err != nil {
		return nil, fmt.Errorf("failed to parse cargo output: %w", err)
	}

	var deps []Dependency
	for _, item := range result.Dependencies {
		updateType := ClassifyUpdate(item.Current, item.Latest)
		dep := Dependency{
			Name:               item.Name,
			CurrentVersion:     item.Current,
			LatestVersion:      item.Latest,
			UpdateType:         updateType,
			IsDirectDep:        item.Kind == "Normal",
			HasBreakingChanges: updateType == "major",
		}
		deps = append(deps, dep)
	}
	return deps, nil
}

// ClassifyUpdate compares two semver strings and returns "major", "minor", or "patch".
func ClassifyUpdate(current, latest string) string {
	curMajor, curMinor, _, err1 := ParseSemver(current)
	latMajor, latMinor, _, err2 := ParseSemver(latest)

	if err1 != nil || err2 != nil {
		return "patch"
	}

	if latMajor > curMajor {
		return "major"
	}
	if latMinor > curMinor {
		return "minor"
	}
	return "patch"
}

// GeneratePlan creates an UpdatePlan from a list of dependencies.
// It prioritizes security fixes, then patch, minor, and major updates.
func (du *DependencyUpdater) GeneratePlan(deps []Dependency) *UpdatePlan {
	if len(deps) == 0 {
		return &UpdatePlan{
			Dependencies: deps,
			RiskLevel:    "low",
		}
	}

	// Sort: security first, then patch, minor, major
	sorted := make([]Dependency, len(deps))
	copy(sorted, deps)
	sort.SliceStable(sorted, func(i, j int) bool {
		// Security fixes always first
		if sorted[i].SecurityFix && !sorted[j].SecurityFix {
			return true
		}
		if !sorted[i].SecurityFix && sorted[j].SecurityFix {
			return false
		}
		// Then by update type priority: patch < minor < major
		priority := map[string]int{"patch": 0, "minor": 1, "major": 2}
		return priority[sorted[i].UpdateType] < priority[sorted[j].UpdateType]
	})

	// Determine risk level
	riskLevel := "low"
	estimatedBreaking := 0
	for _, dep := range sorted {
		if dep.UpdateType == "major" {
			riskLevel = "high"
			estimatedBreaking++
		} else if dep.UpdateType == "minor" && riskLevel == "low" {
			riskLevel = "medium"
		}
	}

	// Determine test and rollback commands
	testCmd := du.suggestTestCommand()
	rollbackCmd := du.suggestRollbackCommand()

	return &UpdatePlan{
		Dependencies:      sorted,
		RiskLevel:         riskLevel,
		TestCommand:       testCmd,
		RollbackCommand:   rollbackCmd,
		EstimatedBreaking: estimatedBreaking,
	}
}

func (du *DependencyUpdater) suggestTestCommand() string {
	switch du.Language {
	case "go":
		return "go test ./..."
	case "javascript":
		return "npm test"
	case "python":
		return "pytest"
	case "rust":
		return "cargo test"
	default:
		return "make test"
	}
}

func (du *DependencyUpdater) suggestRollbackCommand() string {
	switch du.Language {
	case "go":
		return "git checkout go.mod go.sum && go mod download"
	case "javascript":
		return "git checkout package.json package-lock.json && npm install"
	case "python":
		return "git checkout requirements.txt && pip install -r requirements.txt"
	case "rust":
		return "git checkout Cargo.toml Cargo.lock && cargo build"
	default:
		return "git checkout ."
	}
}

// ApplyUpdate applies a single dependency update using the appropriate package manager.
func (du *DependencyUpdater) ApplyUpdate(dep Dependency) error {
	du.mu.Lock()
	defer du.mu.Unlock()

	var cmd *exec.Cmd
	switch du.Language {
	case "go":
		pkg := dep.Name + "@" + dep.LatestVersion
		cmd = exec.Command("go", "get", pkg)
	case "javascript":
		pkg := dep.Name + "@" + dep.LatestVersion
		cmd = exec.Command("npm", "install", pkg)
	case "python":
		pkg := dep.Name + "==" + dep.LatestVersion
		cmd = exec.Command("pip", "install", pkg)
	case "rust":
		return du.applyRustUpdate(dep)
	default:
		return fmt.Errorf("unsupported language: %s", du.Language)
	}

	cmd.Dir = du.ProjectDir
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("update failed for %s: %s: %w", dep.Name, string(output), err)
	}
	return nil
}

func (du *DependencyUpdater) applyRustUpdate(dep Dependency) error {
	cargoPath := filepath.Join(du.ProjectDir, "Cargo.toml")
	data, err := os.ReadFile(cargoPath)
	if err != nil {
		return fmt.Errorf("failed to read Cargo.toml: %w", err)
	}

	content := string(data)
	// Simple replacement: find the dependency line and update the version
	oldPattern := dep.Name + ` = "` + dep.CurrentVersion + `"`
	newPattern := dep.Name + ` = "` + dep.LatestVersion + `"`
	if strings.Contains(content, oldPattern) {
		content = strings.Replace(content, oldPattern, newPattern, 1)
	} else {
		// Try with version key format
		oldPattern = `name = "` + dep.Name + `"` + "\n" + `version = "` + dep.CurrentVersion + `"`
		newPattern = `name = "` + dep.Name + `"` + "\n" + `version = "` + dep.LatestVersion + `"`
		content = strings.Replace(content, oldPattern, newPattern, 1)
	}

	if err := os.WriteFile(cargoPath, []byte(content), 0o644); err != nil {
		return fmt.Errorf("failed to write Cargo.toml: %w", err)
	}
	return nil
}

// ParseSemver parses a version string into major, minor, patch components.
// It handles versions with or without a "v" prefix.
func ParseSemver(version string) (major, minor, patch int, err error) {
	v := strings.TrimPrefix(version, "v")

	// Remove any pre-release or build metadata
	if idx := strings.IndexAny(v, "-+"); idx != -1 {
		v = v[:idx]
	}

	parts := strings.Split(v, ".")
	if len(parts) < 1 {
		return 0, 0, 0, fmt.Errorf("invalid semver: %s", version)
	}

	major, err = strconv.Atoi(parts[0])
	if err != nil {
		return 0, 0, 0, fmt.Errorf("invalid major version in %s: %w", version, err)
	}

	if len(parts) >= 2 {
		minor, err = strconv.Atoi(parts[1])
		if err != nil {
			return 0, 0, 0, fmt.Errorf("invalid minor version in %s: %w", version, err)
		}
	}

	if len(parts) >= 3 {
		patch, err = strconv.Atoi(parts[2])
		if err != nil {
			return 0, 0, 0, fmt.Errorf("invalid patch version in %s: %w", version, err)
		}
	}

	return major, minor, patch, nil
}

// FormatOutdated formats a list of outdated dependencies into a human-readable report.
func FormatOutdated(deps []Dependency) string {
	if len(deps) == 0 {
		return "All dependencies are up to date!"
	}

	var security, patchDeps, minorDeps, majorDeps []Dependency
	for _, dep := range deps {
		if dep.SecurityFix {
			security = append(security, dep)
		} else {
			switch dep.UpdateType {
			case "major":
				majorDeps = append(majorDeps, dep)
			case "minor":
				minorDeps = append(minorDeps, dep)
			default:
				patchDeps = append(patchDeps, dep)
			}
		}
	}

	var sb strings.Builder
	sb.WriteString("Outdated Dependencies:\n")

	if len(security) > 0 {
		sb.WriteString("\n⚠ SECURITY:\n")
		for _, dep := range security {
			sb.WriteString(fmt.Sprintf("  %-40s %s → %s (%s, security fix)\n",
				dep.Name, dep.CurrentVersion, dep.LatestVersion, dep.UpdateType))
		}
	}

	if len(patchDeps) > 0 {
		sb.WriteString("\n\U0001f7e2 PATCH:\n")
		for _, dep := range patchDeps {
			sb.WriteString(fmt.Sprintf("  %-40s %s → %s (%s)\n",
				dep.Name, dep.CurrentVersion, dep.LatestVersion, dep.UpdateType))
		}
	}

	if len(minorDeps) > 0 {
		sb.WriteString("\n\U0001f4e6 MINOR:\n")
		for _, dep := range minorDeps {
			sb.WriteString(fmt.Sprintf("  %-40s %s → %s (%s)\n",
				dep.Name, dep.CurrentVersion, dep.LatestVersion, dep.UpdateType))
		}
	}

	if len(majorDeps) > 0 {
		sb.WriteString("\n\U0001f534 MAJOR:\n")
		for _, dep := range majorDeps {
			sb.WriteString(fmt.Sprintf("  %-40s %s → %s (%s, breaking changes likely)\n",
				dep.Name, dep.CurrentVersion, dep.LatestVersion, dep.UpdateType))
		}
	}

	// Summary
	sb.WriteString(fmt.Sprintf("\nTotal: %d outdated", len(deps)))
	parts := []string{}
	if len(security) > 0 {
		parts = append(parts, fmt.Sprintf("%d security", len(security)))
	}
	if len(patchDeps) > 0 {
		parts = append(parts, fmt.Sprintf("%d patch", len(patchDeps)))
	}
	if len(minorDeps) > 0 {
		parts = append(parts, fmt.Sprintf("%d minor", len(minorDeps)))
	}
	if len(majorDeps) > 0 {
		parts = append(parts, fmt.Sprintf("%d major", len(majorDeps)))
	}
	if len(parts) > 0 {
		sb.WriteString(fmt.Sprintf(" (%s)", strings.Join(parts, ", ")))
	}
	sb.WriteString("\n")

	// Recommendation
	if len(security) > 0 {
		sb.WriteString("Recommendation: Update security fixes immediately")
		if len(minorDeps) > 0 {
			sb.WriteString(", minor updates are safe.")
		} else {
			sb.WriteString(".")
		}
		sb.WriteString("\n")
	} else if len(majorDeps) == 0 {
		sb.WriteString("Recommendation: All updates appear safe to apply.\n")
	} else {
		sb.WriteString("Recommendation: Apply patch and minor updates. Review major updates carefully.\n")
	}

	return sb.String()
}

// FormatPlan formats an UpdatePlan into a human-readable string.
func FormatPlan(plan *UpdatePlan) string {
	if plan == nil || len(plan.Dependencies) == 0 {
		return "No updates planned."
	}

	var sb strings.Builder
	sb.WriteString("Update Plan:\n")
	sb.WriteString(fmt.Sprintf("  Risk Level: %s\n", strings.ToUpper(plan.RiskLevel)))
	sb.WriteString(fmt.Sprintf("  Estimated Breaking Changes: %d\n", plan.EstimatedBreaking))
	sb.WriteString(fmt.Sprintf("  Test Command: %s\n", plan.TestCommand))
	sb.WriteString(fmt.Sprintf("  Rollback Command: %s\n", plan.RollbackCommand))
	sb.WriteString("\n  Update Order:\n")

	for i, dep := range plan.Dependencies {
		prefix := "  "
		if dep.SecurityFix {
			prefix = "⚠ "
		}
		sb.WriteString(fmt.Sprintf("  %d. %s%-40s %s → %s (%s)\n",
			i+1, prefix, dep.Name, dep.CurrentVersion, dep.LatestVersion, dep.UpdateType))
	}

	return sb.String()
}

// BatchUpdate updates dependencies up to the specified risk level.
// maxRisk can be "patch", "minor", or "major" (includes all lower levels).
func (du *DependencyUpdater) BatchUpdate(deps []Dependency, maxRisk string) ([]string, []error) {
	riskOrder := map[string]int{"patch": 0, "minor": 1, "major": 2}
	maxLevel, ok := riskOrder[maxRisk]
	if !ok {
		maxLevel = 0
	}

	var updated []string
	var errors []error

	// Sort by risk: patch first, then minor, then major
	sorted := make([]Dependency, len(deps))
	copy(sorted, deps)
	sort.SliceStable(sorted, func(i, j int) bool {
		// Security fixes always first
		if sorted[i].SecurityFix && !sorted[j].SecurityFix {
			return true
		}
		if !sorted[i].SecurityFix && sorted[j].SecurityFix {
			return false
		}
		return riskOrder[sorted[i].UpdateType] < riskOrder[sorted[j].UpdateType]
	})

	for _, dep := range sorted {
		depLevel, exists := riskOrder[dep.UpdateType]
		if !exists {
			depLevel = 0
		}
		// Always include security fixes regardless of risk level
		if depLevel > maxLevel && !dep.SecurityFix {
			continue
		}

		if err := du.ApplyUpdate(dep); err != nil {
			errors = append(errors, fmt.Errorf("%s: %w", dep.Name, err))
		} else {
			updated = append(updated, dep.Name)
		}
	}

	return updated, errors
}
