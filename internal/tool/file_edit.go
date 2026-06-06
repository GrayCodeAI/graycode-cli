package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

type FileEditTool struct{}

func (FileEditTool) Name() string      { return "Edit" }
func (FileEditTool) RiskLevel() string { return "medium" }
func (FileEditTool) Aliases() []string { return []string{"file_edit"} }
func (FileEditTool) Description() string {
	return "Edit a file by replacing an exact string match with new content."
}

func (FileEditTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"path":       map[string]interface{}{"type": "string", "description": "File path to edit"},
			"file_path":  map[string]interface{}{"type": "string", "description": "Archive-compatible alias for path"},
			"old_str":    map[string]interface{}{"type": "string", "description": "Exact string to find and replace"},
			"old_string": map[string]interface{}{"type": "string", "description": "Archive-compatible alias for old_str"},
			"new_str":    map[string]interface{}{"type": "string", "description": "Replacement string"},
			"new_string": map[string]interface{}{"type": "string", "description": "Archive-compatible alias for new_str"},
		},
	}
}

func (FileEditTool) Execute(ctx context.Context, input json.RawMessage) (string, error) {
	var p struct {
		Path      string  `json:"path"`
		FilePath  string  `json:"file_path"`
		OldStr    string  `json:"old_str"`
		OldString string  `json:"old_string"`
		NewStr    *string `json:"new_str"`
		NewString *string `json:"new_string"`
	}
	if err := json.Unmarshal(input, &p); err != nil {
		return "", err
	}
	path := p.Path
	if path == "" {
		path = p.FilePath
	}
	if path == "" {
		return "", fmt.Errorf("path is required")
	}
	if err := validatePathAllowed(ctx, path); err != nil {
		return "", err
	}
	if reason := IsSensitivePath(path); reason != "" {
		return "", fmt.Errorf("blocked: %s", reason)
	}
	if tc := GetToolContext(ctx); tc != nil && tc.Protected != nil && tc.Protected.IsProtected(path) {
		return "", fmt.Errorf("path %s is protected (read-only)", path)
	}
	oldStr := p.OldStr
	if oldStr == "" {
		oldStr = p.OldString
	}
	newStr := ""
	if p.NewStr != nil {
		newStr = *p.NewStr
	} else if p.NewString != nil {
		newStr = *p.NewString
	}

	info, err := os.Stat(path)
	if err != nil {
		suggestion := suggestSimilar(path)
		if suggestion != "" {
			return "", fmt.Errorf("file not found: %s\nDid you mean: %s", path, suggestion)
		}
		return "", fmt.Errorf("file not found: %s", path)
	}
	if info.Size() > maxFileSize {
		return "", fmt.Errorf("file too large: %d bytes", info.Size())
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read %s: %w", path, err)
	}
	content := string(data)

	// Detect line endings
	lineEnding := "\n"
	if strings.Contains(content, "\r\n") {
		lineEnding = "\r\n"
	}

	count := strings.Count(content, oldStr)
	fuzzyNote := ""
	if count == 0 {
		// Try fuzzy matching fallback
		matched, matchedStr, similarity := fuzzyFind(content, oldStr)
		if !matched {
			return "", fmt.Errorf("old_str not found in %s", path)
		}
		oldStr = matchedStr
		fuzzyNote = fmt.Sprintf(" (Applied via fuzzy match — %.0f%% similarity)", similarity*100)
		count = 1
	}
	if count > 1 {
		return "", fmt.Errorf("old_str found %d times in %s — must be unique", count, path)
	}
	if cred := DetectCredentials(newStr); cred != "" {
		return "", fmt.Errorf("new_str contains a credential (%s) — refusing to edit", cred)
	}

	result := strings.Replace(content, oldStr, newStr, 1)
	if _, backupErr := BackupFile(path); backupErr != nil {
		return "", fmt.Errorf("backup failed (refusing destructive write): %w", backupErr)
	}

	// Preserve line endings
	if lineEnding == "\r\n" {
		result = strings.ReplaceAll(result, "\r\n", "\n")
		result = strings.ReplaceAll(result, "\n", "\r\n")
	}

	if err := os.WriteFile(path, []byte(result), info.Mode()); err != nil {
		return "", fmt.Errorf("write: %w", err)
	}
	if autoCommitEnabled(ctx) {
		_ = AutoCommit(ctx, path, "Edit", "edited file")
	}
	lintNote := postWriteLint(ctx, path)
	return fmt.Sprintf("Edited %s (replaced 1 occurrence)%s%s", path, fuzzyNote, lintNote), nil
}

// --- Fuzzy matching helpers ---

// fuzzyFind attempts to find oldStr in content using progressively fuzzier strategies.
// Returns (matched, actualStringInContent, similarityScore).
func fuzzyFind(content, oldStr string) (bool, string, float64) {
	// Strategy 1: Whitespace-normalized matching
	if matched, actual := whitespaceNormalizedFind(content, oldStr); matched {
		return true, actual, 1.0
	}

	// Strategy 2: Leading-whitespace adjustment (indentation differences)
	if matched, actual := leadingWhitespaceFind(content, oldStr); matched {
		return true, actual, 1.0
	}

	// Strategy 3: Levenshtein-based similarity matching on contiguous line blocks
	if matched, actual, sim := levenshteinBlockFind(content, oldStr, 0.90); matched {
		return true, actual, sim
	}

	return false, "", 0
}

