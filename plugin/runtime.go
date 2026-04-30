package plugin

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/GrayCodeAI/hawk/hooks"
)

// Runtime manages loaded plugins and their execution.
type Runtime struct {
	plugins  []*Manifest
	commands map[string]CommandDef
	hooks    map[string][]HookDef
}

// NewRuntime creates a new plugin runtime.
func NewRuntime() *Runtime {
	return &Runtime{
		commands: make(map[string]CommandDef),
		hooks:    make(map[string][]HookDef),
	}
}

// LoadAll loads all installed plugins.
func (r *Runtime) LoadAll() error {
	plugins, err := List()
	if err != nil {
		return err
	}
	r.plugins = plugins
	for _, p := range plugins {
		for _, cmd := range p.Commands {
			r.commands[cmd.Name] = cmd
		}
		for _, h := range p.Hooks {
			r.hooks[h.Event] = append(r.hooks[h.Event], h)
		}
	}
	return nil
}

// ExecuteCommand runs a plugin command.
func (r *Runtime) ExecuteCommand(name string, args []string) (string, error) {
	cmd, ok := r.commands[name]
	if !ok {
		return "", fmt.Errorf("unknown plugin command: %s", name)
	}
	if cmd.Script == "" {
		return "", fmt.Errorf("command %s has no script", name)
	}
	ctx := context.Background()
	c := exec.CommandContext(ctx, "bash", "-c", cmd.Script)
	c.Args = append(c.Args, args...)
	c.Dir = filepath.Join(pluginsDir(), cmd.Name)
	out, err := c.CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("command failed: %w", err)
	}
	return string(out), nil
}

// RegisterHooks registers all plugin hooks with the hook registry.
func (r *Runtime) RegisterHooks() {
	for event, hookList := range r.hooks {
		for _, h := range hookList {
			cmd := h.Command
			hooks.Register(hooks.Hook{
				Name:  fmt.Sprintf("plugin:%s", event),
				Event: hooks.EventType(event),
				Fn: func(ctx context.Context, data map[string]interface{}) error {
					c := exec.CommandContext(ctx, "bash", "-c", cmd)
					c.Env = os.Environ()
					for k, v := range data {
						c.Env = append(c.Env, fmt.Sprintf("%s=%v", strings.ToUpper(k), v))
					}
					out, err := c.CombinedOutput()
					if err != nil {
						return fmt.Errorf("hook failed: %w\n%s", err, string(out))
					}
					return nil
				},
			})
		}
	}
}

// CommandList returns all available plugin commands.
func (r *Runtime) CommandList() []CommandDef {
	var out []CommandDef
	for _, cmd := range r.commands {
		out = append(out, cmd)
	}
	return out
}

// IsCommand checks if a name is a plugin command.
func (r *Runtime) IsCommand(name string) bool {
	_, ok := r.commands[name]
	return ok
}
