package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/GrayCodeAI/hawk/internal/mention"
	"github.com/GrayCodeAI/hawk/internal/tool"
)

// handleMentions processes @-prefixed file mentions in user input.
// Mentioned files are read and appended to the session context.
// Returns the cleaned input with mentions resolved.
func (m *chatModel) handleMentions(text string) string {
	if !strings.Contains(text, "@") {
		return text
	}

	cwd, err := os.Getwd()
	if err != nil {
		return text
	}

	result := mention.ParseMentions(text, cwd)
	if len(result.MentionedFiles) == 0 {
		return text
	}

	// Read each mentioned file and append to session context.
	var contextParts []string
	for _, path := range result.MentionedFiles {
		// Enforce the same sensitive-path block the Read/Write tools use.
		// Otherwise `@~/.ssh/id_rsa` or `@/etc/passwd` would sidestep the
		// security boundary and inject secrets into the LLM context.
		if reason := tool.IsSensitivePath(path); reason != "" {
			m.messages = append(m.messages, displayMsg{
				role:    "error",
				content: fmt.Sprintf("Refused to read @%s: %s", path, reason),
			})
			continue
		}
		content, err := os.ReadFile(path) // #nosec G304 -- path is user-mentioned file, already checked against sensitive-path list above
		if err != nil {
			m.messages = append(m.messages, displayMsg{
				role:    "error",
				content: fmt.Sprintf("Could not read @%s: %s", path, err.Error()),
			})
			continue
		}

		// Truncate very large files.
		fileContent := string(content)
		if len(fileContent) > 50000 {
			fileContent = fileContent[:50000] + "\n... (truncated, file too large)"
		}

		contextParts = append(contextParts, fmt.Sprintf("--- File: %s ---\n%s\n--- End of %s ---", path, fileContent, path))
	}

	if len(contextParts) > 0 {
		fileContext := strings.Join(contextParts, "\n\n")
		m.session.AppendSystemContext(fileContext)
		m.messages = append(m.messages, displayMsg{
			role:    "system",
			content: fmt.Sprintf("Added %d file(s) to context: %s", len(contextParts), strings.Join(result.RawMentions, ", ")),
		})
	}

	// Return the clean input without the @ tokens.
	if result.CleanInput != "" {
		return result.CleanInput
	}
	return text
}
