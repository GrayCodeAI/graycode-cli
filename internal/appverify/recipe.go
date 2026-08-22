// Package appverify implements the "prove it works" project-verification
// workflow adopted from grok-cli's verify subsystem: a deterministic recipe
// (how to install, build, test, start, and smoke-check the app), a persisted
// manifest that acts as the contract between static detection and agentic
// execution, and the phased QA prompt that turns "it builds" into "it boots,
// serves, and shows evidence".
package appverify

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// SmokeKind describes how an app's liveness is proven after boot.
type SmokeKind string

const (
	// SmokeHTTP proves readiness by polling an HTTP endpoint.
	SmokeHTTP SmokeKind = "http"
	// SmokeCLI apps are one-shot commands with no server to poll.
	SmokeCLI SmokeKind = "cli"
	// SmokeNone means no smoke check is available.
	SmokeNone SmokeKind = "none"
)

// Recipe is the deterministic description of how to verify an app. Fixed
// argument lists (no shell strings) keep execution bounded and safe.
type Recipe struct {
	Ecosystem string    `json:"ecosystem"`          // go, node, python, rust, java, unknown
	AppKind   string    `json:"appKind"`            // web, api, cli, library
	AppLabel  string    `json:"appLabel"`           // human-readable, e.g. "nextjs app"
	Install   []string  `json:"install,omitempty"`  // fixed argv, e.g. ["npm","ci"]
	Build     []string  `json:"build,omitempty"`    // fixed argv
	Test      []string  `json:"test,omitempty"`     // fixed argv
	Start     []string  `json:"start,omitempty"`    // fixed argv for booting the app
	Port      int       `json:"port,omitempty"`     // expected listen port when known
	SmokeKind SmokeKind `json:"smokeKind"`          // http | cli | none
	Evidence  []string  `json:"evidence,omitempty"` // artifact paths produced by verification
	Notes     []string  `json:"notes,omitempty"`    // why this recipe was chosen / caveats
}

// Detect infers a Recipe deterministically from project markers in root. It
// always returns a usable recipe; unknown projects get smokeKind "none" plus a
// note telling the caller to inspect directly.
func Detect(root string) Recipe {
	if r, ok := detectNode(root); ok {
		return r
	}
	if r, ok := detectGo(root); ok {
		return r
	}
	if r, ok := detectPython(root); ok {
		return r
	}
	if r, ok := detectRust(root); ok {
		return r
	}
	return Recipe{
		Ecosystem: "unknown",
		AppKind:   "unknown",
		AppLabel:  "unrecognized project",
		SmokeKind: SmokeNone,
		Notes:     []string{"no recognized project markers — inspect the tree directly"},
	}
}

func hasFile(root, name string) bool {
	st, err := os.Stat(filepath.Join(root, name))
	return err == nil && !st.IsDir()
}

// node framework defaults: dependency marker → (kind, default dev port).
var nodeFrameworks = []struct {
	dependency string
	appKind    string
	label      string
	port       int
}{
	{"next", "web", "next.js app", 3000},
	{"@sveltejs/kit", "web", "sveltekit app", 5173},
	{"astro", "web", "astro app", 4321},
	{"@remix-run/react", "web", "remix app", 3000},
	{"react-scripts", "web", "create-react-app", 3000},
	{"vite", "web", "vite app", 5173},
	{"express", "api", "express api", 3000},
	{"fastify", "api", "fastify api", 3000},
}

type packageJSON struct {
	Name         string            `json:"name"`
	Scripts      map[string]string `json:"scripts"`
	Dependencies map[string]string `json:"dependencies"`
	DevDeps      map[string]string `json:"devDependencies"`
}

// pickPackageManager chooses the runner from lockfile presence.
func pickPackageManager(root string) string {
	switch {
	case hasFile(root, "pnpm-lock.yaml"):
		return "pnpm"
	case hasFile(root, "bun.lockb"), hasFile(root, "bun.lock"):
		return "bun"
	case hasFile(root, "yarn.lock"):
		return "yarn"
	default:
		return "npm"
	}
}

