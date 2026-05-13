package permissions

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"
)

// InputSanitizer cleans and validates all inputs before they reach the LLM,
// preventing injection, encoding attacks, and malformed data.
type InputSanitizer struct {
	MaxLength        int
	StripInvisible   bool
	NormalizeUnicode bool
	mu               sync.RWMutex
}

// SanitizeResult holds the outcome of sanitizing an input string.
type SanitizeResult struct {
	Clean       string
	Original    string
	Changes     []SanitizeChange
	WasModified bool
}

// SanitizeChange describes a single modification made during sanitization.
type SanitizeChange struct {
	Type        string // "stripped", "normalized", "truncated", "escaped"
	Position    int
	Original    string
	Replacement string
}

// NewInputSanitizer creates an InputSanitizer with sensible defaults.
func NewInputSanitizer() *InputSanitizer {
	return &InputSanitizer{
		MaxLength:        100000,
		StripInvisible:   true,
		NormalizeUnicode: true,
	}
}

// ansiPattern matches ANSI escape sequences.
var ansiPattern = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)

// dangerousJSONKeys lists keys that could be used for prototype pollution.
var dangerousJSONKeys = []string{"__proto__", "constructor", "prototype"}

// buildInvisibleRunes returns the map of invisible Unicode code points.
func buildInvisibleRunes() map[rune]string {
	return map[rune]string{
		0x200B: "zero-width space",
		0x200C: "zero-width non-joiner",
		0x200D: "zero-width joiner",
		0xFEFF: "BOM",
		0x200E: "left-to-right mark",
		0x200F: "right-to-left mark",
		0x202A: "left-to-right embedding",
		0x202B: "right-to-left embedding",
		0x202C: "pop directional formatting",
		0x202D: "left-to-right override",
		0x202E: "right-to-left override",
		0x2060: "word joiner",
		0x2061: "function application",
		0x2062: "invisible times",
		0x2063: "invisible separator",
		0x2064: "invisible plus",
		0x2066: "left-to-right isolate",
		0x2067: "right-to-left isolate",
		0x2068: "first strong isolate",
		0x2069: "pop directional isolate",
		0x00AD: "soft hyphen",
		0x034F: "combining grapheme joiner",
		0x061C: "Arabic letter mark",
		0x115F: "hangul choseong filler",
		0x1160: "hangul jungseong filler",
		0x17B4: "Khmer vowel inherent Aq",
		0x17B5: "Khmer vowel inherent Aa",
		0x180E: "Mongolian vowel separator",
		0x2028: "line separator",
		0x2029: "paragraph separator",
	}
}

// invisibleRunes contains Unicode code points that are invisible/control characters.
var invisibleRunes = buildInvisibleRunes()

// cyrillicToLatin maps Cyrillic homoglyphs to their Latin equivalents.
var cyrillicToLatin = map[rune]rune{
	0x0430: 'a', // Cyrillic a -> Latin a
	0x0435: 'e', // Cyrillic e -> Latin e
	0x043E: 'o', // Cyrillic o -> Latin o
	0x0440: 'p', // Cyrillic r -> Latin p
	0x0441: 'c', // Cyrillic s -> Latin c
	0x0443: 'y', // Cyrillic u -> Latin y
	0x0445: 'x', // Cyrillic h -> Latin x
	0x0410: 'A', // Cyrillic A -> Latin A
	0x0412: 'B', // Cyrillic V -> Latin B
	0x0415: 'E', // Cyrillic Ye -> Latin E
	0x041A: 'K', // Cyrillic Ka -> Latin K
	0x041C: 'M', // Cyrillic Em -> Latin M
	0x041D: 'H', // Cyrillic En -> Latin H
	0x041E: 'O', // Cyrillic O -> Latin O
	0x0420: 'P', // Cyrillic Er -> Latin P
	0x0421: 'C', // Cyrillic Es -> Latin C
	0x0422: 'T', // Cyrillic Te -> Latin T
	0x0425: 'X', // Cyrillic Ha -> Latin X
}

