package plugin

import (
	"strings"
	"time"
)

// injectSourceMetadata adds source tracking fields to a SKILL.md's frontmatter.
func injectSourceMetadata(content, repo string) string {
	content = strings.TrimSpace(content)
	now := time.Now().UTC().Format(time.RFC3339)

	meta := "source-repo: " + repo + "\nsource-installed-at: " + now

	if !strings.HasPrefix(content, "---") {
		// No frontmatter — wrap content with new frontmatter.
		return "---\n" + meta + "\n---\n" + content
	}

	rest := content[3:]
	idx := strings.Index(rest, "---")
	if idx < 0 {
		return "---\n" + meta + "\n---\n" + content
	}

	frontmatter := rest[:idx]
	body := rest[idx:]

	// Remove existing source lines to avoid duplicates.
	var lines []string
	for _, line := range strings.Split(frontmatter, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "source-repo:") || strings.HasPrefix(trimmed, "source-installed-at:") {
			continue
		}
		lines = append(lines, line)
	}

	// Append source metadata before closing ---.
	return "---" + strings.Join(lines, "\n") + meta + "\n" + body
}