// runScript builds the fixed argv for running a package.json script under the
// detected package manager.
func runScript(manager, script string) []string {
	switch manager {
	case "pnpm":
		return []string{"pnpm", "run", script}
	case "bun":
		return []string{"bun", "run", script}
	case "yarn":
		return []string{"yarn", script}
	default:
		return []string{"npm", "run", script}
	}
}

func detectNode(root string) (Recipe, bool) {
	raw, err := os.ReadFile(filepath.Join(root, "package.json")) // #nosec G304 -- path built from caller-provided root
	if err != nil {
		return Recipe{}, false
	}
	var pkg packageJSON
	if err := json.Unmarshal(raw, &pkg); err != nil {
		return Recipe{}, false
	}

	manager := pickPackageManager(root)
	deps := map[string]string{}
	for k, v := range pkg.Dependencies {
		deps[k] = v
	}
	for k, v := range pkg.DevDeps {
		deps[k] = v
	}

	r := Recipe{Ecosystem: "node"}
	for _, fw := range nodeFrameworks {
		if _, ok := deps[fw.dependency]; ok {
			r.AppKind = fw.appKind
			r.AppLabel = fw.label
			r.Port = fw.port
			break
		}
	}
	if r.AppKind == "" {
		r.AppKind = "library"
		r.AppLabel = "node package"
	}

	// Prefer explicit dev/start scripts for booting.
	switch {
	case pkg.Scripts["dev"] != "":
		r.Start = runScript(manager, "dev")
	case pkg.Scripts["start"] != "":
		r.Start = runScript(manager, "start")
	}

	if hasFile(root, "package-lock.json") || hasFile(root, "pnpm-lock.yaml") ||
		hasFile(root, "yarn.lock") || hasFile(root, "bun.lockb") || hasFile(root, "bun.lock") {
		switch manager {
		case "pnpm":
			r.Install = []string{"pnpm", "install", "--frozen-lockfile"}
		case "bun":
			r.Install = []string{"bun", "install", "--frozen-lockfile"}
		case "yarn":
			r.Install = []string{"yarn", "install", "--frozen-lockfile"}
		default:
			r.Install = []string{"npm", "ci"}
		}
	} else if _, ok := deps["next"]; ok || len(deps) > 0 {
		r.Install = runScript(manager, "install")
	}

	if pkg.Scripts["build"] != "" {
		r.Build = runScript(manager, "build")
	}
	if pkg.Scripts["test"] != "" {
		r.Test = runScript(manager, "test")
	}

	classifySmoke(&r)
	if r.SmokeKind == SmokeNone && r.Start != nil {
		r.Notes = append(r.Notes, "start script present but no port inferred; confirm the listen port before HTTP smoke checks")
	}
	return r, true
}

func detectGo(root string) (Recipe, bool) {
	if !hasFile(root, "go.mod") {
		return Recipe{}, false
	}
	r := Recipe{
		Ecosystem: "go",
		Build:     []string{"go", "build", "./..."},
		Test:      []string{"go", "test", "./..."},
	}
	if hasFile(root, "main.go") {
		r.AppKind = "cli"
		r.AppLabel = "go application"
		r.Start = []string{"go", "run", "."}
		r.SmokeKind = SmokeCLI
	} else {
		r.AppKind = "library"
		r.AppLabel = "go module"
		r.SmokeKind = SmokeNone
	}
	return r, true
}