// normalizeWhitespace collapses runs of spaces and tabs into a single space.
func normalizeWhitespace(s string) string {
	var b strings.Builder
	inSpace := false
	for _, r := range s {
		if r == ' ' || r == '\t' {
			if !inSpace {
				b.WriteRune(' ')
				inSpace = true
			}
		} else {
			b.WriteRune(r)
			inSpace = false
		}
	}
	return b.String()
}

// whitespaceNormalizedFind normalizes whitespace in both content and oldStr,
// finds the match in normalized content, then maps back to the original.
func whitespaceNormalizedFind(content, oldStr string) (bool, string) {
	normOld := normalizeWhitespace(oldStr)
	if normOld == oldStr {
		// No whitespace difference, skip this strategy
		return false, ""
	}

	normContent := normalizeWhitespace(content)
	idx := strings.Index(normContent, normOld)
	if idx == -1 {
		return false, ""
	}
	// Verify uniqueness in the normalized domain
	if strings.Count(normContent, normOld) != 1 {
		return false, ""
	}

	// Map normalized byte offsets back to original byte offsets.
	// Walk content and normContent in parallel to find the original
	// positions corresponding to the normalized match boundaries.
	endNorm := idx + len(normOld)
	normBytePos := 0
	origStart := -1
	origEnd := -1
	inSpace := false
	for i, r := range content {
		if normBytePos == idx && origStart == -1 {
			origStart = i
		}
		if normBytePos == endNorm {
			origEnd = i
			break
		}
		if r == ' ' || r == '\t' {
			if !inSpace {
				normBytePos++ // one space byte in normalized output
				inSpace = true
			}
			// Additional whitespace chars don't advance normBytePos
		} else {
			runeLen := len(string(r))
			normBytePos += runeLen
			inSpace = false
		}
	}
	if origStart == -1 {
		return false, ""
	}
	if origEnd == -1 {
		// Match extends to end of content
		origEnd = len(content)
	}

	actual := content[origStart:origEnd]
	return true, actual
}

// leadingWhitespaceFind tries matching by ignoring leading whitespace on each line.
func leadingWhitespaceFind(content, oldStr string) (bool, string) {
	oldLines := strings.Split(oldStr, "\n")
	contentLines := strings.Split(content, "\n")

	if len(oldLines) == 0 || len(contentLines) < len(oldLines) {
		return false, ""
	}

	// Trim leading whitespace for comparison
	trimmedOld := make([]string, len(oldLines))
	for i, l := range oldLines {
		trimmedOld[i] = strings.TrimLeft(l, " \t")
	}

	var matchStart int = -1
	for i := 0; i <= len(contentLines)-len(oldLines); i++ {
		found := true
		for j, tl := range trimmedOld {
			contentTrimmed := strings.TrimLeft(contentLines[i+j], " \t")
			if contentTrimmed != tl {
				found = false
				break
			}
		}
		if found {
			if matchStart != -1 {
				// Multiple matches — not unique
				return false, ""
			}
			matchStart = i
		}
	}

	if matchStart == -1 {
		return false, ""
	}

	// Reconstruct the actual string from content lines
	actual := strings.Join(contentLines[matchStart:matchStart+len(oldLines)], "\n")
	return true, actual
}

// levenshteinBlockFind finds the most similar contiguous block of lines in content.
func levenshteinBlockFind(content, oldStr string, threshold float64) (bool, string, float64) {
	oldLines := strings.Split(oldStr, "\n")
	contentLines := strings.Split(content, "\n")
	numOldLines := len(oldLines)

	if numOldLines == 0 || len(contentLines) < numOldLines {
		return false, "", 0
	}

	bestSimilarity := 0.0
	bestStart := -1
	bestEnd := -1

	// Try blocks of exactly the same line count, and also +/- 1 line
	for delta := 0; delta <= 1; delta++ {
		for _, blockLen := range []int{numOldLines + delta, numOldLines - delta} {
			if blockLen <= 0 || blockLen > len(contentLines) {
				continue
			}
			for i := 0; i <= len(contentLines)-blockLen; i++ {
				candidate := strings.Join(contentLines[i:i+blockLen], "\n")
				maxLen := len(oldStr)
				if len(candidate) > maxLen {
					maxLen = len(candidate)
				}
				if maxLen == 0 {
					continue
				}
				dist := levenshteinDistance(candidate, oldStr)
				similarity := 1.0 - float64(dist)/float64(maxLen)
				if similarity > bestSimilarity {
					bestSimilarity = similarity
					bestStart = i
					bestEnd = i + blockLen
				}
			}
		}
	}

	if bestSimilarity >= threshold && bestStart >= 0 {
		actual := strings.Join(contentLines[bestStart:bestEnd], "\n")
		// Verify the actual string exists uniquely in content
		if strings.Count(content, actual) != 1 {
			return false, "", 0
		}
		return true, actual, bestSimilarity
	}

	return false, "", 0
}
