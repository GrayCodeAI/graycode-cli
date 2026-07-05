package spec

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// Archive archives a completed spec workflow. It merges delta specs into
// main specs, then moves the change directory to archive/.
// Returns the archive path.
func Archive(slug string) (string, error) {
	if slug == "" {
		return "", fmt.Errorf("slug is required")
	}

	dir, err := SpecsRoot()
	if err != nil {
		return "", err
	}
	specDir := filepath.Join(dir, slug)

	// Verify the spec directory exists
	if _, err := os.Stat(specDir); os.IsNotExist(err) {
		return "", fmt.Errorf("spec %q not found at %s", slug, specDir)
	}

	// Check if archive already exists
	meta := LoadStageMeta(slug)
	if meta != nil && meta.Stage == "archived" {
		return "", fmt.Errorf("spec %q is already archived", slug)
	}

	// Run delta merge if there are delta specs or a specs.md file
	specsPath := filepath.Join(specDir, "specs.md")
	if _, err := os.Stat(specsPath); err == nil {
		data, err := os.ReadFile(specsPath)
		if err == nil {
			delta, parseErr := ParseDeltaSpec(string(data))
			if parseErr == nil {
				// Merge into the base spec if it exists
				baseSpecPath := filepath.Join(specDir, "spec.md")
				if _, statErr := os.Stat(baseSpecPath); statErr == nil {
					baseData, readErr := os.ReadFile(baseSpecPath)
					if readErr == nil {
						merged, mergeErr := ApplyDelta(string(baseData), delta)
						if mergeErr == nil {
							_ = os.WriteFile(baseSpecPath, []byte(merged), 0o600)
						}
					}
				}
			}
		}
	}

	// Create archive directory
	archiveDir := filepath.Join(dir, "archive")
	if err := os.MkdirAll(archiveDir, 0o700); err != nil {
		return "", fmt.Errorf("mkdir archive: %w", err)
	}

	// Move to archive with date prefix
	datePrefix := time.Now().Format("2006-01-02")
	archiveName := fmt.Sprintf("%s-%s", datePrefix, slug)
	archivePath := filepath.Join(archiveDir, archiveName)

	if err := os.Rename(specDir, archivePath); err != nil {
		return "", fmt.Errorf("move to archive: %w", err)
	}

	// Update meta to archived (in new location)
	_ = WriteStageMeta(archiveName, "archived", "", "")

	return archivePath, nil
}

// ConvergenceGap classifies a gap between spec and implementation.
type ConvergenceGap struct {
	Description string `json:"description"`
	Category    string `json:"category"` // missing, partial, contradicts, unrequested
	Severity    string `json:"severity"` // critical, high, medium, low
	Source      string `json:"source"`   // which spec requirement or section
}

// ConvergenceReport is the output of a convergence assessment.
type ConvergenceReport struct {
	Converged bool             `json:"converged"`
	Gaps      []ConvergenceGap `json:"gaps,omitempty"`
	Summary   string           `json:"summary"`
}