// Sanitize applies all sanitization steps to the input and returns a detailed result.
func (s *InputSanitizer) Sanitize(input string) *SanitizeResult {
	s.mu.RLock()
	maxLen := s.MaxLength
	stripInvis := s.StripInvisible
	normalizeUni := s.NormalizeUnicode
	s.mu.RUnlock()

	result := &SanitizeResult{
		Original: input,
		Changes:  make([]SanitizeChange, 0),
	}

	current := input

	// Strip null bytes
	if strings.ContainsRune(current, '\x00') {
		var changes []SanitizeChange
		cleaned := strings.Builder{}
		pos := 0
		for _, r := range current {
			if r == '\x00' {
				changes = append(changes, SanitizeChange{
					Type:        "stripped",
					Position:    pos,
					Original:    "\\x00",
					Replacement: "",
				})
			} else {
				cleaned.WriteRune(r)
			}
			pos += utf8.RuneLen(r)
		}
		current = cleaned.String()
		result.Changes = append(result.Changes, changes...)
	}

	// Normalize line endings
	if strings.Contains(current, "\r\n") {
		count := strings.Count(current, "\r\n")
		current = strings.ReplaceAll(current, "\r\n", "\n")
		result.Changes = append(result.Changes, SanitizeChange{
			Type:        "normalized",
			Position:    -1,
			Original:    "\\r\\n",
			Replacement: fmt.Sprintf("\\n (x%d)", count),
		})
	}
	// Strip lone \r as well
	if strings.ContainsRune(current, '\r') {
		current = strings.ReplaceAll(current, "\r", "\n")
		result.Changes = append(result.Changes, SanitizeChange{
			Type:        "normalized",
			Position:    -1,
			Original:    "\\r",
			Replacement: "\\n",
		})
	}

	// Strip invisible chars
	if stripInvis {
		cleaned, changes := StripInvisibleChars(current)
		if len(changes) > 0 {
			current = cleaned
			result.Changes = append(result.Changes, changes...)
		}
	}

	// Normalize homoglyphs
	if normalizeUni {
		cleaned, changes := NormalizeHomoglyphs(current)
		if len(changes) > 0 {
			current = cleaned
			result.Changes = append(result.Changes, changes...)
		}
	}

	// Strip ANSI escape sequences
	if ansiPattern.MatchString(current) {
		locs := ansiPattern.FindAllStringIndex(current, -1)
		for _, loc := range locs {
			result.Changes = append(result.Changes, SanitizeChange{
				Type:        "stripped",
				Position:    loc[0],
				Original:    current[loc[0]:loc[1]],
				Replacement: "",
			})
		}
		current = ansiPattern.ReplaceAllString(current, "")
	}

	// Truncate if over max length
	if len(current) > maxLen {
		result.Changes = append(result.Changes, SanitizeChange{
			Type:        "truncated",
			Position:    maxLen,
			Original:    fmt.Sprintf("(%d bytes)", len(current)),
			Replacement: fmt.Sprintf("(%d bytes)", maxLen),
		})
		current = current[:maxLen]
	}

	result.Clean = current
	result.WasModified = current != input

	return result
}

// StripInvisibleChars removes invisible Unicode characters from text.
// This includes zero-width space/joiner/non-joiner, BOM markers,
// bidirectional overrides, invisible separators, and tag characters.
func StripInvisibleChars(text string) (string, []SanitizeChange) {
	var changes []SanitizeChange
	var cleaned strings.Builder
	cleaned.Grow(len(text))

	pos := 0
	for _, r := range text {
		if _, isInvisible := invisibleRunes[r]; isInvisible {
			changes = append(changes, SanitizeChange{
				Type:        "stripped",
				Position:    pos,
				Original:    fmt.Sprintf("U+%04X", r),
				Replacement: "",
			})
		} else if r >= 0xE0001 && r <= 0xE007F {
			// Tag characters (U+E0001 to U+E007F)
			changes = append(changes, SanitizeChange{
				Type:        "stripped",
				Position:    pos,
				Original:    fmt.Sprintf("U+%05X", r),
				Replacement: "",
			})
		} else {
			cleaned.WriteRune(r)
		}
		pos += utf8.RuneLen(r)
	}

	return cleaned.String(), changes
}

