// Package providers catalogs the AI coding agents that hawk can be
// invoked from. The matrix is ported from JuliusBrussee/caveman's
// bin/install.js (PROVIDERS array, 34 entries) and adapted to native Go.
//
// Each entry in the matrix describes:
//   - id: short kebab-case identifier
//   - label: human-readable name
//   - mech: install mechanism (e.g. "npx skills add (claude)")
//   - profile: vercel-labs/skills slug (for npx-based installs)
//   - detect: detection clause spec ("command:foo" or
//     "dir:$HOME/.foo" or "vscode-ext:foo" or "macapp:Name" or
//     "jetbrains-plugin:foo"); clauses are OR-separated by "||"
//   - soft: optional; true means detection is best-effort, the
//     provider is excluded from auto-detect and only installable
//     via explicit opt-in
//
// The Detected field is populated by Detect() when a probe succeeds
// (e.g. the command is on PATH, the directory exists, etc.).
//
// Source: github.com/JuliusBrussee/caveman, bin/install.js.
// Ported to native Go.
package providers

import (
	"bufio"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
)

// Mech describes how a provider is installed.
type Mech string

const (
	MechClaudePlugin     Mech = "claude plugin install"
	MechGeminiExtensions Mech = "gemini extensions install"
	MechOpenCodePlugin   Mech = "native opencode plugin"
	MechOpenClawSkill    Mech = "workspace skill + SOUL.md"
	MechNpxSkillsAdd     Mech = "npx skills add"
)

// ProbeKind identifies a single detection clause kind.
type ProbeKind string

const (
	ProbeCommand         ProbeKind = "command"          // command:<name>   — binary on PATH
	ProbeDir             ProbeKind = "dir"              // dir:<path>       — directory exists
	ProbeVSCodeExt       ProbeKind = "vscode-ext"       // vscode-ext:<id>  — VS Code/Cursor/Windsurf extension
	ProbeCursorExt       ProbeKind = "cursor-ext"       // cursor-ext:<id>  — Cursor-specific extension
	ProbeMacApp          ProbeKind = "macapp"           // macapp:<name>    — installed macOS app
	ProbeJetBrainsPlugin ProbeKind = "jetbrains-plugin" // jetbrains-plugin:<id>
)

// Probe is a single detection clause within a provider's detect spec.
type Probe struct {
	Kind ProbeKind
	Arg  string // e.g. "claude", "$HOME/.foo", "Cursor"
}

// Provider is a single row in the PROVIDERS matrix.
type Provider struct {
	ID      string
	Label   string
	Mech    Mech
	Detect  string // raw spec string for human display
	Probes  []Probe
	Profile string
	Soft    bool
	// Detected is set by Detect() to true when at least one of
	// the Probes fired. It is NOT serialized as part of the
	// catalog snapshot.
	Detected bool `json:"-"`
}

// ProbesParse parses a "detect" spec string into a slice of Probe.
// Clauses are OR-separated by "||". Each clause is "<kind>:<arg>".
// $HOME and ~ are expanded.
func ProbesParse(spec string) []Probe {
	var out []Probe
	for _, clause := range strings.Split(spec, "||") {
		clause = strings.TrimSpace(clause)
		if clause == "" {
			continue
		}
		idx := strings.Index(clause, ":")
		if idx < 0 {
			continue
		}
		kind := ProbeKind(clause[:idx])
		arg := strings.TrimSpace(clause[idx+1:])
		// Expand $HOME and ~ in arg
		arg = expandHome(arg)
		out = append(out, Probe{Kind: kind, Arg: arg})
	}
	return out
}

// expandHome replaces a leading $HOME or ~ with the user's home dir.
func expandHome(p string) string {
	if strings.HasPrefix(p, "$HOME") {
		home, _ := os.UserHomeDir()
		return home + strings.TrimPrefix(p, "$HOME")
	}
	if strings.HasPrefix(p, "~") {
		home, _ := os.UserHomeDir()
		return home + strings.TrimPrefix(p, "~")
	}
	return p
}

