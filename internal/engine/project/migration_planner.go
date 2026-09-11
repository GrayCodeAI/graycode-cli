package project

import (
	"bufio"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/GrayCodeAI/hawk/internal/fsutil"
)

// MigrationPlan represents a complete plan for a large-scale code migration.
type MigrationPlan struct {
	Name             string
	Description      string
	Steps            []MigrationStep
	AffectedFiles    []string
	EstimatedChanges int
	RiskLevel        string
	Validated        bool
	CreatedAt        time.Time
}

// MigrationStep represents a single step within a migration plan.
type MigrationStep struct {
	Order       int
	Description string
	Pattern     string // regex to find
	Replacement string
	Files       []string
	Manual      bool // needs human review
	Completed   bool
	Error       string
}

// MigrationResult holds the outcome of executing a migration plan.
type MigrationResult struct {
	Completed    int
	Skipped      int
	Failed       int
	ManualReview []MigrationStep
}

// MigrationPlanner plans and executes large-scale code migrations.
type MigrationPlanner struct {
	ProjectDir string
	mu         sync.Mutex

	// backups stores original file contents for rollback, keyed by file path.
	backups map[string][]byte
}

// NewMigrationPlanner creates a new MigrationPlanner for the given project directory.
func NewMigrationPlanner(projectDir string) *MigrationPlanner {
	return &MigrationPlanner{
		ProjectDir: projectDir,
		backups:    make(map[string][]byte),
	}
}

// PlanRename creates a migration plan to rename oldName to newName across the project.
// It orders definitions first, then usages.
func (mp *MigrationPlanner) PlanRename(oldName, newName string) (*MigrationPlan, error) {
	mp.mu.Lock()
	defer mp.mu.Unlock()

	if oldName == "" || newName == "" {
		return nil, fmt.Errorf("oldName and newName must not be empty")
	}

	plan := &MigrationPlan{
		Name:        fmt.Sprintf("Rename %s to %s", oldName, newName),
		Description: fmt.Sprintf("Rename all occurrences of %q to %q across the project", oldName, newName),
		CreatedAt:   time.Now(),
	}

	// Find all files containing the old name.
	matches, err := mp.findFilesContaining(oldName)
	if err != nil {
		return nil, fmt.Errorf("scanning project: %w", err)
	}

	if len(matches) == 0 {
		plan.RiskLevel = "NONE"
		plan.Validated = true
		return plan, nil
	}

	plan.AffectedFiles = matches

	// Separate definition files (where the name is defined) from usage files.
	// Heuristic: definitions typically appear with "func ", "type ", "var ", "const " prefix.
	defPattern := fmt.Sprintf(`(?:func|type|var|const)\s+%s\b`, regexp.QuoteMeta(oldName))
	defRe, err := regexp.Compile(defPattern)
	if err != nil {
		return nil, fmt.Errorf("compiling definition pattern: %w", err)
	}

	var defFiles, usageFiles []string
	for _, f := range matches {
		content, readErr := os.ReadFile(f) // #nosec G304 -- path provided by caller via tool/task parameters, inherent to this dev CLI's file operations
		if readErr != nil {
			continue
		}
		if defRe.Match(content) {
			defFiles = append(defFiles, f)
		} else {
			usageFiles = append(usageFiles, f)
		}
	}

	order := 1
	pattern := `\b` + regexp.QuoteMeta(oldName) + `\b`

	// Definitions first.
	for _, f := range defFiles {
		lines := mp.findMatchingLines(f, oldName)
		for _, line := range lines {
			step := MigrationStep{
				Order:       order,
				Description: fmt.Sprintf("%s: rename definition (L%d)", mp.relPath(f), line),
				Pattern:     pattern,
				Replacement: newName,
				Files:       []string{f},
				Manual:      false,
			}
			plan.Steps = append(plan.Steps, step)
			order++
		}
	}

	// Usages second.
	for _, f := range usageFiles {
		lines := mp.findMatchingLines(f, oldName)
		isTest := strings.HasSuffix(f, "_test.go") || strings.Contains(f, "test")
		for _, line := range lines {
			step := MigrationStep{
				Order:       order,
				Description: fmt.Sprintf("%s: update reference (L%d)", mp.relPath(f), line),
				Pattern:     pattern,
				Replacement: newName,
				Files:       []string{f},
				Manual:      isTest,
			}
			if isTest {
				step.Description = fmt.Sprintf("%s: review test names (L%d)", mp.relPath(f), line)
			}
			plan.Steps = append(plan.Steps, step)
			order++
		}
	}

	plan.EstimatedChanges = len(plan.Steps)
	plan.RiskLevel = mp.assessRisk(plan)

	return plan, nil
}