// NormalizeHomoglyphs detects mixed Latin+Cyrillic scripts and replaces
// Cyrillic lookalikes with Latin equivalents. Pure Cyrillic text is left alone.
func NormalizeHomoglyphs(text string) (string, []SanitizeChange) {
	if !isMixedScript(text) {
		return text, nil
	}

	var changes []SanitizeChange
	var cleaned strings.Builder
	cleaned.Grow(len(text))

	pos := 0
	for _, r := range text {
		if latin, ok := cyrillicToLatin[r]; ok {
			changes = append(changes, SanitizeChange{
				Type:        "normalized",
				Position:    pos,
				Original:    string(r),
				Replacement: string(latin),
			})
			cleaned.WriteRune(latin)
		} else {
			cleaned.WriteRune(r)
		}
		pos += utf8.RuneLen(r)
	}

	return cleaned.String(), changes
}

// isMixedScript checks whether text contains both Latin and Cyrillic characters.
func isMixedScript(text string) bool {
	hasLatin := false
	hasCyrillic := false
	for _, r := range text {
		if unicode.Is(unicode.Latin, r) {
			hasLatin = true
		}
		if unicode.Is(unicode.Cyrillic, r) {
			hasCyrillic = true
		}
		if hasLatin && hasCyrillic {
			return true
		}
	}
	return false
}

// StripANSI removes ANSI escape sequences from text.
func StripANSI(text string) string {
	return ansiPattern.ReplaceAllString(text, "")
}

// DetectEncoding determines the encoding category of text.
// Returns "utf8", "ascii", "binary", or "mixed".
func DetectEncoding(text string) string {
	if len(text) == 0 {
		return "ascii"
	}

	hasHighBit := false
	hasInvalidUTF8 := false
	hasNull := false

	for i := 0; i < len(text); {
		if text[i] == 0 {
			hasNull = true
			i++
			continue
		}
		if text[i] < 128 {
			i++
			continue
		}
		hasHighBit = true
		r, size := utf8.DecodeRuneInString(text[i:])
		if r == utf8.RuneError && size == 1 {
			hasInvalidUTF8 = true
		}
		i += size
	}

	if hasNull {
		return "binary"
	}
	if hasInvalidUTF8 {
		return "mixed"
	}
	if hasHighBit {
		return "utf8"
	}
	return "ascii"
}

// SanitizeFilePath validates and normalizes a file path, preventing traversal attacks.
func SanitizeFilePath(path string) (string, error) {
	if path == "" {
		return "", fmt.Errorf("empty path")
	}

	// Strip null bytes
	if strings.ContainsRune(path, '\x00') {
		return "", fmt.Errorf("path contains null bytes")
	}

	// Normalize separators (backslash to forward slash)
	cleaned := strings.ReplaceAll(path, "\\", "/")

	// Clean the path (resolves ../ sequences)
	cleaned = filepath.Clean(cleaned)

	// Check for path traversal after cleaning
	if strings.Contains(cleaned, "..") {
		return "", fmt.Errorf("path traversal detected: %q", path)
	}

	// Reject paths that start with traversal
	if strings.HasPrefix(cleaned, "../") || cleaned == ".." {
		return "", fmt.Errorf("path traversal detected: %q", path)
	}

	return cleaned, nil
}

// SanitizeJSON removes potentially dangerous keys from JSON input.
// It strips __proto__, constructor, and prototype keys to prevent prototype pollution.
func SanitizeJSON(input string) string {
	result := input
	for _, key := range dangerousJSONKeys {
		result = removeDangerousKey(result, key)
	}

	// Clean up any trailing commas before closing braces/brackets
	trailingComma := regexp.MustCompile(`,\s*([}\]])`)
	result = trailingComma.ReplaceAllString(result, "$1")

	return result
}