// Catalog is the full PROVIDERS matrix.
var Catalog = []Provider{
	// Native plugin providers
	{ID: "claude", Label: "Claude Code", Mech: MechClaudePlugin, Detect: "command:claude"},
	{ID: "gemini", Label: "Gemini CLI", Mech: MechGeminiExtensions, Detect: "command:gemini"},
	{ID: "opencode", Label: "opencode", Mech: MechOpenCodePlugin, Detect: "command:opencode"},
	{ID: "openclaw", Label: "OpenClaw", Mech: MechOpenClawSkill, Detect: "command:openclaw||dir:$HOME/.openclaw/workspace"},
	{ID: "codex", Label: "Codex CLI", Mech: MechNpxSkillsAdd, Detect: "command:codex", Profile: "codex"},

	// IDE / VS Code family
	{ID: "cursor", Label: "Cursor", Mech: MechNpxSkillsAdd, Detect: "command:cursor||macapp:Cursor", Profile: "cursor"},
	{ID: "windsurf", Label: "Windsurf", Mech: MechNpxSkillsAdd, Detect: "command:windsurf||macapp:Windsurf", Profile: "windsurf"},
	{ID: "cline", Label: "Cline", Mech: MechNpxSkillsAdd, Detect: "vscode-ext:cline", Profile: "cline"},
	{ID: "continue", Label: "Continue", Mech: MechNpxSkillsAdd, Detect: "vscode-ext:continue.continue||vscode-ext:continue", Profile: "continue"},
	{ID: "kilo", Label: "Kilo Code", Mech: MechNpxSkillsAdd, Detect: "vscode-ext:kilocode", Profile: "kilo"},
	{ID: "roo", Label: "Roo Code", Mech: MechNpxSkillsAdd, Detect: "vscode-ext:roo||vscode-ext:rooveterinaryinc.roo-cline||cursor-ext:roo", Profile: "roo"},
	{ID: "augment", Label: "Augment Code", Mech: MechNpxSkillsAdd, Detect: "vscode-ext:augment||jetbrains-plugin:augment", Profile: "augment"},

	// Soft: opt-in only
	{ID: "copilot", Label: "GitHub Copilot", Mech: MechNpxSkillsAdd, Detect: "command:copilot", Profile: "github-copilot", Soft: true},

	// CLI agents
	{ID: "aider-desk", Label: "Aider Desk", Mech: MechNpxSkillsAdd, Detect: "command:aider", Profile: "aider-desk"},
	{ID: "amp", Label: "Sourcegraph Amp", Mech: MechNpxSkillsAdd, Detect: "command:amp", Profile: "amp"},
	{ID: "bob", Label: "IBM Bob", Mech: MechNpxSkillsAdd, Detect: "command:bob", Profile: "bob"},
	{ID: "crush", Label: "Crush", Mech: MechNpxSkillsAdd, Detect: "command:crush", Profile: "crush"},
	{ID: "devin", Label: "Devin (terminal)", Mech: MechNpxSkillsAdd, Detect: "command:devin", Profile: "devin"},
	{ID: "droid", Label: "Droid (Factory)", Mech: MechNpxSkillsAdd, Detect: "command:droid", Profile: "droid"},
	{ID: "forgecode", Label: "ForgeCode", Mech: MechNpxSkillsAdd, Detect: "command:forge", Profile: "forgecode"},
	{ID: "goose", Label: "Block Goose", Mech: MechNpxSkillsAdd, Detect: "command:goose", Profile: "goose"},
	{ID: "iflow", Label: "iFlow CLI", Mech: MechNpxSkillsAdd, Detect: "command:iflow", Profile: "iflow-cli"},
	{ID: "kiro", Label: "Kiro CLI", Mech: MechNpxSkillsAdd, Detect: "command:kiro", Profile: "kiro-cli"},
	{ID: "mistral", Label: "Mistral Vibe", Mech: MechNpxSkillsAdd, Detect: "command:mistral", Profile: "mistral-vibe"},
	{ID: "openhands", Label: "OpenHands", Mech: MechNpxSkillsAdd, Detect: "command:openhands", Profile: "openhands"},
	{ID: "qwen", Label: "Qwen Code", Mech: MechNpxSkillsAdd, Detect: "command:qwen", Profile: "qwen-code"},
	{ID: "rovodev", Label: "Atlassian Rovo Dev", Mech: MechNpxSkillsAdd, Detect: "command:rovodev", Profile: "rovodev"},
	{ID: "tabnine", Label: "Tabnine CLI", Mech: MechNpxSkillsAdd, Detect: "command:tabnine", Profile: "tabnine-cli"},
	{ID: "trae", Label: "Trae", Mech: MechNpxSkillsAdd, Detect: "command:trae", Profile: "trae"},
	{ID: "warp", Label: "Warp", Mech: MechNpxSkillsAdd, Detect: "command:warp", Profile: "warp"},
	{ID: "replit", Label: "Replit Agent", Mech: MechNpxSkillsAdd, Detect: "command:replit", Profile: "replit"},

	// Soft: best-effort detection
	{ID: "junie", Label: "JetBrains Junie", Mech: MechNpxSkillsAdd, Detect: "jetbrains-plugin:junie", Profile: "junie", Soft: true},
	{ID: "qoder", Label: "Qoder", Mech: MechNpxSkillsAdd, Detect: "dir:$HOME/.qoder", Profile: "qoder", Soft: true},
	{ID: "antigravity", Label: "Google Antigravity", Mech: MechNpxSkillsAdd, Detect: "dir:$HOME/.gemini/antigravity", Profile: "antigravity", Soft: true},
}

