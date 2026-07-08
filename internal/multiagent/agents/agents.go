package agents

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/GrayCodeAI/hawk/internal/storage"
)

// Agent is a user-defined persona with a custom system prompt.
// Stored as markdown files with YAML frontmatter in Hawk user state.
type Agent struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Model       string `json:"model,omitempty"`
	Prompt      string `json:"prompt"`
	FilePath    string `json:"file_path"`
}

// Load reads an agent definition from a markdown file.
// Format:
//
//	---
//	name: reviewer
//	description: Code review specialist
//	model: inherit
//	---
//	You are a code reviewer...
func Load(path string) (*Agent, error) {
	data, err := os.ReadFile(path) // #nosec G304 -- path is from agentDirs()/ListAll enumeration of Hawk's own persona dir, or an explicit caller-supplied agent file path
	if err != nil {
		return nil, err
	}
	return Parse(string(data), path)
}

// Parse extracts agent metadata and prompt from markdown content.
func Parse(content, filePath string) (*Agent, error) {
	content = strings.TrimSpace(content)
	if !strings.HasPrefix(content, "---") {
		return nil, fmt.Errorf("agent file must start with --- frontmatter")
	}

	// Find closing ---
	rest := content[3:]
	endIdx := strings.Index(rest, "\n---")
	if endIdx < 0 {
		return nil, fmt.Errorf("agent file missing closing --- for frontmatter")
	}

	frontmatter := rest[:endIdx]
	body := strings.TrimSpace(rest[endIdx+4:])

	agent := &Agent{
		Prompt:   body,
		FilePath: filePath,
	}

	for _, line := range strings.Split(frontmatter, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		key, val, ok := parseYAMLLine(line)
		if !ok {
			continue
		}
		switch key {
		case "name":
			agent.Name = val
		case "description":
			agent.Description = val
		case "model":
			if val != "inherit" {
				agent.Model = val
			}
		}
	}

	if agent.Name == "" {
		base := filepath.Base(filePath)
		agent.Name = strings.TrimSuffix(base, filepath.Ext(base))
	}

	return agent, nil
}

// ListAll discovers all agent definitions from Hawk user state.
func ListAll() ([]*Agent, error) {
	var agents []*Agent

	dirs := agentDirs()
	for _, dir := range dirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
				continue
			}
			a, err := Load(filepath.Join(dir, e.Name()))
			if err != nil {
				continue
			}
			agents = append(agents, a)
		}
	}
	return agents, nil
}

// Get finds an agent by name from all known directories.
func Get(name string) (*Agent, error) {
	agents, err := ListAll()
	if err != nil {
		return nil, err
	}
	for _, a := range agents {
		if a.Name == name {
			return a, nil
		}
	}
	return nil, fmt.Errorf("agent %q not found", name)
}

// DefaultDir returns the user's agent directory.
func DefaultDir() string {
	return storage.PersonasDir()
}

// agentDirs returns the list of directories to search for agents.
func agentDirs() []string {
	return []string{DefaultDir()}
}

func parseYAMLLine(line string) (key, val string, ok bool) {
	idx := strings.Index(line, ":")
	if idx < 0 {
		return "", "", false
	}
	key = strings.TrimSpace(line[:idx])
	val = strings.TrimSpace(line[idx+1:])
	// Strip surrounding quotes
	if len(val) >= 2 {
		if (val[0] == '"' && val[len(val)-1] == '"') ||
			(val[0] == '\'' && val[len(val)-1] == '\'') {
			val = val[1 : len(val)-1]
		}
	}
	// Handle multi-line YAML >- by collapsing (simplified)
	val = strings.TrimPrefix(val, ">-")
	val = strings.TrimSpace(val)
	return key, val, true
}
