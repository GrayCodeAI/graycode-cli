package plugin

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/GrayCodeAI/hawk/internal/fsutil"
)

// DiscoveredComponents is the result of scanning a multi-component plugin package.
type DiscoveredComponents struct {
	Root       string
	Skills     []string // absolute paths to SKILL.md parent dirs
	HookFiles  []string // absolute paths under hooks/
	MCPServers []MCPServerSpec
	HasTools   bool // plugin.json tools or tools/ present
}

// DiscoverComponents scans a plugin directory for tools, hooks, skills, and MCP.
// Layout (convention):
//
//	plugin/
//	  plugin.json          # required
//	  tools/               # optional tool binaries/scripts
//	  hooks/               # optional hook scripts
//	  skills/<name>/SKILL.md
//	  mcp.json             # optional MCP server definitions
func DiscoverComponents(pluginDir string) (DiscoveredComponents, error) {
	out := DiscoveredComponents{Root: pluginDir}

	m, err := ParseManifestV2(pluginDir)
	if err != nil {
		// Still try convention scan without manifest
		m = &ManifestV2{}
	} else {
		out.HasTools = len(m.Tools) > 0
	}

	skillsDir := "skills"
	hooksDir := "hooks"
	mcpFile := "mcp.json"
	if m.Components != nil {
		if m.Components.SkillsDir != "" {
			skillsDir = m.Components.SkillsDir
		}
		if m.Components.HooksDir != "" {
			hooksDir = m.Components.HooksDir
		}
		if m.Components.MCPFile != "" {
			mcpFile = m.Components.MCPFile
		}
		out.MCPServers = append(out.MCPServers, m.Components.MCP...)
	}

	// tools/
	if st, err := os.Stat(filepath.Join(pluginDir, "tools")); err == nil && st.IsDir() {
		out.HasTools = true
	}

	// skills/
	skillsRoot := filepath.Join(pluginDir, skillsDir)
	if m.Components != nil && len(m.Components.Skills) > 0 {
		for _, name := range m.Components.Skills {
			p := filepath.Join(skillsRoot, name)
			if skillMD := filepath.Join(p, "SKILL.md"); fileExists(skillMD) {
				out.Skills = append(out.Skills, p)
			}
		}
	} else {
		entries, err := os.ReadDir(skillsRoot)
		if err == nil {
			for _, e := range entries {
				if !e.IsDir() {
					continue
				}
				p := filepath.Join(skillsRoot, e.Name())
				if fileExists(filepath.Join(p, "SKILL.md")) {
					out.Skills = append(out.Skills, p)
				}
			}
		}
	}

	// hooks/
	hooksRoot := filepath.Join(pluginDir, hooksDir)
	_ = filepath.WalkDir(hooksRoot, func(path string, d os.DirEntry, err error) error {
		if err != nil || d == nil || d.IsDir() {
			return nil
		}
		name := d.Name()
		if strings.HasSuffix(name, ".sh") || strings.HasSuffix(name, ".md") || strings.HasSuffix(name, ".json") {
			out.HookFiles = append(out.HookFiles, path)
		}
		return nil
	})

	// mcp.json
	mcpPath := filepath.Join(pluginDir, mcpFile)
	if data, err := fsutil.ReadPinnedFile(mcpPath); err == nil {
		var file struct {
			Servers []MCPServerSpec `json:"servers"`
			// also accept map form { "servers": { "name": {...} } }
			ServerMap map[string]MCPServerSpec `json:"mcpServers"`
		}
		if json.Unmarshal(data, &file) == nil {
			out.MCPServers = append(out.MCPServers, file.Servers...)
			for name, spec := range file.ServerMap {
				if spec.Name == "" {
					spec.Name = name
				}
				out.MCPServers = append(out.MCPServers, spec)
			}
		}
	}

	return out, nil
}

// ComponentSummary returns a short human-readable description of components.
func (d DiscoveredComponents) ComponentSummary() string {
	var parts []string
	if d.HasTools {
		parts = append(parts, "tools")
	}
	if len(d.HookFiles) > 0 {
		parts = append(parts, "hooks")
	}
	if len(d.Skills) > 0 {
		parts = append(parts, "skills")
	}
	if len(d.MCPServers) > 0 {
		parts = append(parts, "mcp")
	}
	if len(parts) == 0 {
		return "empty"
	}
	return strings.Join(parts, "+")
}

func fileExists(path string) bool {
	st, err := os.Stat(path)
	return err == nil && !st.IsDir()
}