// removeDangerousKey removes a specific key and its value from JSON text.
// Handles string values, nested objects, arrays, and primitive values.
func removeDangerousKey(input, key string) string {
	target := `"` + key + `"`
	for {
		idx := strings.Index(input, target)
		if idx == -1 {
			return input
		}

		// Find the colon after the key
		afterKey := idx + len(target)
		colonIdx := afterKey
		for colonIdx < len(input) && input[colonIdx] != ':' {
			colonIdx++
		}
		if colonIdx >= len(input) {
			return input
		}

		// Skip whitespace after colon
		valStart := colonIdx + 1
		for valStart < len(input) && (input[valStart] == ' ' || input[valStart] == '\t' || input[valStart] == '\n') {
			valStart++
		}
		if valStart >= len(input) {
			return input
		}

		// Determine end of value
		var valEnd int
		switch input[valStart] {
		case '"':
			// String value - find closing quote
			valEnd = valStart + 1
			for valEnd < len(input) && input[valEnd] != '"' {
				if input[valEnd] == '\\' {
					valEnd++ // skip escaped character
				}
				valEnd++
			}
			if valEnd < len(input) {
				valEnd++ // include closing quote
			}
		case '{', '[':
			// Object or array - find matching closing bracket
			opener := input[valStart]
			var closer byte
			if opener == '{' {
				closer = '}'
			} else {
				closer = ']'
			}
			depth := 1
			valEnd = valStart + 1
			for valEnd < len(input) && depth > 0 {
				switch input[valEnd] {
				case opener:
					depth++
				case closer:
					depth--
				case '"':
					// Skip strings inside nested structures
					valEnd++
					for valEnd < len(input) && input[valEnd] != '"' {
						if input[valEnd] == '\\' {
							valEnd++
						}
						valEnd++
					}
				}
				valEnd++
			}
		default:
			// Primitive value (number, boolean, null)
			valEnd = valStart
			for valEnd < len(input) && input[valEnd] != ',' && input[valEnd] != '}' && input[valEnd] != ']' {
				valEnd++
			}
		}

		// Determine what to remove including surrounding comma/whitespace
		removeStart := idx
		removeEnd := valEnd

		// Check for trailing comma and whitespace
		trailIdx := removeEnd
		for trailIdx < len(input) && (input[trailIdx] == ' ' || input[trailIdx] == '\t' || input[trailIdx] == '\n') {
			trailIdx++
		}
		if trailIdx < len(input) && input[trailIdx] == ',' {
			removeEnd = trailIdx + 1
			// Also skip whitespace after the comma
			for removeEnd < len(input) && (input[removeEnd] == ' ' || input[removeEnd] == '\t' || input[removeEnd] == '\n') {
				removeEnd++
			}
		} else {
			// No trailing comma - check for leading comma
			leadIdx := removeStart - 1
			for leadIdx >= 0 && (input[leadIdx] == ' ' || input[leadIdx] == '\t' || input[leadIdx] == '\n') {
				leadIdx--
			}
			if leadIdx >= 0 && input[leadIdx] == ',' {
				removeStart = leadIdx
			}
		}

		input = input[:removeStart] + input[removeEnd:]
	}
}