// PlanPatternReplace creates a migration plan to replace all matches of pattern
// in files matching fileGlob with the given replacement.
func (mp *MigrationPlanner) PlanPatternReplace(pattern, replacement, fileGlob string) (*MigrationPlan, error) {
	mp.mu.Lock()
	defer mp.mu.Unlock()

	if pattern == "" {
		return nil, fmt.Errorf("pattern must not be empty")
	}

	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil, fmt.Errorf("invalid pattern %q: %w", pattern, err)
	}

	plan := &MigrationPlan{
		Name:        fmt.Sprintf("Pattern replace: %s -> %s", pattern, replacement),
		Description: fmt.Sprintf("Replace pattern %q with %q in files matching %q", pattern, replacement, fileGlob),
		CreatedAt:   time.Now(),
	}

	files, err := mp.globFiles(fileGlob)
	if err != nil {
		return nil, fmt.Errorf("globbing files: %w", err)
	}

	order := 1
	for _, f := range files {
		content, readErr := os.ReadFile(f) // #nosec G304 -- path provided by caller via tool/task parameters, inherent to this dev CLI's file operations
		if readErr != nil {
			continue
		}
		if re.Match(content) {
			matchCount := len(re.FindAllIndex(content, -1))
			step := MigrationStep{
				Order:       order,
				Description: fmt.Sprintf("%s: replace %d occurrence(s)", mp.relPath(f), matchCount),
				Pattern:     pattern,
				Replacement: replacement,
				Files:       []string{f},
				Manual:      false,
			}
			plan.Steps = append(plan.Steps, step)
			plan.AffectedFiles = append(plan.AffectedFiles, f)
			plan.EstimatedChanges += matchCount
			order++
		}
	}

	plan.RiskLevel = mp.assessRisk(plan)
	return plan, nil
}

// PlanDependencyUpgrade creates a migration plan for upgrading a dependency
// from one version to another.
func (mp *MigrationPlanner) PlanDependencyUpgrade(pkg, fromVersion, toVersion string) (*MigrationPlan, error) {
	mp.mu.Lock()
	defer mp.mu.Unlock()

	if pkg == "" {
		return nil, fmt.Errorf("package name must not be empty")
	}

	plan := &MigrationPlan{
		Name:        fmt.Sprintf("Upgrade %s from %s to %s", pkg, fromVersion, toVersion),
		Description: fmt.Sprintf("Upgrade dependency %q from version %s to %s", pkg, fromVersion, toVersion),
		CreatedAt:   time.Now(),
		RiskLevel:   "MEDIUM",
	}

	order := 1

	// Step 1: Update go.mod if it exists.
	goMod := filepath.Join(mp.ProjectDir, "go.mod")
	if _, err := os.Stat(goMod); err == nil {
		versionPattern := regexp.QuoteMeta(pkg) + `\s+` + regexp.QuoteMeta(fromVersion)
		step := MigrationStep{
			Order:       order,
			Description: "go.mod: update version constraint",
			Pattern:     versionPattern,
			Replacement: pkg + " " + toVersion,
			Files:       []string{goMod},
			Manual:      false,
		}
		plan.Steps = append(plan.Steps, step)
		plan.AffectedFiles = append(plan.AffectedFiles, goMod)
		plan.EstimatedChanges++
		order++
	}

	// Step 2: Update package.json if it exists.
	pkgJSON := filepath.Join(mp.ProjectDir, "package.json")
	if _, err := os.Stat(pkgJSON); err == nil {
		versionPattern := `"` + regexp.QuoteMeta(pkg) + `"\s*:\s*"[^"]*` + regexp.QuoteMeta(fromVersion) + `[^"]*"`
		replacementStr := fmt.Sprintf(`"%s": "%s"`, pkg, toVersion)
		step := MigrationStep{
			Order:       order,
			Description: "package.json: update version",
			Pattern:     versionPattern,
			Replacement: replacementStr,
			Files:       []string{pkgJSON},
			Manual:      false,
		}
		plan.Steps = append(plan.Steps, step)
		plan.AffectedFiles = append(plan.AffectedFiles, pkgJSON)
		plan.EstimatedChanges++
		order++
	}

	// Step 3: Find all import usages of the package for deprecated API detection.
	importFiles, err := mp.findFilesContaining(pkg)
	if err != nil {
		return nil, fmt.Errorf("scanning for package usages: %w", err)
	}

	for _, f := range importFiles {
		// Skip the manifest files already handled.
		if f == goMod || f == pkgJSON {
			continue
		}
		plan.AffectedFiles = append(plan.AffectedFiles, f)
		step := MigrationStep{
			Order:       order,
			Description: fmt.Sprintf("%s: review for breaking changes", mp.relPath(f)),
			Pattern:     regexp.QuoteMeta(pkg),
			Replacement: pkg,
			Files:       []string{f},
			Manual:      true, // breaking changes need human review
		}
		plan.Steps = append(plan.Steps, step)
		plan.EstimatedChanges++
		order++
	}

	// Upgrade risk is always at least MEDIUM due to potential breaking changes.
	if len(importFiles) > 10 {
		plan.RiskLevel = "HIGH"
	}

	return plan, nil
}

