package cmd

import (
	"os"
	"strings"
	"sync"
)

// GhostText provides predictive suggestions shown as dim text after the cursor.
// After the AI responds, it suggests the likely next command.
type GhostText struct {
	mu         sync.Mutex
	suggestion string
	active     bool
}

// followup maps action keywords to likely next commands (ordered, first match wins).
type followup struct {
	keyword string
	cmd     string
}

var commonFollowups = []followup{
	{"fixed", "go test ./..."},
	{"test", "go test ./..."},
	{"refactored", "go test ./..."},
	{"compiled", "./"},
	{"built", "./"},
	{"created", "cat "},
	{"wrote", "cat "},
	{"installed", "go mod tidy"},
	{"added", "git add -p"},
	{"deleted", "git status"},
	{"formatted", "git diff"},
}

// projectFollowups overrides based on detected project type.
var projectFollowups = map[string][]followup{
	"go.mod":       {{"fixed", "go test ./..."}, {"installed", "go mod tidy"}},
	"package.json": {{"fixed", "npm test"}, {"installed", "npm install"}, {"test", "npm test"}},
	"Cargo.toml":   {{"fixed", "cargo test"}, {"built", "cargo build"}, {"test", "cargo test"}},
	"pyproject.toml": {{"fixed", "pytest"}, {"test", "pytest"}, {"installed", "pip install -e ."}},
}

// NewGhostText creates a new ghost text manager.
func NewGhostText() *GhostText {
	return &GhostText{}
}

// Suggest sets a ghost text suggestion based on the AI's last response.
func (g *GhostText) Suggest(aiResponse string) {
	g.mu.Lock()
	defer g.mu.Unlock()

	lower := strings.ToLower(aiResponse)
	g.suggestion = ""
	g.active = false

	// Try project-specific followups first
	for file, followups := range projectFollowups {
		if _, err := os.Stat(file); err == nil {
			for _, f := range followups {
				if strings.Contains(lower, f.keyword) {
					g.suggestion = f.cmd
					g.active = true
					return
				}
			}
		}
	}

	// Fall back to generic followups
	for _, f := range commonFollowups {
		if strings.Contains(lower, f.keyword) {
			g.suggestion = f.cmd
			g.active = true
			return
		}
	}
}

// SuggestExplicit sets an explicit suggestion (e.g., from reroute context).
func (g *GhostText) SuggestExplicit(cmd string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.suggestion = cmd
	g.active = cmd != ""
}

// Get returns the current suggestion, or empty if none.
func (g *GhostText) Get() string {
	g.mu.Lock()
	defer g.mu.Unlock()
	if !g.active {
		return ""
	}
	return g.suggestion
}

// Accept returns the suggestion and clears it (user pressed → or Tab).
func (g *GhostText) Accept() string {
	g.mu.Lock()
	defer g.mu.Unlock()
	s := g.suggestion
	g.suggestion = ""
	g.active = false
	return s
}

// Clear dismisses the current suggestion (user started typing).
func (g *GhostText) Clear() {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.suggestion = ""
	g.active = false
}

// Active reports whether a suggestion is currently showing.
func (g *GhostText) Active() bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.active
}
