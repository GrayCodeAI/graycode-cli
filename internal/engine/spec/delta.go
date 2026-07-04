package spec

import (
	"fmt"
	"regexp"
	"strings"
)

// DeltaSection identifies the type of delta operation.
type DeltaSection int

const (
	DeltaAdded    DeltaSection = iota // ## ADDED Requirements
	DeltaModified                     // ## MODIFIED Requirements
	DeltaRemoved                      // ## REMOVED Requirements
	DeltaRenamed                      // ## RENAMED Requirements
)

func (ds DeltaSection) String() string {
	switch ds {
	case DeltaAdded:
		return "ADDED"
	case DeltaModified:
		return "MODIFIED"
	case DeltaRemoved:
		return "REMOVED"
	case DeltaRenamed:
		return "RENAMED"
	default:
		return "UNKNOWN"
	}
}

// Scenario represents a testable scenario for a requirement.
type Scenario struct {
	Name string `json:"name"`
	When string `json:"when"`
	Then string `json:"then"`
}

// DeltaRequirement represents a single requirement within a delta section.
type DeltaRequirement struct {
	Name        string       `json:"name"`
	Description string       `json:"description,omitempty"` // body text after header
	Scenarios   []Scenario   `json:"scenarios,omitempty"`   // for ADDED/MODIFIED
	Reason      string       `json:"reason,omitempty"`      // for REMOVED
	Migration   string       `json:"migration,omitempty"`   // for REMOVED
	OldName     string       `json:"old_name,omitempty"`    // for RENAMED (FROM)
	NewName     string       `json:"new_name,omitempty"`    // for RENAMED (TO)
	Section     DeltaSection `json:"section"`
}

// DeltaSpec represents a parsed delta specification file.
type DeltaSpec struct {
	ID           string             `json:"id"` // capability name (directory)
	Requirements []DeltaRequirement `json:"requirements"`
}

var (
	reDeltaSection = regexp.MustCompile(`(?m)^## (ADDED|MODIFIED|REMOVED|RENAMED) Requirements$`)
	reRequirement  = regexp.MustCompile(`(?m)^### Requirement: (.+)$`)
	reScenario     = regexp.MustCompile(`(?m)^#### Scenario: (.+)$`)
	reWhen         = regexp.MustCompile(`(?m)^-\s*\*\*WHEN\*\*\s+(.+)$`)
	reThen         = regexp.MustCompile(`(?m)^-\s*\*\*THEN\*\*\s+(.+)$`)
	reReason       = regexp.MustCompile(`(?m)^\*\*Reason\*\*:\s*(.+)$`)
	reMigration    = regexp.MustCompile(`(?m)^\*\*Migration\*\*:\s*(.+)$`)
	reRenamedFrom  = regexp.MustCompile(`(?m)^-\s*FROM:\s*[""]?(.+?)[""]?\s*$`)
	reRenamedTo    = regexp.MustCompile(`(?m)^-\s*TO:\s*[""]?(.+?)[""]?\s*$`)
	reSHALLMUST    = regexp.MustCompile(`(?i)\b(SHALL|MUST)\b`)
	reNeedsClarify = regexp.MustCompile(`\[NEEDS CLARIFICATION.*?\]`)
)

// ParseDeltaSpec parses delta spec markdown content.
func ParseDeltaSpec(content string) (*DeltaSpec, error) {
	ds := &DeltaSpec{}
	sectionMap := map[string]DeltaSection{
		"ADDED":    DeltaAdded,
		"MODIFIED": DeltaModified,
		"REMOVED":  DeltaRemoved,
		"RENAMED":  DeltaRenamed,
	}

	// Find section boundaries
	sectionMatches := reDeltaSection.FindAllStringSubmatchIndex(content, -1)
	if len(sectionMatches) == 0 {
		return nil, fmt.Errorf("no delta section headers found (need ## ADDED/MODIFIED/REMOVED/RENAMED Requirements)")
	}

	for i, match := range sectionMatches {
		sectionName := content[match[2]:match[3]]
		section, ok := sectionMap[sectionName]
		if !ok {
			continue
		}

		// Determine section content boundaries
		start := match[1]
		end := len(content)
		if i+1 < len(sectionMatches) {
			end = sectionMatches[i+1][0]
		}
		sectionContent := content[start:end]

		// Parse requirements within this section
		reqMatches := reRequirement.FindAllStringSubmatchIndex(sectionContent, -1)
		for j, rm := range reqMatches {
			reqName := sectionContent[rm[2]:rm[3]]
			reqStart := rm[1]
			reqEnd := len(sectionContent)
			if j+1 < len(reqMatches) {
				reqEnd = reqMatches[j+1][0]
			}
			reqBody := sectionContent[reqStart:reqEnd]

			dr := DeltaRequirement{
				Name:    strings.TrimSpace(reqName),
				Section: section,
			}

			switch section {
			case DeltaAdded, DeltaModified:
				dr.Description = extractDescription(reqBody)
				dr.Scenarios = parseScenarios(reqBody)
			case DeltaRemoved:
				if m := reReason.FindStringSubmatch(reqBody); len(m) > 1 {
					dr.Reason = strings.TrimSpace(m[1])
				}
				if m := reMigration.FindStringSubmatch(reqBody); len(m) > 1 {
					dr.Migration = strings.TrimSpace(m[1])
				}
			case DeltaRenamed:
				if m := reRenamedFrom.FindStringSubmatch(reqBody); len(m) > 1 {
					dr.OldName = strings.TrimSpace(m[1])
				}
				if m := reRenamedTo.FindStringSubmatch(reqBody); len(m) > 1 {
					dr.NewName = strings.TrimSpace(m[1])
				}
			}

			ds.Requirements = append(ds.Requirements, dr)
		}
	}

	if len(ds.Requirements) == 0 {
		return nil, fmt.Errorf("delta spec has section headers but no requirements parsed")
	}

	return ds, nil
}

