package spec

import (
	"bufio"
	"fmt"
	"regexp"
	"strings"
)

// ApplyDelta applies a delta spec to the main spec content and returns
// the merged result.
func ApplyDelta(mainSpec string, delta *DeltaSpec) (string, error) {
	if len(delta.Requirements) == 0 {
		return mainSpec, nil
	}

	result := mainSpec

	for _, req := range delta.Requirements {
		switch req.Section {
		case DeltaAdded:
			result = appendAdded(result, req)
		case DeltaModified:
			result = replaceModified(result, req)
		case DeltaRemoved:
			result = deleteRemoved(result, req)
		case DeltaRenamed:
			result = applyRename(result, req)
		}
	}

	return result, nil
}

// appendAdded adds a new requirement to the main spec.
func appendAdded(content string, req DeltaRequirement) string {
	block := renderRequirementBlock(req)

	// If content is empty, start fresh
	if strings.TrimSpace(content) == "" {
		return "# Requirements\n\n" + block
	}

	// Find the ADDED Requirements section or create one
	addedSection := regexp.MustCompile(`(?m)^## ADDED Requirements$`)
	if addedSection.MatchString(content) {
		// Append to existing ADDED section (before next ## or end)
		return appendAfterLastMatch(content, `(?m)^## ADDED Requirements`, "\n"+block)
	}

	// No ADDED section — find existing requirements section and add
	if strings.Contains(content, "## Requirements") {
		return appendAfterLastMatch(content, `(?m)^## Requirements`, "\n\n### ADDED\n"+block)
	}

	// No sections at all — append
	return content + "\n\n## ADDED Requirements\n" + block
}

// replaceModified finds the existing requirement by name and replaces its
// entire block (header + body + scenarios).
func replaceModified(content string, req DeltaRequirement) string {
	return replaceRequirementBlock(content, req.Name, renderRequirementBlock(req))
}

// deleteRemoved removes the requirement block entirely.
func deleteRemoved(content string, req DeltaRequirement) string {
	return removeRequirementBlock(content, req.Name)
}

// applyRename renames an existing requirement header.
func applyRename(content string, req DeltaRequirement) string {
	oldName := regexp.QuoteMeta(req.OldName)
	newName := req.NewName
	re := regexp.MustCompile(`(?m)^### Requirement: ` + oldName + `$`)
	return re.ReplaceAllString(content, "### Requirement: "+newName)
}

// renderRequirementBlock renders a delta requirement as markdown.
func renderRequirementBlock(req DeltaRequirement) string {
	var b strings.Builder

	fmt.Fprintf(&b, "### Requirement: %s\n", req.Name)
	if req.Description != "" {
		b.WriteString(req.Description + "\n")
	}

	for _, sc := range req.Scenarios {
		fmt.Fprintf(&b, "#### Scenario: %s\n", sc.Name)
		if sc.When != "" {
			fmt.Fprintf(&b, "- **WHEN** %s\n", sc.When)
		}
		if sc.Then != "" {
			fmt.Fprintf(&b, "- **THEN** %s\n", sc.Then)
		}
		b.WriteString("\n")
	}

	return strings.TrimRight(b.String(), "\n")
}

// replaceRequirementBlock replaces the full block of a named requirement.
func replaceRequirementBlock(content, reqName, newBlock string) string {
	scanner := bufio.NewScanner(strings.NewReader(content))
	var lines []string
	inTarget := false
	found := false
	targetPattern := "### Requirement: " + reqName

	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)

		if !inTarget {
			if trimmed == targetPattern {
				inTarget = true
				found = true
				continue
			}
			lines = append(lines, line)
		} else {
			// skip lines until next ## heading or next ### Requirement
			if strings.HasPrefix(trimmed, "## ") {
				inTarget = false
				lines = append(lines, "")
				lines = append(lines, newBlock)
				lines = append(lines, "")
				lines = append(lines, line)
			} else if strings.HasPrefix(trimmed, "### ") {
				inTarget = false
				lines = append(lines, "")
				lines = append(lines, newBlock)
				lines = append(lines, "")
				lines = append(lines, line)
			}
		}
	}

	if inTarget {
		// Requirement was at end of file
		lines = append(lines, "")
		lines = append(lines, newBlock)
	}

	if !found {
		// Requirement not found — append it
		lines = append(lines, "")
		lines = append(lines, newBlock)
	}

	return strings.Join(lines, "\n")
}

// removeRequirementBlock removes the full block of a named requirement.
func removeRequirementBlock(content, reqName string) string {
	scanner := bufio.NewScanner(strings.NewReader(content))
	var lines []string
	inTarget := false
	targetPattern := "### Requirement: " + reqName

	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)

		if !inTarget {
			if trimmed == targetPattern {
				inTarget = true
				continue
			}
			lines = append(lines, line)
		} else {
			// Skip until next heading of same level or higher
			if strings.HasPrefix(trimmed, "## ") || strings.HasPrefix(trimmed, "### ") {
				inTarget = false
				lines = append(lines, line)
			}
		}
	}

	return strings.Join(lines, "\n")
}

// appendAfterLastMatch appends text after the last occurrence of a pattern.
func appendAfterLastMatch(content, pattern, text string) string {
	re := regexp.MustCompile(pattern)
	locs := re.FindAllStringIndex(content, -1)
	if len(locs) == 0 {
		return content + text
	}
	lastLoc := locs[len(locs)-1]
	// Find the end of the matched line
	afterMatch := content[lastLoc[1]:]
	// Find the start of the next section or end of content
	nextSection := regexp.MustCompile(`(?m)^## `)
	nextLoc := nextSection.FindStringIndex(afterMatch)
	if nextLoc != nil && nextLoc[0] > 0 {
		insertPoint := lastLoc[1] + nextLoc[0]
		return content[:insertPoint] + text + content[insertPoint:]
	}
	return content + text
}

// MergeDeltaSpecs merges multiple delta specs into one combined delta spec.
func MergeDeltaSpecs(deltas []*DeltaSpec) *DeltaSpec {
	merged := &DeltaSpec{}
	seen := make(map[string]bool)

	for _, d := range deltas {
		for _, req := range d.Requirements {
			key := fmt.Sprintf("%s:%s", req.Section, req.Name)
			if seen[key] {
				continue
			}
			seen[key] = true
			merged.Requirements = append(merged.Requirements, req)
		}
	}

	return merged
}
