package spec

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// ValidationLevel represents the severity of a validation issue.
type ValidationLevel int

const (
	ValidationInfo    ValidationLevel = iota // informative observation
	ValidationWarning                        // potential quality concern
	ValidationError                          // must fix to proceed
)

func (vl ValidationLevel) String() string {
	switch vl {
	case ValidationInfo:
		return "info"
	case ValidationWarning:
		return "warning"
	case ValidationError:
		return "error"
	default:
		return "unknown"
	}
}

// ValidationIssue represents a single validation finding.
type ValidationIssue struct {
	Level   ValidationLevel `json:"level"`
	Path    string          `json:"path,omitempty"`   // which artifact file
	Line    int             `json:"line,omitempty"`   // approximate line
	Column  int             `json:"column,omitempty"` // approximate column
	Code    string          `json:"code"`             // diagnostic code (e.g., "NO_SHALL_MUST")
	Message string          `json:"message"`          // human-readable description
}

// ValidationResult is the output of a validation pass.
type ValidationResult struct {
	Valid  bool              `json:"valid"`
	Issues []ValidationIssue `json:"issues"`
}

func (vr ValidationResult) ErrorCount() int {
	count := 0
	for _, iss := range vr.Issues {
		if iss.Level == ValidationError {
			count++
		}
	}
	return count
}

func (vr ValidationResult) WarningCount() int {
	count := 0
	for _, iss := range vr.Issues {
		if iss.Level == ValidationWarning {
			count++
		}
	}
	return count
}

// Format returns a human-readable validation report.
func (vr ValidationResult) Format() string {
	var b strings.Builder
	if vr.Valid {
		b.WriteString("+ Validation passed\n")
	} else {
		b.WriteString("x Validation failed\n")
	}
	fmt.Fprintf(&b, "  %d errors, %d warnings\n", vr.ErrorCount(), vr.WarningCount())
	for _, iss := range vr.Issues {
		icon := "i"
		switch iss.Level {
		case ValidationError:
			icon = "x"
		case ValidationWarning:
			icon = "!"
		}
		loc := ""
		if iss.Path != "" {
			loc = iss.Path
			if iss.Line > 0 {
				loc = fmt.Sprintf("%s:%d", loc, iss.Line)
			}
			loc = " [" + loc + "]"
		}
		fmt.Fprintf(&b, "  %s [%s]%s %s\n", icon, iss.Code, loc, iss.Message)
	}
	return strings.TrimRight(b.String(), "\n")
}

var (
	reSuccessCriteriaMetric = regexp.MustCompile(`(?i)\b(under\s+\d+|less\s+than\s+\d+|within\s+\d+|\d+\s*%|\d+\s*ms|\d+\s*s|\d+\s*req/s)\b`)
	reSectionHeader         = regexp.MustCompile(`(?m)^## \S+`)
	reNoImplementation      = regexp.MustCompile(`(?i)implementation details|tech stack|language:|framework:|database:`)
	reUserValue             = regexp.MustCompile(`(?i)user|stakeholder|customer|business|value|benefit`)
	reEdgeCases             = regexp.MustCompile(`(?i)edge case|error|failure|boundary|limit|exception|fallback`)
)

