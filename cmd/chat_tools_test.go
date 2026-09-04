package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	graycodeconfig "github.com/GrayCodeAI/graycode-cli/internal/config"
	"github.com/GrayCodeAI/graycode-cli/internal/tool"
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

func TestConfiguredStartupMCPServers_DispatchesByType(t *testing.T) {
	settings := graycodeconfig.Settings{
		MCPServers: []graycodeconfig.MCPServerConfig{
			{Name: "stdio-default", Command: "stdio-mcp"},
			{Name: "stdio-explicit", Type: "stdio", Command: "stdio-mcp-2"},
			{Name: "http-server", Type: "http", URL: "https://example.com/mcp"},
			{Name: "sse-server", Type: "sse", URL: "https://example.com/sse", Headers: map[string]string{"X-Custom": "1"}},
			{Name: "ws-server", Type: "websocket", URL: "wss://example.com/ws"},
			// Invalid/incomplete entries must be skipped, not error.
			{Name: "", Command: "no-name"},
			{Name: "no-command"},                // stdio with no Command
			{Name: "http-no-url", Type: "http"}, // http with no URL
			{Name: "unknown-type", Type: "carrier-pigeon", URL: "x"},
		},
	}

	specs := configuredStartupMCPServers(settings)

	byName := make(map[string]startupMCPServerSpec, len(specs))
	for _, s := range specs {
		byName[s.name] = s
	}
	if len(specs) != 5 {
		t.Fatalf("expected 5 valid specs, got %d: %+v", len(specs), specs)
	}

	stdioDefault, ok := byName["stdio-default"]
	if !ok || stdioDefault.isRemote() || stdioDefault.command != "stdio-mcp" {
		t.Fatalf("stdio-default spec wrong: %+v (ok=%v)", stdioDefault, ok)
	}

	httpServer, ok := byName["http-server"]
	if !ok || !httpServer.isRemote() || httpServer.serverType != "http" || httpServer.url != "https://example.com/mcp" {
		t.Fatalf("http-server spec wrong: %+v (ok=%v)", httpServer, ok)
	}

	sseServer, ok := byName["sse-server"]
	if !ok || sseServer.serverType != "sse" || sseServer.headers["X-Custom"] != "1" {
		t.Fatalf("sse-server spec wrong: %+v (ok=%v)", sseServer, ok)
	}

	wsServer, ok := byName["ws-server"]
	if !ok || wsServer.serverType != "websocket" {
		t.Fatalf("ws-server spec wrong: %+v (ok=%v)", wsServer, ok)
	}

	for _, missing := range []string{"", "no-command", "http-no-url", "unknown-type"} {
		if _, found := byName[missing]; found {
			t.Fatalf("expected %q to be skipped, but it was included", missing)
		}
	}
}

func TestLoadStartupMCPToolSets_DispatchesRemoteSpecsToRemoteLoader(t *testing.T) {
	origStdio := defaultRegistryLoadMCPTools
	origRemote := defaultRegistryLoadRemoteMCPTools
	t.Cleanup(func() {
		defaultRegistryLoadMCPTools = origStdio
		defaultRegistryLoadRemoteMCPTools = origRemote
	})

	var stdioCalled, remoteCalled bool
	var gotServerType, gotURL string
	var gotHeaders map[string]string

	defaultRegistryLoadMCPTools = func(ctx context.Context, name, command string, args ...string) ([]tool.Tool, error) {
		stdioCalled = true
		return []tool.Tool{registryTestTool{name: name}}, nil
	}
	defaultRegistryLoadRemoteMCPTools = func(ctx context.Context, name, serverType, url string, headers map[string]string) ([]tool.Tool, error) {
		remoteCalled = true
		gotServerType = serverType
		gotURL = url
		gotHeaders = headers
		return []tool.Tool{registryTestTool{name: name}}, nil
	}

	sets := loadStartupMCPToolSets([]startupMCPServerSpec{
		{name: "stdio-one", command: "cmd"},
		{
			name:       "remote-one",
			serverType: "http",
			url:        "https://example.com/mcp",
			headers:    map[string]string{"Authorization": "Bearer xyz"},
		},
	})

	if !stdioCalled {
		t.Error("expected the stdio loader to be called for the stdio spec")
	}
	if !remoteCalled {
		t.Fatal("expected the remote loader to be called for the remote spec")
	}
	if gotServerType != "http" || gotURL != "https://example.com/mcp" {
		t.Errorf("remote loader got serverType=%q url=%q", gotServerType, gotURL)
	}
	if gotHeaders["Authorization"] != "Bearer xyz" {
		t.Errorf("remote loader did not receive configured headers: %+v", gotHeaders)
	}
	if len(sets) != 2 || len(sets[0]) != 1 || len(sets[1]) != 1 {
		t.Fatalf("expected both specs to produce one tool each, got %+v", sets)
	}
}

func TestMergedMCPHeaders_ConfiguredAuthorizationTakesPrecedence(t *testing.T) {
	cfg := graycodeconfig.MCPServerConfig{
		Name:    "svc",
		Headers: map[string]string{"Authorization": "Bearer static-token", "X-Other": "1"},
	}
	headers := mergedMCPHeaders(cfg)
	if headers["Authorization"] != "Bearer static-token" {
		t.Errorf("expected static Authorization header to win, got %q", headers["Authorization"])
	}
	if headers["X-Other"] != "1" {
		t.Errorf("expected other static headers to be preserved, got %+v", headers)
	}
}

// TestEssentialOptionalTools_NoOverlapOrDuplicates guards against drift between
// the hand-maintained essentialTools() and optionalTools() lists (M11): a tool
// must appear in exactly one list. Duplicates within a list or an overlap across
// lists would silently double-register or misclassify a tool.
func TestEssentialOptionalTools_NoOverlapOrDuplicates(t *testing.T) {
	essential := essentialTools()
	optional := optionalTools()

	seen := make(map[string]struct{})
	for _, tl := range essential {
		if _, dup := seen[tl.Name()]; dup {
			t.Fatalf("essentialTools() duplicates tool %q", tl.Name())
		}
		seen[tl.Name()] = struct{}{}
	}
	for _, tl := range optional {
		if _, dup := seen[tl.Name()]; dup {
			t.Fatalf("optionalTools() tool %q also appears in essentialTools()", tl.Name())
		}
		seen[tl.Name()] = struct{}{}
	}
}

func TestDefaultRegistry_SkipsFailedStartupMCPServers(t *testing.T) {
	orig := defaultRegistryLoadMCPTools
	t.Cleanup(func() { defaultRegistryLoadMCPTools = orig })

	defaultRegistryLoadMCPTools = func(ctx context.Context, name, command string, args ...string) ([]tool.Tool, error) {
		return nil, errors.New("boom")
	}

	registry, err := defaultRegistry(graycodeconfig.Settings{
		MCPServers: []graycodeconfig.MCPServerConfig{{Name: "demo", Command: "demo-mcp"}},
	})
	if err != nil {
		t.Fatalf("defaultRegistry returned error: %v", err)
	}
	if _, ok := registry.Get("Bash"); !ok {
		t.Fatal("expected essential tools to remain available when MCP startup load fails")
	}
}
