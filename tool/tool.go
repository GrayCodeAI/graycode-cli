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

// ToolContext carries session-level functions for tools that need them.
type ToolContext struct {
	AgentSpawnFn func(ctx context.Context, prompt string) (string, error)
	AskUserFn    func(question string) (string, error)
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
	tools map[string]Tool
}

// NewRegistry creates a registry with the given tools.
func NewRegistry(tools ...Tool) *Registry {
	r := &Registry{tools: make(map[string]Tool, len(tools))}
	for _, t := range tools {
		r.tools[t.Name()] = t
	}
	return r
}

// Get returns a tool by name.
func (r *Registry) Get(name string) (Tool, bool) {
	t, ok := r.tools[name]
	return t, ok
}

// EyrieTools converts all tools to eyrie tool definitions for the API.
func (r *Registry) EyrieTools() []client.EyrieTool {
	out := make([]client.EyrieTool, 0, len(r.tools))
	for _, t := range r.tools {
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