// FormatChanges produces a human-readable summary of all sanitization changes.
func FormatChanges(result *SanitizeResult) string {
	if len(result.Changes) == 0 {
		return "Input clean, no changes needed."
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Input sanitized (%d changes):\n", len(result.Changes)))

	// Group changes by type for cleaner output
	stripped := groupByType(result.Changes, "stripped")
	normalized := groupByType(result.Changes, "normalized")
	truncated := groupByType(result.Changes, "truncated")

	if len(stripped) > 0 {
		// Subgroup stripped changes
		zwChars := filterByPrefix(stripped, "U+")
		ansiChars := filterByContent(stripped, "\x1b")
		nullChars := filterByContent(stripped, "\\x00")
		otherStripped := filterOther(stripped, zwChars, ansiChars, nullChars)

		if len(zwChars) > 0 {
			positions := getPositions(zwChars)
			sb.WriteString(fmt.Sprintf("  - Stripped %d zero-width characters at positions %s\n",
				len(zwChars), joinInts(positions)))
		}
		if len(nullChars) > 0 {
			positions := getPositions(nullChars)
			sb.WriteString(fmt.Sprintf("  - Stripped %d null bytes at positions %s\n",
				len(nullChars), joinInts(positions)))
		}
		if len(ansiChars) > 0 {
			positions := getPositions(ansiChars)
			sb.WriteString(fmt.Sprintf("  - Removed ANSI escape sequence at position%s %s\n",
				pluralS(len(ansiChars)), joinInts(positions)))
		}
		if len(otherStripped) > 0 {
			positions := getPositions(otherStripped)
			sb.WriteString(fmt.Sprintf("  - Stripped %d characters at positions %s\n",
				len(otherStripped), joinInts(positions)))
		}
	}

	if len(normalized) > 0 {
		// Separate homoglyph normalizations from line ending normalizations
		homoglyphs := filterHomoglyphs(normalized)
		lineEndings := filterLineEndings(normalized)

		if len(homoglyphs) > 0 {
			for _, ch := range homoglyphs {
				sb.WriteString(fmt.Sprintf("  - Normalized 1 Cyrillic homoglyph ('%s' -> '%s') at position %d\n",
					ch.Original, ch.Replacement, ch.Position))
			}
		}
		if len(lineEndings) > 0 {
			sb.WriteString("  - Normalized line endings\n")
		}
	}

	if len(truncated) > 0 {
		for _, ch := range truncated {
			sb.WriteString(fmt.Sprintf("  - Truncated from %s to %s\n",
				ch.Original, ch.Replacement))
		}
	}

	return strings.TrimRight(sb.String(), "\n")
}

// Helper functions for FormatChanges

func groupByType(changes []SanitizeChange, typ string) []SanitizeChange {
	var result []SanitizeChange
	for _, c := range changes {
		if c.Type == typ {
			result = append(result, c)
		}
	}
	return result
}

func filterByPrefix(changes []SanitizeChange, prefix string) []SanitizeChange {
	var result []SanitizeChange
	for _, c := range changes {
		if strings.HasPrefix(c.Original, prefix) {
			result = append(result, c)
		}
	}
	return result
}

func filterByContent(changes []SanitizeChange, content string) []SanitizeChange {
	var result []SanitizeChange
	for _, c := range changes {
		if strings.Contains(c.Original, content) {
			result = append(result, c)
		}
	}
	return result
}

func filterOther(all []SanitizeChange, groups ...[]SanitizeChange) []SanitizeChange {
	seen := make(map[int]bool)
	for _, group := range groups {
		for _, c := range group {
			seen[c.Position] = true
		}
	}
	var result []SanitizeChange
	for _, c := range all {
		if !seen[c.Position] {
			result = append(result, c)
		}
	}
	return result
}

func filterHomoglyphs(changes []SanitizeChange) []SanitizeChange {
	var result []SanitizeChange
	for _, c := range changes {
		if c.Position >= 0 && len(c.Original) > 0 && len(c.Replacement) > 0 &&
			!strings.HasPrefix(c.Original, "\\") {
			result = append(result, c)
		}
	}
	return result
}

func filterLineEndings(changes []SanitizeChange) []SanitizeChange {
	var result []SanitizeChange
	for _, c := range changes {
		if strings.HasPrefix(c.Original, "\\r") {
			result = append(result, c)
		}
	}
	return result
}

func getPositions(changes []SanitizeChange) []int {
	positions := make([]int, 0, len(changes))
	for _, c := range changes {
		positions = append(positions, c.Position)
	}
	return positions
}

func joinInts(ints []int) string {
	parts := make([]string, len(ints))
	for i, v := range ints {
		parts[i] = fmt.Sprintf("%d", v)
	}
	return strings.Join(parts, ", ")
}

func pluralS(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}