// PlanAPIChange creates a migration plan for changing a function/method signature.
func (mp *MigrationPlanner) PlanAPIChange(oldSig, newSig string) (*MigrationPlan, error) {
	mp.mu.Lock()
	defer mp.mu.Unlock()

	if oldSig == "" || newSig == "" {
		return nil, fmt.Errorf("oldSig and newSig must not be empty")
	}

	// Extract function name from signature (first word or up to parenthesis).
	funcName := migrationExtractFuncName(oldSig)
	if funcName == "" {
		return nil, fmt.Errorf("could not extract function name from %q", oldSig)
	}

	plan := &MigrationPlan{
		Name:        fmt.Sprintf("API change: %s", funcName),
		Description: fmt.Sprintf("Change API signature from %q to %q", oldSig, newSig),
		CreatedAt:   time.Now(),
	}

	// Find all files that reference the function.
	matches, err := mp.findFilesContaining(funcName)
	if err != nil {
		return nil, fmt.Errorf("scanning for callers: %w", err)
	}

	plan.AffectedFiles = matches
	pattern := regexp.QuoteMeta(oldSig)

	order := 1
	for _, f := range matches {
		lines := mp.findMatchingLines(f, funcName)
		for _, line := range lines {
			step := MigrationStep{
				Order:       order,
				Description: fmt.Sprintf("%s: update caller (L%d)", mp.relPath(f), line),
				Pattern:     pattern,
				Replacement: newSig,
				Files:       []string{f},
				Manual:      false,
			}
			plan.Steps = append(plan.Steps, step)
			order++
		}
	}

	plan.EstimatedChanges = len(plan.Steps)
	plan.RiskLevel = mp.assessRisk(plan)

	return plan, nil
}

// Preview returns a human-readable preview of the migration plan.
func (mp *MigrationPlanner) Preview(plan *MigrationPlan) string {
	if plan == nil {
		return ""
	}

	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("Migration: %q\n", plan.Name))
	sb.WriteString(fmt.Sprintf("Risk: %s | Files: %d | Changes: %d\n", plan.RiskLevel, len(plan.AffectedFiles), plan.EstimatedChanges))
	sb.WriteString("\nSteps:\n")

	for _, step := range plan.Steps {
		tag := "auto"
		if step.Manual {
			tag = "manual"
		}
		sb.WriteString(fmt.Sprintf("  %d. [%s] %s\n", step.Order, tag, step.Description))
	}

	if len(plan.Steps) == 0 {
		sb.WriteString("  (no steps)\n")
	}

	return sb.String()
}

