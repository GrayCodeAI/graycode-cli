package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/GrayCodeAI/eyrie/client"
	"github.com/GrayCodeAI/hawk/internal/intelligence/memory"
	"github.com/GrayCodeAI/hawk/internal/lint"
	"github.com/GrayCodeAI/hawk/internal/sandbox"
	"github.com/GrayCodeAI/hawk/internal/types"
)

// Tool is the interface every hawk tool implements.
type Tool interface {
	Name() string
	Description() string
	Parameters() map[string]interface{}
	Execute(ctx context.Context, input json.RawMessage) (string, error)
}

// AliasedTool can be implemented by tools that need backward-compatible wire names.
type AliasedTool interface {
	Aliases() []string
}

// RiskLevelProvider can be implemented by tools to declare their risk level.
// Tools that don't implement it default to "medium".
type RiskLevelProvider interface {
	RiskLevel() string // "low", "medium", "high"
}

// PathProtector checks whether a file path is protected (read-only).
// engine.ProtectedPaths implements this interface.
type PathProtector interface {
	IsProtected(path string) bool
}

// CodeSearchResult is returned by CodeSearchFn.
type CodeSearchResult struct {
	Path      string
	StartLine int
	EndLine   int
	Content   string
	Symbol    string
	Language  string
	Score     float64
}

// ToolContext carries session-level functions for tools that need them.
type ToolContext struct {
	AgentSpawnFn       func(ctx context.Context, prompt string) (string, error)
	AskUserFn          func(question string) (string, error)
	CodeSearchFn       func(ctx context.Context, query string, limit int) ([]CodeSearchResult, error)
	RefreshCodeIndexFn func(ctx context.Context) error
	AvailableTools     []Tool
	AllowedDirectories []string
	SandboxMode        sandbox.Mode
	AutoCommit         bool
	Protected          PathProtector
	YaadBridge         *memory.YaadBridge
	Attribution        *types.Attribution
	SettingsGet        func(key string) (string, bool)
	SettingsSet        func(key, value string) error
	// BackgroundManager tracks background sub-agents. If nil, background
	// mode is not available.
	BackgroundManager *BackgroundAgentManager
	// Lint configures the optional post-write auto-lint cycle. The zero value
	// (Enabled=false) keeps linting off so users are not surprised.
	Lint lint.Config
}

// ctxKey is the context key for ToolContext.
type ctxKey struct{}

// WithToolContext attaches a ToolContext to a context.
func WithToolContext(ctx context.Context, tc *ToolContext) context.Context {
	return context.WithValue(ctx, ctxKey{}, tc)
}

// GetToolContext retrieves the ToolContext from a context.
func GetToolContext(ctx context.Context) *ToolContext {
	if tc, ok := ctx.Value(ctxKey{}).(*ToolContext); ok {
		return tc
	}
	return nil
}

// ReadOnlyTools is the canonical allowlist of tool names whose execution is
// side-effect-free and therefore safe to run in parallel with each other
// within a single agent turn. Consumers (engine/stream.go classifyToolCalls,
// engine/stream.go safeSnapshotTools) MUST go through this set instead of
// redefining it inline. To add a new read-only tool, append its canonical
// name here AND ensure canonicalToolName normalises all of its aliases.
var ReadOnlyTools = map[string]bool{
	"Read":       true,
	"Grep":       true,
	"Glob":       true,
	"LS":         true,
	"WebSearch":  true,
	"WebFetch":   true,
	"ToolSearch": true,
}

// IsReadOnly reports whether the given (possibly-aliased) tool name is in
// the read-only allowlist. It first canonicalises the name so an LLM-emitted
// alias like "read" or "file_read" still classifies correctly.
func IsReadOnly(name string) bool {
	return ReadOnlyTools[canonicalForReadOnly(name)]
}

// canonicalForReadOnly is a small case-folded map lookup; it intentionally
// duplicates a subset of engine.canonicalToolName to avoid an import cycle
// (tool → engine would be a cycle because engine imports tool). The map
// below MUST be kept in sync with engine.permission_session_methods.canonicalToolName.
func canonicalForReadOnly(name string) string {
	switch strings.ToLower(name) {
	case "read", "file_read":
		return "Read"
	case "grep":
		return "Grep"
	case "glob":
		return "Glob"
	case "ls":
		return "LS"
	case "websearch", "web_search":
		return "WebSearch"
	case "webfetch", "web_fetch":
		return "WebFetch"
	case "toolsearch", "tool_search":
		return "ToolSearch"
	}
	return name
}

// Registry holds all registered tools.
type Registry struct {
	tools   map[string]Tool
	primary []Tool
}

// NewRegistry creates a registry with the given tools.
func NewRegistry(tools ...Tool) *Registry {
	r := &Registry{tools: make(map[string]Tool, len(tools))}
	for _, t := range tools {
		r.tools[t.Name()] = t
		r.primary = append(r.primary, t)
		if aliased, ok := t.(AliasedTool); ok {
			for _, alias := range aliased.Aliases() {
				if alias != "" {
					if existing, exists := r.tools[alias]; exists && existing.Name() != t.Name() {
						fmt.Fprintf(os.Stderr, "warning: tool alias %q already registered to %s, overriding with %s\n", alias, existing.Name(), t.Name())
					}
					r.tools[alias] = t
				}
			}
		}
	}
	return r
}

// Get returns a tool by name.
func (r *Registry) Get(name string) (Tool, bool) {
	t, ok := r.tools[name]
	return t, ok
}

// PrimaryTools returns the model-visible tools registered in this registry.
func (r *Registry) PrimaryTools() []Tool {
	out := make([]Tool, len(r.primary))
	copy(out, r.primary)
	return out
}

// Filter returns a new Registry containing only tools whose names are in the allowlist.
func (r *Registry) Filter(allow []string) *Registry {
	set := make(map[string]bool, len(allow))
	for _, name := range allow {
		set[name] = true
	}
	var filtered []Tool
	for _, t := range r.primary {
		if set[t.Name()] {
			filtered = append(filtered, t)
		}
	}
	return NewRegistry(filtered...)
}

// EyrieTools converts all tools to eyrie tool definitions for the API.
func (r *Registry) EyrieTools() []client.EyrieTool {
	out := make([]client.EyrieTool, 0, len(r.primary))
	for _, t := range r.primary {
		out = append(out, client.EyrieTool{
			Name:        t.Name(),
			Description: t.Description(),
			Parameters:  t.Parameters(),
		})
	}
	return out
}

// Execute runs a tool by name with the given JSON input.
func (r *Registry) Execute(ctx context.Context, name string, input json.RawMessage) (string, error) {
	t, ok := r.tools[name]
	if !ok {
		return "", fmt.Errorf("unknown tool: %s", name)
	}
	return t.Execute(ctx, input)
}

// Register adds a tool to the registry after creation.
// Returns error if a tool with the same name already exists (unless it's an alias).
func (r *Registry) Register(t Tool) error {
	if _, exists := r.tools[t.Name()]; exists {
		return fmt.Errorf("tool %q already registered", t.Name())
	}
	r.tools[t.Name()] = t
	r.primary = append(r.primary, t)
	if aliased, ok := t.(AliasedTool); ok {
		for _, alias := range aliased.Aliases() {
			if alias != "" {
				r.tools[alias] = t
			}
		}
	}
	return nil
}
