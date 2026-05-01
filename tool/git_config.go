package tool

import (
	"bufio"
	"os"
	"strings"
)

// GitConfig represents a parsed .git/config file. Keys are stored as
// "section.key" or "section.subsection.key".
type GitConfig map[string]map[string]string

// ParseGitConfig reads and parses a git config file without spawning git.
// It handles: [section], [section "subsection"], key = value, and comments.
func ParseGitConfig(path string) (GitConfig, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	config := make(GitConfig)
	currentSection := ""

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}

		// Section header
		if strings.HasPrefix(line, "[") {
			currentSection = parseSectionHeader(line)
			if _, ok := config[currentSection]; !ok {
				config[currentSection] = make(map[string]string)
			}
			continue
		}

		// Key-value pair
		if currentSection != "" {
			key, value := parseKeyValue(line)
			if key != "" {
				config[currentSection][key] = value
			}
		}
	}

	return config, scanner.Err()
}

// parseSectionHeader parses [section] or [section "subsection"] headers.
func parseSectionHeader(line string) string {
	// Remove brackets
	line = strings.TrimPrefix(line, "[")
	line = strings.TrimSuffix(line, "]")
	line = strings.TrimSpace(line)

	// Check for subsection: [section "subsection"]
	if idx := strings.Index(line, "\""); idx >= 0 {
		section := strings.TrimSpace(line[:idx])
		rest := line[idx+1:]
		if endIdx := strings.Index(rest, "\""); endIdx >= 0 {
			subsection := rest[:endIdx]
			return section + "." + subsection
		}
		return section
	}

	return line
}

// parseKeyValue parses "key = value" lines. Also handles "key=value" and bare
// "key" (treated as boolean true).
func parseKeyValue(line string) (string, string) {
	// Remove inline comments
	for _, commentChar := range []string{" #", " ;", "\t#", "\t;"} {
		if idx := strings.Index(line, commentChar); idx >= 0 {
			line = line[:idx]
		}
	}
	line = strings.TrimSpace(line)

	if idx := strings.Index(line, "="); idx >= 0 {
		key := strings.TrimSpace(line[:idx])
		value := strings.TrimSpace(line[idx+1:])
		// Strip surrounding quotes from value
		if len(value) >= 2 && value[0] == '"' && value[len(value)-1] == '"' {
			value = value[1 : len(value)-1]
		}
		return strings.ToLower(key), value
	}

	// Bare key (boolean true)
	return strings.ToLower(strings.TrimSpace(line)), "true"
}