// Execute applies all auto steps in the plan, skipping manual ones.
func (mp *MigrationPlanner) Execute(plan *MigrationPlan) (*MigrationResult, error) {
	mp.mu.Lock()
	defer mp.mu.Unlock()

	if plan == nil {
		return nil, fmt.Errorf("plan must not be nil")
	}

	result := &MigrationResult{}

	// Sort steps by order.
	sort.Slice(plan.Steps, func(i, j int) bool {
		return plan.Steps[i].Order < plan.Steps[j].Order
	})

	for i := range plan.Steps {
		step := &plan.Steps[i]

		if step.Manual {
			result.Skipped++
			result.ManualReview = append(result.ManualReview, *step)
			continue
		}

		err := mp.executeStep(step)
		if err != nil {
			step.Error = err.Error()
			result.Failed++
		} else {
			step.Completed = true
			result.Completed++
		}
	}

	return result, nil
}

// Validate checks the plan for potential issues and returns a list of warnings.
func (mp *MigrationPlanner) Validate(plan *MigrationPlan) []string {
	mp.mu.Lock()
	defer mp.mu.Unlock()

	if plan == nil {
		return []string{"plan is nil"}
	}

	var warnings []string

	// Check that affected files exist.
	for _, f := range plan.AffectedFiles {
		if _, err := os.Stat(f); os.IsNotExist(err) {
			warnings = append(warnings, fmt.Sprintf("file does not exist: %s", f))
		}
	}

	// Check that patterns have matches in their target files.
	for _, step := range plan.Steps {
		if step.Pattern == "" {
			warnings = append(warnings, fmt.Sprintf("step %d: empty pattern", step.Order))
			continue
		}
		re, err := regexp.Compile(step.Pattern)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("step %d: invalid pattern %q: %v", step.Order, step.Pattern, err))
			continue
		}
		for _, f := range step.Files {
			content, readErr := os.ReadFile(f) // #nosec G304 -- path provided by caller via tool/task parameters, inherent to this dev CLI's file operations
			if readErr != nil {
				warnings = append(warnings, fmt.Sprintf("step %d: cannot read %s: %v", step.Order, f, readErr))
				continue
			}
			if !re.Match(content) {
				warnings = append(warnings, fmt.Sprintf("step %d: pattern has no matches in %s", step.Order, f))
			}
		}
	}

	// Estimate conflict potential: multiple steps targeting the same file.
	fileCounts := make(map[string]int)
	for _, step := range plan.Steps {
		for _, f := range step.Files {
			fileCounts[f]++
		}
	}
	for f, count := range fileCounts {
		if count > 3 {
			warnings = append(warnings, fmt.Sprintf("high conflict potential: %s has %d steps targeting it", mp.relPath(f), count))
		}
	}

	if len(warnings) == 0 {
		plan.Validated = true
	}

	return warnings
}

// Rollback undoes applied steps in reverse order.
func (mp *MigrationPlanner) Rollback(plan *MigrationPlan) error {
	mp.mu.Lock()
	defer mp.mu.Unlock()

	if plan == nil {
		return fmt.Errorf("plan must not be nil")
	}

	// Restore files from backups in reverse step order.
	var completedSteps []MigrationStep
	for _, step := range plan.Steps {
		if step.Completed {
			completedSteps = append(completedSteps, step)
		}
	}

	// Reverse order.
	for i := len(completedSteps) - 1; i >= 0; i-- {
		step := completedSteps[i]
		for _, f := range step.Files {
			backup, ok := mp.backups[f]
			if !ok {
				return fmt.Errorf("no backup found for %s", f)
			}
			if err := fsutil.WritePinnedFile(f, backup, 0o600); err != nil {
				return fmt.Errorf("restoring %s: %w", f, err)
			}
		}
	}

	// Mark all steps as not completed.
	for i := range plan.Steps {
		plan.Steps[i].Completed = false
		plan.Steps[i].Error = ""
	}

	return nil
}

// executeStep applies a single migration step.
func (mp *MigrationPlanner) executeStep(step *MigrationStep) error {
	if step.Pattern == "" {
		return fmt.Errorf("step has empty pattern")
	}

	re, err := regexp.Compile(step.Pattern)
	if err != nil {
		return fmt.Errorf("invalid pattern %q: %w", step.Pattern, err)
	}

	for _, f := range step.Files {
		content, readErr := os.ReadFile(f) // #nosec G304 -- path provided by caller via tool/task parameters, inherent to this dev CLI's file operations
		if readErr != nil {
			return fmt.Errorf("reading %s: %w", f, readErr)
		}

		// Store backup before modification.
		if _, exists := mp.backups[f]; !exists {
			mp.backups[f] = content
		}

		newContent := re.ReplaceAll(content, []byte(step.Replacement))
		if err := fsutil.WritePinnedFile(f, newContent, 0o600); err != nil {
			return fmt.Errorf("writing %s: %w", f, err)
		}
	}

	return nil
}