// ValidateSpec validates the quality of a spec document.
func ValidateSpec(content string) ValidationResult {
	var issues []ValidationIssue

	if strings.TrimSpace(content) == "" {
		return ValidationResult{
			Issues: []ValidationIssue{{
				Level:   ValidationError,
				Code:    "EMPTY_SPEC",
				Message: "spec content is empty",
			}},
		}
	}

	// Check for implementation details in spec
	if reNoImplementation.MatchString(content) {
		issues = append(issues, ValidationIssue{
			Level:   ValidationWarning,
			Code:    "IMPLEMENTATION_DETAILS",
			Message: "spec should focus on WHAT and WHY, not HOW — avoid tech stack, frameworks, and implementation details",
		})
	}

	// Check for user value and business needs
	if !reUserValue.MatchString(content) {
		issues = append(issues, ValidationIssue{
			Level:   ValidationWarning,
			Code:    "NO_USER_VALUE",
			Message: "spec should articulate user value or business benefit explicitly",
		})
	}

	// Check for measurable success criteria
	if !reSuccessCriteriaMetric.MatchString(content) {
		issues = append(issues, ValidationIssue{
			Level:   ValidationWarning,
			Code:    "NO_MEASURABLE_CRITERIA",
			Message: "success criteria should be measurable (time, percentage, count, rate)",
		})
	}

	// Check for [NEEDS CLARIFICATION] markers
	clarifyMatches := reNeedsClarify.FindAllString(content, -1)
	clarifyCount := len(clarifyMatches)
	for _, m := range clarifyMatches {
		issues = append(issues, ValidationIssue{
			Level:   ValidationInfo,
			Code:    "NEEDS_CLARIFICATION",
			Message: fmt.Sprintf("unresolved marker: %s", m),
		})
	}
	if clarifyCount > 3 {
		issues = append(issues, ValidationIssue{
			Level:   ValidationWarning,
			Code:    "TOO_MANY_CLARIFICATIONS",
			Message: fmt.Sprintf("max 3 [NEEDS CLARIFICATION] markers recommended, found %d", clarifyCount),
		})
	}

	// Check that sections are present
	hasObj := hasSection(content, "Objective", "objective")
	hasReq := hasSection(content, "Requirements", "requirement")
	if !hasObj && !hasReq {
		issues = append(issues, ValidationIssue{
			Level:   ValidationInfo,
			Code:    "MISSING_OBJECTIVE",
			Message: "consider adding an Objective section explaining what is being built and why",
		})
	}

	// Check for edge cases consideration
	if !reEdgeCases.MatchString(content) {
		issues = append(issues, ValidationIssue{
			Level:   ValidationInfo,
			Code:    "NO_EDGE_CASES",
			Message: "consider documenting edge cases and error scenarios",
		})
	}

	// Check for scope boundaries
	if !strings.Contains(content, "out of scope") && !strings.Contains(content, "boundar") && !strings.Contains(content, "non-goal") {
		issues = append(issues, ValidationIssue{
			Level:   ValidationInfo,
			Code:    "NO_BOUNDARIES",
			Message: "consider defining scope boundaries (what is explicitly in/out of scope)",
		})
	}

	valid := true
	for _, iss := range issues {
		if iss.Level == ValidationError {
			valid = false
			break
		}
	}

	return ValidationResult{Valid: valid, Issues: issues}
}

// ValidatePlan validates the quality of a plan document.
func ValidatePlan(content string) ValidationResult {
	var issues []ValidationIssue

	if strings.TrimSpace(content) == "" {
		return ValidationResult{
			Issues: []ValidationIssue{{
				Level:   ValidationError,
				Code:    "EMPTY_PLAN",
				Message: "plan content is empty",
			}},
		}
	}

	// Check for summary
	if !hasSection(content, "Summary", "Overview") {
		issues = append(issues, ValidationIssue{
			Level:   ValidationWarning,
			Code:    "MISSING_SUMMARY",
			Message: "plan should include a summary or overview section",
		})
	}

	// Check for technical approach / architecture decisions
	if !containsAny(content, "decision", "approach", "architecture", "design") {
		issues = append(issues, ValidationIssue{
			Level:   ValidationWarning,
			Code:    "MISSING_DECISIONS",
			Message: "plan should document key design decisions with rationale",
		})
	}

	// Check for risks
	if !containsAny(content, "risk", "concern", "challenge", "trade-off") {
		issues = append(issues, ValidationIssue{
			Level:   ValidationInfo,
			Code:    "NO_RISKS",
			Message: "consider documenting risks and mitigations",
		})
	}

	valid := true
	for _, iss := range issues {
		if iss.Level == ValidationError {
			valid = false
			break
		}
	}

	return ValidationResult{Valid: valid, Issues: issues}
}