// compiled is the catalog with parsed Probes. Built once on first use.
var (
	compiledOnce sync.Once
	compiled     []Provider
	idIndex      map[string]int
)

// compiledCatalog returns the catalog with each provider's Probes
// pre-parsed from its Detect string. Safe for concurrent use.
func compiledCatalog() []Provider {
	compiledOnce.Do(func() {
		compiled = make([]Provider, len(Catalog))
		idIndex = make(map[string]int, len(Catalog))
		for i, p := range Catalog {
			compiled[i] = p
			compiled[i].Probes = ProbesParse(p.Detect)
			idIndex[p.ID] = i
		}
	})
	return compiled
}

// Get returns a provider by id, or nil if not found.
func Get(id string) *Provider {
	cat := compiledCatalog()
	idx, ok := idIndex[id]
	if !ok {
		return nil
	}
	p := cat[idx]
	return &p
}

// All returns all providers in the catalog.
func All() []Provider {
	cat := compiledCatalog()
	out := make([]Provider, len(cat))
	copy(out, cat)
	return out
}

// Hard returns only non-soft providers (the ones that participate in
// auto-detect).
func Hard() []Provider {
	cat := compiledCatalog()
	var out []Provider
	for _, p := range cat {
		if !p.Soft {
			out = append(out, p)
		}
	}
	return out
}

// Detect runs every probe in the catalog and returns the set of
// providers that have at least one probe firing. Soft providers are
// included (the caller can filter if needed).
//
// The function is safe for concurrent use; the per-provider state
// is stored in a snapshot returned by Detect so callers can inspect
// it without mutating the global catalog.
func Detect() []Provider {
	cat := compiledCatalog()
	var out []Provider
	for _, base := range cat {
		p := base // copy
		p.Detected = false
		for _, probe := range p.Probes {
			if probeFire(probe) {
				p.Detected = true
				break
			}
		}
		if p.Detected {
			out = append(out, p)
		}
	}
	return out
}

