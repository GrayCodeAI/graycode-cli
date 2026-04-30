package tool

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/hawk/eyrie/client"
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

// ToolContext carries session-level functions for tools that need them.
type ToolContext struct {
	AgentSpawnFn       func(ctx context.Context, prompt string) (string, error)
	AskUserFn          func(question string) (string, error)
	AvailableTools     []Tool
	AllowedDirectories []string
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