// findFilesContaining searches project files for those containing the given text.
func (mp *MigrationPlanner) findFilesContaining(text string) ([]string, error) {
	var matches []string

	err := filepath.WalkDir(mp.ProjectDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			base := filepath.Base(path)
			// Skip hidden directories, vendor, node_modules.
			if strings.HasPrefix(base, ".") || base == "vendor" || base == "node_modules" {
				return filepath.SkipDir
			}
			return nil
		}
		// Only scan text-like files.
		if !isTextFile(path) {
			return nil
		}
		content, readErr := fsutil.ReadPinnedFile(path)
		if readErr != nil {
			return nil
		}
		if strings.Contains(string(content), text) {
			matches = append(matches, path)
		}
		return nil
	})

	return matches, err
}

// globFiles returns files matching the given glob pattern relative to the project dir.
func (mp *MigrationPlanner) globFiles(fileGlob string) ([]string, error) {
	if fileGlob == "" {
		// Default: all text files.
		var files []string
		err := filepath.WalkDir(mp.ProjectDir, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				base := filepath.Base(path)
				if strings.HasPrefix(base, ".") || base == "vendor" || base == "node_modules" {
					return filepath.SkipDir
				}
				return nil
			}
			if isTextFile(path) {
				files = append(files, path)
			}
			return nil
		})
		return files, err
	}

	pattern := filepath.Join(mp.ProjectDir, fileGlob)
	return filepath.Glob(pattern)
}

// findMatchingLines returns line numbers in the file that contain the given text.
func (mp *MigrationPlanner) findMatchingLines(filePath, text string) []int {
	f, err := os.Open(filePath) // #nosec G304 -- path provided by caller via tool/task parameters, inherent to this dev CLI's file operations
	if err != nil {
		return nil
	}
	defer func() { _ = f.Close() }()

	var lines []int
	scanner := bufio.NewScanner(f)
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		if strings.Contains(scanner.Text(), text) {
			lines = append(lines, lineNum)
		}
	}
	return lines
}

// relPath returns a path relative to the project directory for display.
func (mp *MigrationPlanner) relPath(path string) string {
	rel, err := filepath.Rel(mp.ProjectDir, path)
	if err != nil {
		return path
	}
	return rel
}

// assessRisk determines risk level based on the number of files and changes.
func (mp *MigrationPlanner) assessRisk(plan *MigrationPlan) string {
	files := len(plan.AffectedFiles)
	changes := plan.EstimatedChanges

	manualCount := 0
	for _, step := range plan.Steps {
		if step.Manual {
			manualCount++
		}
	}

	if files > 20 || changes > 50 || manualCount > 10 {
		return "HIGH"
	}
	if files > 5 || changes > 15 || manualCount > 3 {
		return "MEDIUM"
	}
	return "LOW"
}

// isTextFile returns true if the file appears to be a text file based on extension.
func isTextFile(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	textExts := map[string]bool{
		".go": true, ".js": true, ".ts": true, ".tsx": true, ".jsx": true,
		".py": true, ".rb": true, ".java": true, ".c": true, ".h": true,
		".cpp": true, ".rs": true, ".swift": true, ".kt": true,
		".json": true, ".yaml": true, ".yml": true, ".toml": true,
		".xml": true, ".html": true, ".css": true, ".scss": true,
		".md": true, ".txt": true, ".mod": true, ".sum": true,
		".sh": true, ".bash": true, ".zsh": true,
		".sql": true, ".graphql": true, ".proto": true,
	}
	return textExts[ext]
}

// migrationExtractFuncName extracts the function name from a signature string.
func migrationExtractFuncName(sig string) string {
	sig = strings.TrimSpace(sig)
	// Try to get the name before the first parenthesis.
	if idx := strings.Index(sig, "("); idx > 0 {
		parts := strings.Fields(sig[:idx])
		if len(parts) > 0 {
			return parts[len(parts)-1]
		}
	}
	// Fallback: first word.
	parts := strings.Fields(sig)
	if len(parts) > 0 {
		return parts[0]
	}
	return ""
}