// probeFire runs a single detection probe. Returns true if the
// underlying check passed (command on PATH, dir exists, etc.).
func probeFire(p Probe) bool {
	switch p.Kind {
	case ProbeCommand:
		return hasCommand(p.Arg)
	case ProbeDir:
		return dirExists(p.Arg)
	case ProbeVSCodeExt:
		return vscodeExtPresent(p.Arg, false)
	case ProbeCursorExt:
		return vscodeExtPresent(p.Arg, true)
	case ProbeMacApp:
		if runtime.GOOS != "darwin" {
			return false
		}
		return macAppPresent(p.Arg)
	case ProbeJetBrainsPlugin:
		return jetbrainsPluginPresent(p.Arg)
	}
	return false
}

// hasCommand returns true if name is on PATH (executable in some
// directory listed in PATH).
func hasCommand(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

// dirExists returns true if path exists and is a directory.
func dirExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return info.IsDir()
}

// vscodeExtPresent checks if a VS Code / Cursor / Windsurf
// extension matching needle (case-insensitive substring) is
// installed. If cursorOnly is true, only the .cursor/extensions
// dir is checked.
func vscodeExtPresent(needle string, cursorOnly bool) bool {
	home, err := os.UserHomeDir()
	if err != nil {
		return false
	}
	var roots []string
	if cursorOnly {
		roots = []string{filepath.Join(home, ".cursor/extensions")}
	} else {
		roots = []string{
			filepath.Join(home, ".vscode/extensions"),
			filepath.Join(home, ".vscode-server/extensions"),
			filepath.Join(home, ".cursor/extensions"),
			filepath.Join(home, ".windsurf/extensions"),
		}
	}
	for _, root := range roots {
		entries, err := os.ReadDir(root)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if strings.Contains(strings.ToLower(e.Name()), strings.ToLower(needle)) {
				return true
			}
		}
	}
	return false
}

// macAppPresent checks if a macOS application bundle is installed
// at /Applications or ~/Applications. No-op on non-darwin platforms.
func macAppPresent(name string) bool {
	if runtime.GOOS != "darwin" {
		return false
	}
	home, _ := os.UserHomeDir()
	candidates := []string{
		"/Applications",
		filepath.Join(home, "Applications"),
	}
	for _, dir := range candidates {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			low := strings.ToLower(e.Name())
			if strings.HasPrefix(low, strings.ToLower(name)+".app") {
				return true
			}
		}
	}
	return false
}

// jetbrainsPluginPresent checks if a JetBrains plugin matching
// needle is present in ~/.config/JetBrains/<product>/plugins or
// ~/Library/Application Support/JetBrains/<product>/plugins.
//
// Implementation note: the caveman version is more thorough (walks
// every product dir). This port covers the common cases; extend
// per-product as needed.
func jetbrainsPluginPresent(needle string) bool {
	home, err := os.UserHomeDir()
	if err != nil {
		return false
	}
	var roots []string
	switch runtime.GOOS {
	case "darwin":
		roots = []string{
			filepath.Join(home, "Library/Application Support/JetBrains"),
		}
	default:
		roots = []string{
			filepath.Join(home, ".config/JetBrains"),
		}
	}
	for _, root := range roots {
		products, err := os.ReadDir(root)
		if err != nil {
			continue
		}
		for _, p := range products {
			if !p.IsDir() {
				continue
			}
			pluginDir := filepath.Join(root, p.Name(), "plugins")
			entries, err := os.ReadDir(pluginDir)
			if err != nil {
				continue
			}
			for _, e := range entries {
				if strings.Contains(strings.ToLower(e.Name()), strings.ToLower(needle)) {
					return true
				}
			}
		}
	}
	return false
}

// _ ensures bufio is imported (used in future streaming readers;
// keep the import to avoid drift).
var _ = bufio.NewScanner

// _ ensures strings is used (the imports are referenced above).
var _ = strings.Contains
