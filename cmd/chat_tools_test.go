package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	hawkconfig "github.com/GrayCodeAI/hawk/internal/config"
	"github.com/GrayCodeAI/hawk/internal/tool"
)

type registryTestTool struct {
	name string
}

func (t registryTestTool) Name() string { return t.name }

func (t registryTestTool) Description() string { return t.name }

func (t registryTestTool) Parameters() map[string]interface{} { return map[string]interface{}{} }

func (t registryTestTool) Execute(context.Context, json.RawMessage) (string, error) {
	return "", nil
}

func TestLoadStartupMCPToolSets_UsesTimeoutAndPreservesOrder(t *testing.T) {
	orig := defaultRegistryLoadMCPTools
	t.Cleanup(func() { defaultRegistryLoadMCPTools = orig })

	var mu sync.Mutex
	seenDeadlines := map[string]time.Time{}
	defaultRegistryLoadMCPTools = func(ctx context.Context, name, command string, args ...string) ([]tool.Tool, error) {
		deadline, ok := ctx.Deadline()
		if !ok {
			t.Fatalf("expected deadline for %s", name)
		}
		mu.Lock()
		seenDeadlines[name] = deadline
		mu.Unlock()
		return []tool.Tool{registryTestTool{name: name}}, nil
	}

	sets := loadStartupMCPToolSets([]startupMCPServerSpec{
		{name: "alpha", command: "alpha-mcp"},
		{name: "beta", command: "beta-mcp"},
	})

	if len(sets) != 2 {
		t.Fatalf("expected 2 result slots, got %d", len(sets))
	}
	for i, want := range []string{"alpha", "beta"} {
		if len(sets[i]) != 1 {
			t.Fatalf("slot %d: expected 1 tool, got %d", i, len(sets[i]))
		}
		if got := sets[i][0].Name(); got != want {
			t.Fatalf("slot %d: got %q want %q", i, got, want)
		}
		mu.Lock()
		deadline, ok := seenDeadlines[want]
		mu.Unlock()
		if !ok {
			t.Fatalf("missing deadline for %s", want)
		}
		until := time.Until(deadline)
		if until <= 0 || until > startupMCPToolLoadTimeout {
			t.Fatalf("%s deadline = %s, want within (0,%s]", want, until, startupMCPToolLoadTimeout)
		}
	}
}

func TestDefaultRegistry_SkipsFailedStartupMCPServers(t *testing.T) {
	orig := defaultRegistryLoadMCPTools
	t.Cleanup(func() { defaultRegistryLoadMCPTools = orig })

	defaultRegistryLoadMCPTools = func(ctx context.Context, name, command string, args ...string) ([]tool.Tool, error) {
		return nil, errors.New("boom")
	}

	registry, err := defaultRegistry(hawkconfig.Settings{
		MCPServers: []hawkconfig.MCPServerConfig{{Name: "demo", Command: "demo-mcp"}},
	})
	if err != nil {
		t.Fatalf("defaultRegistry returned error: %v", err)
	}
	if _, ok := registry.Get("Bash"); !ok {
		t.Fatal("expected essential tools to remain available when MCP startup load fails")
	}
}