func detectPython(root string) (Recipe, bool) {
	isPy := hasFile(root, "pyproject.toml") || hasFile(root, "requirements.txt") ||
		hasFile(root, "manage.py") || hasFile(root, "setup.py")
	if !isPy {
		return Recipe{}, false
	}
	r := Recipe{Ecosystem: "python"}
	switch {
	case hasFile(root, "manage.py"):
		r.AppKind = "web"
		r.AppLabel = "django app"
		r.Install = []string{"python3", "-m", "pip", "install", "-r", "requirements.txt"}
		r.Test = []string{"python3", "manage.py", "test"}
		r.Start = []string{"python3", "manage.py", "runserver", "0.0.0.0:8000"}
		r.Port = 8000
	default:
		r.AppKind = "library"
		r.AppLabel = "python project"
		if hasFile(root, "requirements.txt") {
			r.Install = []string{"python3", "-m", "pip", "install", "-r", "requirements.txt"}
		}
		r.Test = []string{"python3", "-m", "pytest"}
	}
	classifySmoke(&r)
	return r, true
}

func detectRust(root string) (Recipe, bool) {
	if !hasFile(root, "Cargo.toml") {
		return Recipe{}, false
	}
	r := Recipe{
		Ecosystem: "rust",
		AppKind:   "cli",
		AppLabel:  "rust binary",
		Install:   nil, // cargo fetches on build
		Build:     []string{"cargo", "build"},
		Test:      []string{"cargo", "test"},
		Start:     []string{"cargo", "run"},
		SmokeKind: SmokeCLI,
	}
	return r, true
}

// classifySmoke sets SmokeKind/Port based on what the rest of the recipe
// implies: a Start command with a known port means HTTP smoke is possible.
func classifySmoke(r *Recipe) {
	if r.Port > 0 && r.Start != nil {
		r.SmokeKind = SmokeHTTP
		return
	}
	if r.Start != nil && r.SmokeKind == "" {
		r.SmokeKind = SmokeCLI
		return
	}
	if r.SmokeKind == "" {
		r.SmokeKind = SmokeNone
	}
}

// Normalize validates and canonicalizes a recipe parsed from untrusted JSON
// (e.g. an LLM-proposed manifest). It filters garbage rather than failing so
// callers can safely round-trip agent output.
func Normalize(raw []byte) (Recipe, error) {
	var r Recipe
	if err := json.Unmarshal(raw, &r); err != nil {
		return Recipe{}, fmt.Errorf("appverify: parse recipe: %w", err)
	}
	r.Ecosystem = sanitizeToken(r.Ecosystem)
	r.AppKind = sanitizeToken(r.AppKind)
	r.AppLabel = strings.TrimSpace(r.AppLabel)
	if r.Ecosystem == "" {
		return Recipe{}, fmt.Errorf("appverify: recipe missing ecosystem")
	}
	switch r.SmokeKind {
	case SmokeHTTP, SmokeCLI, SmokeNone:
	case "":
		r.SmokeKind = SmokeNone
	default:
		r.SmokeKind = SmokeNone
	}
	if r.Port < 0 || r.Port > 65535 {
		r.Port = 0
	}
	r.Install = sanitizeArgs(r.Install)
	r.Build = sanitizeArgs(r.Build)
	r.Test = sanitizeArgs(r.Test)
	r.Start = sanitizeArgs(r.Start)
	if r.Start == nil && r.SmokeKind == SmokeHTTP {
		r.SmokeKind = SmokeNone
	}
	return r, nil
}

func sanitizeToken(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	for _, c := range s {
		if (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '-' || c == '_' {
			b.WriteRune(c)
		}
	}
	return b.String()
}

// sanitizeArgs keeps only non-empty, control-character-free arguments.
func sanitizeArgs(args []string) []string {
	out := make([]string, 0, len(args))
	for _, a := range args {
		a = strings.TrimSpace(a)
		if a == "" || strings.ContainsAny(a, "\x00\n\r") {
			continue
		}
		out = append(out, a)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// SmokeTarget renders the URL used for HTTP readiness polling.
func (r Recipe) SmokeTarget() string {
	if r.Port <= 0 {
		return ""
	}
	return fmt.Sprintf("http://127.0.0.1:%d/", r.Port)
}