// ValidateTasks validates the quality of a tasks document.
func ValidateTasks(content string) ValidationResult {
	var issues []ValidationIssue

	if strings.TrimSpace(content) == "" {
		return ValidationResult{
			Issues: []ValidationIssue{{
				Level:   ValidationError,
				Code:    "EMPTY_TASKS",
				Message: "tasks content is empty",
			}},
		}
	}

	// Check for checkbox format
	if !strings.Contains(content, "- [ ]") {
		issues = append(issues, ValidationIssue{
			Level:   ValidationWarning,
			Code:    "NO_CHECKBOXES",
			Message: "tasks should use `- [ ]` checkbox format for trackable progress",
		})
	}

	// Check for numbered task groups
	if !reSectionHeader.MatchString(content) {
		issues = append(issues, ValidationIssue{
			Level:   ValidationInfo,
			Code:    "NO_SECTIONS",
			Message: "consider organizing tasks under ## numbered headings",
		})
	}

	valid := true
	for _, iss := range issues {
		if iss.Level == ValidationError {
			valid = false
			break
		}
	}

	return ValidationResult{Valid: valid, Issues: issues}
}

// ValidateSpecFile validates a spec file on disk.
func ValidateSpecFile(path string) ValidationResult {
	data, err := os.ReadFile(path)
	if err != nil {
		return ValidationResult{
			Issues: []ValidationIssue{{
				Level:   ValidationError,
				Code:    "READ_ERROR",
				Message: fmt.Sprintf("cannot read %s: %v", path, err),
			}},
		}
	}
	result := ValidateSpec(string(data))
	for i := range result.Issues {
		if result.Issues[i].Path == "" {
			result.Issues[i].Path = filepath.Base(path)
		}
	}
	return result
}

// ValidateDirectory validates all spec artifacts in a directory.
// Checks spec.md, plan.md, and tasks.md if they exist.
func ValidateDirectory(slug string) ValidationResult {
	dir, err := SpecsRoot()
	if err != nil {
		return ValidationResult{
			Issues: []ValidationIssue{{
				Level:   ValidationError,
				Code:    "NO_SPECS_DIR",
				Message: fmt.Sprintf("cannot access specs directory: %v", err),
			}},
		}
	}
	specDir := filepath.Join(dir, slug)

	var allIssues []ValidationResult
	for _, f := range []string{"spec.md", "plan.md", "tasks.md"} {
		path := filepath.Join(specDir, f)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			continue
		}
		var result ValidationResult
		switch f {
		case "spec.md":
			result = ValidateSpecFile(path)
		case "plan.md":
			data, _ := os.ReadFile(path)
			result = ValidatePlan(string(data))
			for i := range result.Issues {
				if result.Issues[i].Path == "" {
					result.Issues[i].Path = "plan.md"
				}
			}
		case "tasks.md":
			data, _ := os.ReadFile(path)
			result = ValidateTasks(string(data))
			for i := range result.Issues {
				if result.Issues[i].Path == "" {
					result.Issues[i].Path = "tasks.md"
				}
			}
		}
		allIssues = append(allIssues, result)
	}

	if len(allIssues) == 0 {
		return ValidationResult{Valid: true}
	}

	var combined []ValidationIssue
	valid := true
	for _, r := range allIssues {
		combined = append(combined, r.Issues...)
		if !r.Valid {
			valid = false
		}
	}

	return ValidationResult{Valid: valid, Issues: combined}
}

// hasSection checks if content contains a section header matching any of the given names.
func hasSection(content string, names ...string) bool {
	lower := strings.ToLower(content)
	for _, name := range names {
		// Check for markdown section header (## name) or plain text
		if strings.Contains(lower, "## "+strings.ToLower(name)) {
			return true
		}
	}
	return false
}

// containsAny checks if content contains any of the given substrings (case-insensitive).
func containsAny(content string, substrs ...string) bool {
	lower := strings.ToLower(content)
	for _, s := range substrs {
		if strings.Contains(lower, s) {
			return true
		}
	}
	return false
}
