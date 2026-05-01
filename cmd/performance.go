package cmd

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/GrayCodeAI/hawk/mcp"
	"github.com/GrayCodeAI/hawk/plugin"
)

// ──────────────────────────────────────────────────────────────────────────────
// 1. Lazy MCP — deferred connection to MCP servers
// ──────────────────────────────────────────────────────────────────────────────

// lazyMCP wraps an MCP server connection so the subprocess is only spawned on
// the first call that actually needs it. This avoids blocking startup when
// MCP servers are configured but not immediately used.
type lazyMCP struct {
	name    string
	command string
	args    []string

	once   sync.Once
	server *mcp.Server
	err    error
}

// newLazyMCP creates a deferred MCP connection. The actual process is not
// started until Connect is called.
func newLazyMCP(name, command string, args ...string) *lazyMCP {
	return &lazyMCP{
		name:    name,
		command: command,
		args:    args,
	}
}

// Connect returns the underlying MCP server, connecting on first call.
// Subsequent calls return the cached connection (or cached error).
func (l *lazyMCP) Connect(ctx context.Context) (*mcp.Server, error) {
	l.once.Do(func() {
		l.server, l.err = mcp.Connect(ctx, l.name, l.command, l.args...)
	})
	return l.server, l.err
}

// Close shuts down the MCP server if it was ever started.
func (l *lazyMCP) Close() error {
	if l.server != nil {
		return l.server.Close()
	}
	return nil
}

// ──────────────────────────────────────────────────────────────────────────────
// 2. Lazy plugins — load plugins on first command, not startup
// ──────────────────────────────────────────────────────────────────────────────

// lazyPlugins wraps the plugin runtime so that plugins are only loaded from
// disk when the first plugin-related operation occurs.
type lazyPlugins struct {
	once    sync.Once
	runtime *plugin.Runtime
	err     error
}

// newLazyPlugins creates a deferred plugin loader.
func newLazyPlugins() *lazyPlugins {
	return &lazyPlugins{}
}

// Runtime returns the plugin runtime, loading all plugins on first call.
func (lp *lazyPlugins) Runtime() (*plugin.Runtime, error) {
	lp.once.Do(func() {
		rt := plugin.NewRuntime()
		lp.err = rt.LoadAll()
		if lp.err == nil {
			rt.RegisterHooks()
			lp.runtime = rt
		}
	})
	return lp.runtime, lp.err
}

// IsCommand checks whether name is a plugin command, loading lazily if needed.
func (lp *lazyPlugins) IsCommand(name string) bool {
	rt, err := lp.Runtime()
	if err != nil || rt == nil {
		return false
	}
	return rt.IsCommand(name)
}

// ──────────────────────────────────────────────────────────────────────────────
// 3. Startup profiling — measure and report startup time
// ──────────────────────────────────────────────────────────────────────────────

// startupProfile measures wall-clock time for a startup phase and, when
// HAWK_DEBUG=1, prints the result to stderr. In non-debug mode it is a no-op.
type startupProfile struct {
	label string
	start time.Time
	debug bool
}

// newStartupProfile begins timing a startup phase. Call .Done() when the
// phase completes.
func newStartupProfile(label string) *startupProfile {
	debug := os.Getenv("HAWK_DEBUG") == "1"
	return &startupProfile{
		label: label,
		start: time.Now(),
		debug: debug,
	}
}

// Done marks the phase as complete and, in debug mode, prints elapsed time.
func (sp *startupProfile) Done() time.Duration {
	elapsed := time.Since(sp.start)
	if sp.debug {
		fmt.Fprintf(os.Stderr, "[hawk:perf] %s: %s\n", sp.label, elapsed.Round(time.Microsecond))
	}
	return elapsed
}

// ──────────────────────────────────────────────────────────────────────────────
// 4. API pre-warming — optional TCP connection warm-up
// ──────────────────────────────────────────────────────────────────────────────

// preWarmAPI sends a lightweight HTTP HEAD request to the given API base URL
// in a background goroutine so the TCP+TLS handshake completes before the
// first real API call. This reduces perceived latency for the first query.
//
// The goroutine is fire-and-forget; failures are silently ignored because
// the real API call will surface errors.
func preWarmAPI(apiBaseURL string) {
	if apiBaseURL == "" {
		return
	}
	go func() {
		transport := &http.Transport{
			DialContext: (&net.Dialer{
				Timeout: 3 * time.Second,
			}).DialContext,
			TLSHandshakeTimeout: 3 * time.Second,
		}
		c := &http.Client{
			Transport: transport,
			Timeout:   5 * time.Second,
		}
		// HEAD request keeps payload minimal; we only care about the connection.
		req, err := http.NewRequest(http.MethodHead, apiBaseURL, nil)
		if err != nil {
			return
		}
		resp, err := c.Do(req)
		if err != nil {
			return
		}
		resp.Body.Close()
	}()
}