// ValidateDeltaSpec checks delta spec structural quality.
func ValidateDeltaSpec(ds *DeltaSpec) ValidationResult {
	var issues []ValidationIssue

	namesSeen := make(map[string]DeltaSection)
	var crossSectionErrors []string

	for _, req := range ds.Requirements {
		if req.Name == "" {
			issues = append(issues, ValidationIssue{
				Level:   ValidationError,
				Code:    "EMPTY_REQUIREMENT_NAME",
				Message: "requirement with empty name found",
			})
			continue
		}

		// Check for duplicate names within same section
		if prevSection, exists := namesSeen[req.Name]; exists {
			if prevSection == req.Section {
				issues = append(issues, ValidationIssue{
					Level:   ValidationError,
					Code:    "DUPLICATE_REQUIREMENT",
					Message: fmt.Sprintf("requirement %q appears twice in %s section", req.Name, req.Section),
				})
			}
		}
		namesSeen[req.Name] = req.Section

		switch req.Section {
		case DeltaAdded, DeltaModified:
			// SHALL/MUST check
			if !reSHALLMUST.MatchString(req.Description) {
				issues = append(issues, ValidationIssue{
					Level:   ValidationError,
					Code:    "NO_SHALL_MUST",
					Message: fmt.Sprintf("requirement %q: body must contain SHALL or MUST (RFC 2119)", req.Name),
				})
			}
			// Scenario check
			if len(req.Scenarios) == 0 {
				issues = append(issues, ValidationIssue{
					Level:   ValidationWarning,
					Code:    "NO_SCENARIOS",
					Message: fmt.Sprintf("requirement %q: each requirement should have at least one scenario", req.Name),
				})
			}
			// [NEEDS CLARIFICATION] check
			if reNeedsClarify.MatchString(req.Description) {
				issues = append(issues, ValidationIssue{
					Level:   ValidationInfo,
					Code:    "NEEDS_CLARIFICATION",
					Message: fmt.Sprintf("requirement %q: contains unresolved [NEEDS CLARIFICATION] marker", req.Name),
				})
			}

		case DeltaRenamed:
			if req.OldName == "" {
				issues = append(issues, ValidationIssue{
					Level:   ValidationError,
					Code:    "RENAME_MISSING_FROM",
					Message: fmt.Sprintf("RENAMED requirement %q: missing FROM field", req.Name),
				})
			}
			if req.NewName == "" {
				issues = append(issues, ValidationIssue{
					Level:   ValidationError,
					Code:    "RENAME_MISSING_TO",
					Message: fmt.Sprintf("RENAMED requirement %q: missing TO field", req.Name),
				})
			}
		}

		// Cross-section conflict detection
		for _, other := range ds.Requirements {
			if &other == &req {
				continue
			}
			if other.Name == req.Name && other.Section != req.Section {
				conflict := fmt.Sprintf("requirement %q appears in both %s and %s", req.Name, req.Section, other.Section)
				crossSectionErrors = append(crossSectionErrors, conflict)
			}
		}
	}

	for _, err := range uniqueStrings(crossSectionErrors) {
		issues = append(issues, ValidationIssue{
			Level:   ValidationError,
			Code:    "CROSS_SECTION_CONFLICT",
			Message: err,
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

// parseScenarios extracts scenario blocks from requirement body text.
func parseScenarios(body string) []Scenario {
	scMatches := reScenario.FindAllStringSubmatchIndex(body, -1)
	var scenarios []Scenario
	for i, sm := range scMatches {
		scName := body[sm[2]:sm[3]]
		start := sm[1]
		end := len(body)
		if i+1 < len(scMatches) {
			end = scMatches[i+1][0]
		}
		scContent := body[start:end]

		s := Scenario{Name: strings.TrimSpace(scName)}
		if m := reWhen.FindStringSubmatch(scContent); len(m) > 1 {
			s.When = strings.TrimSpace(m[1])
		}
		if m := reThen.FindStringSubmatch(scContent); len(m) > 1 {
			s.Then = strings.TrimSpace(m[1])
		}
		scenarios = append(scenarios, s)
	}
	return scenarios
}

// extractDescription gets the text after the requirement header, before scenarios.
func extractDescription(body string) string {
	// Remove the requirement header line
	lines := strings.Split(body, "\n")
	var descLines []string
	inHeader := true
	for _, line := range lines {
		if inHeader {
			if strings.HasPrefix(strings.TrimSpace(line), "### Requirement:") {
				inHeader = false
			}
			continue
		}
		// Stop at scenario header or next requirement
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "#### Scenario:") || strings.HasPrefix(trimmed, "### Requirement:") {
			break
		}
		descLines = append(descLines, line)
	}
	return strings.TrimSpace(strings.Join(descLines, "\n"))
}

// uniqueStrings deduplicates a string slice.
func uniqueStrings(s []string) []string {
	seen := make(map[string]bool)
	var result []string
	for _, v := range s {
		if !seen[v] {
			seen[v] = true
			result = append(result, v)
		}
	}
	return result
}