// AssessConvergence checks if the implementation matches the spec.
// Reads spec/plan/tasks and assesses the codebase for gaps.
func AssessConvergence(slug string) ConvergenceReport {
	dir, err := SpecsRoot()
	if err != nil {
		return ConvergenceReport{
			Summary: fmt.Sprintf("cannot access specs: %v", err),
		}
	}
	specDir := filepath.Join(dir, slug)

	report := ConvergenceReport{Converged: true}

	// Read spec.md to extract requirements
	specContent := readFileOrEmpty(filepath.Join(specDir, "spec.md"))
	if specContent == "" {
		report.Gaps = append(report.Gaps, ConvergenceGap{
			Description: "No spec.md found — cannot assess convergence",
			Category:    "missing",
			Severity:    "critical",
			Source:      "spec.md",
		})
		report.Converged = false
	}

	// Read tasks.md to find incomplete tasks
	tasksContent := readFileOrEmpty(filepath.Join(specDir, "tasks.md"))
	if tasksContent != "" {
		incompleteTasks := countIncompleteTasks(tasksContent)
		if incompleteTasks > 0 {
			report.Gaps = append(report.Gaps, ConvergenceGap{
				Description: fmt.Sprintf("%d task(s) still incomplete in tasks.md", incompleteTasks),
				Category:    "partial",
				Severity:    "high",
				Source:      "tasks.md",
			})
			report.Converged = false
		}
	}

	// Check if plan.md exists
	planPath := filepath.Join(specDir, "plan.md")
	if _, err := os.Stat(planPath); os.IsNotExist(err) {
		report.Gaps = append(report.Gaps, ConvergenceGap{
			Description: "No plan.md found",
			Category:    "missing",
			Severity:    "medium",
			Source:      "plan.md",
		})
	}

	// Check for requirements without validation
	if specContent != "" {
		reqMatches := reRequirement.FindAllStringSubmatch(specContent, -1)
		for _, m := range reqMatches {
			reqName := strings.TrimSpace(m[1])
			// Check if requirement still has unresolved markers
			if reNeedsClarify.MatchString(specContent) {
				report.Gaps = append(report.Gaps, ConvergenceGap{
					Description: fmt.Sprintf("Requirement %q still has unresolved needs-clarification markers", reqName),
					Category:    "partial",
					Severity:    "high",
					Source:      fmt.Sprintf("spec.md → %s", reqName),
				})
				report.Converged = false
			}
		}

		// Check for SHALL/MUST requirements
		if !reSHALLMUST.MatchString(specContent) {
			report.Gaps = append(report.Gaps, ConvergenceGap{
				Description: "No SHALL/MUST requirements found in spec — requirements may not be explicit enough",
				Category:    "partial",
				Severity:    "medium",
				Source:      "spec.md",
			})
		}
	}

	if report.Converged && len(report.Gaps) == 0 {
		report.Summary = "Implementation is converged with the spec — all requirements are addressed."
	} else {
		report.Summary = fmt.Sprintf("Found %d gap(s) — implementation does not fully satisfy the spec.", len(report.Gaps))
	}

	return report
}

// AppendConvergenceTasks appends convergence tasks to tasks.md.
func AppendConvergenceTasks(slug string, report ConvergenceReport) (string, error) {
	if report.Converged {
		return "", nil
	}

	dir, err := SpecsRoot()
	if err != nil {
		return "", err
	}
	tasksPath := filepath.Join(dir, slug, "tasks.md")

	existing := readFileOrEmpty(tasksPath)

	var b strings.Builder
	b.WriteString(existing)
	if existing != "" && !strings.HasSuffix(existing, "\n") {
		b.WriteString("\n")
	}

	b.WriteString(fmt.Sprintf("\n## %d. Convergence Tasks\n\n", countPhases(existing)+1))
	b.WriteString("**Purpose**: Address gaps identified in convergence assessment.\n\n")

	taskNum := countTasks(existing) + 1
	for _, gap := range report.Gaps {
		b.WriteString(fmt.Sprintf("- [ ] T%03d %s — %s\n", taskNum, gap.Description, gap.Severity))
		taskNum++
	}

	content := b.String()
	if err := os.WriteFile(tasksPath, []byte(content), 0o600); err != nil {
		return "", fmt.Errorf("write convergence tasks: %w", err)
	}

	return content, nil
}

// readFileOrEmpty reads a file and returns its content or empty string on error.
func readFileOrEmpty(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(data)
}

var reCheckboxTask = regexp.MustCompile(`(?m)^\s*-\s*\[\s*\]\s+`)

// countIncompleteTasks counts unchecked checkboxes in tasks content.
func countIncompleteTasks(content string) int {
	return len(reCheckboxTask.FindAllString(content, -1))
}

var reCompletedTask = regexp.MustCompile(`(?m)^\s*-\s*\[\s*x\s*\]\s+`)

// countCompletedTasks counts checked checkboxes in tasks content.
func countCompletedTasks(content string) int {
	return len(reCompletedTask.FindAllString(content, -1))
}

var rePhaseHeader = regexp.MustCompile(`(?m)^## \d+\.`)

// countPhases counts the number of phase headers in tasks content.
func countPhases(content string) int {
	return len(rePhaseHeader.FindAllString(content, -1))
}

// countTasks counts the total number of checkbox tasks in tasks content.
func countTasks(content string) int {
	return countIncompleteTasks(content) + countCompletedTasks(content)
}
