package engine

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/GrayCodeAI/hawk/internal/storage"
)

// CodingSoul defines the persistent coding personality and style preferences.
// Loaded from Hawk user state.
type CodingSoul struct {
	Style       string // communication style
	Preferences string // coding preferences
	Path        string
}

// DefaultSoulPath returns the path to the soul file.
func DefaultSoulPath() string {
	return filepath.Join(storage.StateDir(), "soul.md")
}

// LoadCodingSoul reads the soul file. Returns empty soul if not found.
func LoadCodingSoul() *CodingSoul {
	path := DefaultSoulPath()
	data, err := os.ReadFile(path)
	if err != nil {
		return &CodingSoul{Path: path}
	}
	content := string(data)
	soul := &CodingSoul{Path: path}

	// Parse sections
	sections := strings.Split(content, "##")
	for _, sec := range sections {
		sec = strings.TrimSpace(sec)
		if strings.HasPrefix(sec, "Style") {
			soul.Style = strings.TrimSpace(strings.TrimPrefix(sec, "Style"))
		} else if strings.HasPrefix(sec, "Preferences") {
			soul.Preferences = strings.TrimSpace(strings.TrimPrefix(sec, "Preferences"))
		}
	}
	return soul
}

// ForPrompt formats the soul as system prompt context.
func (s *CodingSoul) ForPrompt() string {
	if s.Style == "" && s.Preferences == "" {
		return ""
	}
	var parts []string
	if s.Style != "" {
		parts = append(parts, "## Communication Style\n"+s.Style)
	}
	if s.Preferences != "" {
		parts = append(parts, "## Coding Preferences\n"+s.Preferences)
	}
	return strings.Join(parts, "\n\n")
}

// InitSoulPrompt returns a prompt to generate an initial soul.md.
func InitSoulPrompt() string {
	return `Generate a coding soul profile for Hawk user state based on my coding patterns. Analyze my recent code and infer:

## Style
- How I communicate (terse vs verbose, formal vs casual)
- How I like explanations (show code vs explain concepts)

## Preferences
- Naming conventions I use
- Error handling patterns I prefer
- Testing style (TDD, after-the-fact, minimal)
- Code organization (flat vs nested, small files vs large)
- Comments style (minimal, docstrings, inline)

Keep it concise — 10-15 bullet points total. This will be loaded on every session.`
}
