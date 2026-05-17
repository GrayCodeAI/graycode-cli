package tool

import (
	"embed"
	"strings"
)

//go:embed descriptions/*.md
var descriptionFS embed.FS

// LoadDescription loads a tool description from embedded markdown.
// Falls back to the provided default if no file exists.
func LoadDescription(toolName, fallback string) string {
	data, err := descriptionFS.ReadFile("descriptions/" + toolName + ".md")
	if err != nil {
		return fallback
	}
	return strings.TrimSpace(string(data))
}
