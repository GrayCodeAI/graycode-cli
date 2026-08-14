package plugin

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/GrayCodeAI/hawk/internal/storage"
	"github.com/GrayCodeAI/hawk/internal/trust"
)

// PluginState represents the lifecycle state of a dynamic plugin.
type PluginState string

const (
	StateDiscovered PluginState = "discovered"
	StateLoaded     PluginState = "loaded"
	StateActive     PluginState = "active"
	StateFailed     PluginState = "failed"
	StateDisabled   PluginState = "disabled"
)

// DynamicPlugin extends the base Plugin with lifecycle management.
type DynamicPlugin struct {
	Plugin      // embed existing Plugin
	State       PluginState
	Error       string // last error message
	ActivatedAt time.Time
	Process     *PluginProcess // running process (for long-lived plugins)
	HookIDs     []string       // registered hook IDs (for cleanup on deactivate)
	ManifestV2  *ManifestV2    // extended manifest if available
}

// PluginProcess represents a long-lived plugin daemon process.
type PluginProcess struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout *bufio.Scanner
	cancel context.CancelFunc
	mu     sync.Mutex
}

// Send sends a JSON-RPC request to the daemon process and reads the response.
func (pp *PluginProcess) Send(request map[string]interface{}) (map[string]interface{}, error) {
	pp.mu.Lock()
	defer pp.mu.Unlock()

	data, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}
	data = append(data, '\n')

	if _, err := pp.stdin.Write(data); err != nil {
		return nil, fmt.Errorf("write to plugin: %w", err)
	}

	if !pp.stdout.Scan() {
		if err := pp.stdout.Err(); err != nil {
			return nil, fmt.Errorf("read from plugin: %w", err)
		}
		return nil, fmt.Errorf("plugin process closed stdout")
	}

	var response map[string]interface{}
	if err := json.Unmarshal(pp.stdout.Bytes(), &response); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}
	return response, nil
}

// Stop terminates the daemon process.
func (pp *PluginProcess) Stop() {
	pp.mu.Lock()
	defer pp.mu.Unlock()

	if pp.cancel != nil {
		pp.cancel()
	}
	if pp.stdin != nil {
		_ = pp.stdin.Close()
	}
}

// ToolRegistrar allows plugins to add/remove tools from the main registry.
type ToolRegistrar interface {
	AddTool(name string, t interface{})
	RemoveTool(name string)
}

// HookRegistrar allows plugins to add/remove hooks.
type HookRegistrar interface {
	RegisterHook(id string, event string, fn func(ctx context.Context, data map[string]interface{}) error)
	UnregisterHook(id string)
}

// PluginEvent represents a lifecycle event for a plugin.
type PluginEvent struct {
	Type       string // "activated", "deactivated", "failed", "installed"
	PluginName string
	Timestamp  time.Time
	Error      string
}

// PluginStatus provides a snapshot of a plugin's state.
type PluginStatus struct {
	Name        string
	Version     string
	State       PluginState
	ToolCount   int
	HookCount   int
	Error       string
	ActivatedAt time.Time
}

// DynamicPluginManager manages dynamic plugin lifecycle.
type DynamicPluginManager struct {
	mu           sync.RWMutex
	plugins      map[string]*DynamicPlugin
	pluginDirs   []string
	toolRegistry ToolRegistrar
	hookRegistry HookRegistrar
	eventCh      chan PluginEvent
}

// NewDynamicPluginManager creates a new DynamicPluginManager with the given directories and registries.
func NewDynamicPluginManager(dirs []string, tools ToolRegistrar, hooks HookRegistrar) *DynamicPluginManager {
	if len(dirs) == 0 {
		dirs = ResolvePluginDirs("") // managed > user > project (trust-gated)
	}
	return &DynamicPluginManager{
		plugins:      make(map[string]*DynamicPlugin),
		pluginDirs:   dirs,
		toolRegistry: tools,
		hookRegistry: hooks,
		eventCh:      make(chan PluginEvent, 64),
	}
}

// DiscoverAll scans all plugin directories and registers discovered plugins.
// Project-scoped plugin directories require folder trust when HAWK_Y0_FOLDER_TRUST is on.
func (dm *DynamicPluginManager) DiscoverAll() error {
	dm.mu.Lock()
	defer dm.mu.Unlock()

	for _, dir := range dm.pluginDirs {
		if err := trust.AllowLoadPath(dir); err != nil {
			// Skip untrusted project plugin dirs (do not fail entire discovery).
			continue
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return fmt.Errorf("read plugin dir %s: %w", dir, err)
		}

		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			pluginPath := filepath.Join(dir, entry.Name())

			// Try V2 manifest first, fall back to V1
			manifestV2, err := ParseManifestV2(pluginPath)
			if err != nil {
				continue
			}

			name := manifestV2.Name
			if _, exists := dm.plugins[name]; exists {
				continue
			}

			// Convert manifest to plugin tools
			p := manifestV2ToPlugin(manifestV2, pluginPath)

			dm.plugins[name] = &DynamicPlugin{
				Plugin:     *p,
				State:      StateDiscovered,
				ManifestV2: manifestV2,
			}
		}
	}
	return nil
}

// Activate loads a plugin, starts its process (if daemon mode), and registers tools + hooks.
func (dm *DynamicPluginManager) Activate(name string) error {
	dm.mu.Lock()
	dp, exists := dm.plugins[name]
	if !exists {
		dm.mu.Unlock()
		return fmt.Errorf("plugin %q not found; run DiscoverAll first", name)
	}
	if dp.State == StateActive {
		dm.mu.Unlock()
		return nil
	}
	dm.mu.Unlock()

	// Check dependencies
	if dp.ManifestV2 != nil {
		for _, dep := range dp.ManifestV2.Dependencies {
			dm.mu.RLock()
			depPlugin, depExists := dm.plugins[dep]
			dm.mu.RUnlock()
			if !depExists || depPlugin.State != StateActive {
				err := fmt.Errorf("dependency %q is not active", dep)
				dm.setFailed(name, err)
				return err
			}
		}
	}

	// Year 0 PACK-04: export plugin root/data for hook and tool processes.
	_ = ensurePluginDataDir(dp.Path, name)

	// Start daemon process if mode is "daemon"
	if dp.ManifestV2 != nil && dp.ManifestV2.Mode == "daemon" {
		if err := dm.startDaemon(dp); err != nil {
			dm.setFailed(name, err)
			return err
		}
	}

	// Register tools
	if dm.toolRegistry != nil {
		for i := range dp.Tools {
			adapter := &PluginToolAdapter{
				plugin:  dp,
				tool:    dp.Tools[i],
				manager: dm,
			}
			dm.toolRegistry.AddTool(adapter.Name(), adapter)
		}
	}

	// Register hooks from manifest
	if dp.ManifestV2 != nil && dm.hookRegistry != nil {
		for i, h := range dp.ManifestV2.Hooks {
			hookID := fmt.Sprintf("plugin_%s_hook_%d", name, i)
			hookCmd := h.Command
			hookAsync := h.Async
			hookEvent := h.Event

			fn := func(ctx context.Context, data map[string]interface{}) error {
				c := exec.CommandContext(ctx, "bash", "-c", hookCmd) // #nosec G204 -- hookCmd is defined in a locally installed plugin's own manifest, trusted like other plugin config
				c.Dir = dp.Path
				c.Env = pluginHookEnv(dp.Path, name, data)
				if hookAsync {
					go func() {
						_ = c.Run()
					}()
					return nil
				}
				out, err := c.CombinedOutput()
				if err != nil {
					return fmt.Errorf("hook %s failed: %w\n%s", hookEvent, err, string(out))
				}
				return nil
			}

			dm.hookRegistry.RegisterHook(hookID, h.Event, fn)
			dp.HookIDs = append(dp.HookIDs, hookID)
		}
	}

	dm.mu.Lock()
	dp.State = StateActive
	dp.ActivatedAt = time.Now()
	dp.Error = ""
	dm.mu.Unlock()

	dm.emitEvent(PluginEvent{
		Type:       "activated",
		PluginName: name,
		Timestamp:  time.Now(),
	})

	return nil
}

// Deactivate unregisters hooks, removes tools, stops process, and sets state to Disabled.
func (dm *DynamicPluginManager) Deactivate(name string) error {
	dm.mu.Lock()
	dp, exists := dm.plugins[name]
	if !exists {
		dm.mu.Unlock()
		return fmt.Errorf("plugin %q not found", name)
	}
	if dp.State != StateActive && dp.State != StateFailed {
		dm.mu.Unlock()
		return nil
	}
	dm.mu.Unlock()

	// Unregister hooks
	if dm.hookRegistry != nil {
		for _, hookID := range dp.HookIDs {
			dm.hookRegistry.UnregisterHook(hookID)
		}
	}
	dp.HookIDs = nil

	// Remove tools
	if dm.toolRegistry != nil {
		for _, t := range dp.Tools {
			toolName := fmt.Sprintf("plugin__%s__%s", dp.Plugin.Name, t.Name)
			dm.toolRegistry.RemoveTool(toolName)
		}
	}

	// Stop daemon process
	if dp.Process != nil {
		dp.Process.Stop()
		dp.Process = nil
	}

	dm.mu.Lock()
	dp.State = StateDisabled
	dm.mu.Unlock()

	dm.emitEvent(PluginEvent{
		Type:       "deactivated",
		PluginName: name,
		Timestamp:  time.Now(),
	})

	return nil
}

// Reload deactivates and then reactivates a plugin.
func (dm *DynamicPluginManager) Reload(name string) error {
	if err := dm.Deactivate(name); err != nil {
		return fmt.Errorf("deactivate for reload: %w", err)
	}

	// Re-discover to pick up manifest changes
	dm.mu.Lock()
	delete(dm.plugins, name)
	dm.mu.Unlock()

	if err := dm.DiscoverAll(); err != nil {
		return fmt.Errorf("discover for reload: %w", err)
	}

	return dm.Activate(name)
}

// Status returns the status of all known plugins.
func (dm *DynamicPluginManager) Status() []PluginStatus {
	dm.mu.RLock()
	defer dm.mu.RUnlock()

	statuses := make([]PluginStatus, 0, len(dm.plugins))
	for _, dp := range dm.plugins {
		hookCount := len(dp.HookIDs)
		if dp.ManifestV2 != nil {
			hookCount = len(dp.ManifestV2.Hooks)
		}
		statuses = append(statuses, PluginStatus{
			Name:        dp.Plugin.Name,
			Version:     dp.Plugin.Version,
			State:       dp.State,
			ToolCount:   len(dp.Tools),
			HookCount:   hookCount,
			Error:       dp.Error,
			ActivatedAt: dp.ActivatedAt,
		})
	}
	return statuses
}

// Get returns a specific plugin by name.
func (dm *DynamicPluginManager) Get(name string) (*DynamicPlugin, bool) {
	dm.mu.RLock()
	defer dm.mu.RUnlock()
	dp, ok := dm.plugins[name]
	return dp, ok
}

// InstallFromGitHub clones a repo into the plugins directory.
func (dm *DynamicPluginManager) InstallFromGitHub(repo string) error {
	destDir := filepath.Join(storage.StateDir(), "plugins")
	if err := os.MkdirAll(destDir, 0o750); err != nil {
		return fmt.Errorf("create plugins dir: %w", err)
	}

	// Determine plugin name from repo (last path component)
	repoName := filepath.Base(repo)
	pluginDir := filepath.Join(destDir, repoName)

	// Clone the repository
	url := repo
	if !isFullURL(repo) {
		url = "https://github.com/" + repo + ".git"
	}

	// "--" terminates option parsing so a url is never interpreted as a git
	// flag (defense-in-depth; isFullURL already forces an http(s):// prefix).
	cmd := exec.CommandContext(context.Background(), "git", "clone", "--depth", "1", "--single-branch", "--", url, pluginDir) // #nosec G204 -- url is either a caller-supplied full https(s) URL (isFullURL-checked) or built from a repo slug prefixed with a fixed GitHub URL; "--" prevents flag injection
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git clone failed: %w\n%s", err, string(out))
	}

	dm.emitEvent(PluginEvent{
		Type:       "installed",
		PluginName: repoName,
		Timestamp:  time.Now(),
	})

	return nil
}

// Uninstall deactivates a plugin and removes it from disk.
func (dm *DynamicPluginManager) Uninstall(name string) error {
	// Deactivate first if active
	dm.mu.RLock()
	dp, exists := dm.plugins[name]
	dm.mu.RUnlock()

	if exists && (dp.State == StateActive || dp.State == StateFailed) {
		if err := dm.Deactivate(name); err != nil {
			return fmt.Errorf("deactivate before uninstall: %w", err)
		}
	}

	// Find and remove the plugin directory
	var pluginPath string
	if exists {
		pluginPath = dp.Path
	} else {
		// Search for it
		for _, dir := range dm.pluginDirs {
			candidate := filepath.Join(dir, name)
			if _, err := os.Stat(candidate); err == nil {
				pluginPath = candidate
				break
			}
		}
	}

	if pluginPath == "" {
		return fmt.Errorf("plugin %q not found on disk", name)
	}

	if err := os.RemoveAll(pluginPath); err != nil {
		return fmt.Errorf("remove plugin directory: %w", err)
	}

	dm.mu.Lock()
	delete(dm.plugins, name)
	dm.mu.Unlock()

	return nil
}

// Events returns a channel for subscribing to plugin lifecycle events.
func (dm *DynamicPluginManager) Events() <-chan PluginEvent {
	return dm.eventCh
}

// ExecuteTool executes a specific tool on a plugin, using the daemon if available.
func (dm *DynamicPluginManager) ExecuteTool(ctx context.Context, pluginName, toolName string, input json.RawMessage) (string, error) {
	dm.mu.RLock()
	dp, exists := dm.plugins[pluginName]
	dm.mu.RUnlock()

	if !exists {
		return "", fmt.Errorf("plugin %q not found", pluginName)
	}
	if dp.State != StateActive {
		return "", fmt.Errorf("plugin %q is not active (state: %s)", pluginName, dp.State)
	}

	// Find the tool
	var pluginTool *PluginTool
	for i := range dp.Tools {
		if dp.Tools[i].Name == toolName {
			pluginTool = &dp.Tools[i]
			break
		}
	}
	if pluginTool == nil {
		return "", fmt.Errorf("tool %q not found in plugin %q", toolName, pluginName)
	}

	// If daemon mode, use JSON-RPC over stdin/stdout
	if dp.Process != nil {
		return dm.executeDaemonTool(ctx, dp, toolName, input)
	}

	// Otherwise use subprocess execution (existing behavior)
	return dm.executeSubprocessTool(ctx, dp, pluginTool, input)
}

// startDaemon starts a long-lived plugin process.
func (dm *DynamicPluginManager) startDaemon(dp *DynamicPlugin) error {
	entrypoint := "main"
	if dp.ManifestV2 != nil && dp.ManifestV2.Entrypoint != "" {
		entrypoint = dp.ManifestV2.Entrypoint
	}

	cmdPath := filepath.Join(dp.Path, entrypoint)
	if _, err := os.Stat(cmdPath); err != nil {
		// Try with common extensions
		for _, ext := range []string{".exe", ""} {
			candidate := cmdPath + ext
			if _, err := os.Stat(candidate); err == nil {
				cmdPath = candidate
				break
			}
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(ctx, cmdPath) // #nosec G204 -- cmdPath is the locally installed plugin's own entrypoint binary, resolved from its manifest/directory, not external input
	cmd.Dir = dp.Path
	cmd.Env = pluginHookEnv(dp.Path, dp.Name, nil)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		cancel()
		return fmt.Errorf("create stdin pipe: %w", err)
	}

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		return fmt.Errorf("create stdout pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		cancel()
		return fmt.Errorf("start plugin daemon: %w", err)
	}

	dp.Process = &PluginProcess{
		cmd:    cmd,
		stdin:  stdin,
		stdout: bufio.NewScanner(stdoutPipe),
		cancel: cancel,
	}

	// Monitor process in background
	go func() {
		err := cmd.Wait()
		if err != nil && ctx.Err() == nil {
			// Process crashed unexpectedly
			dm.mu.Lock()
			dp.State = StateFailed
			dp.Error = fmt.Sprintf("daemon crashed: %v", err)
			dp.Process = nil
			dm.mu.Unlock()

			dm.emitEvent(PluginEvent{
				Type:       "failed",
				PluginName: dp.Plugin.Name,
				Timestamp:  time.Now(),
				Error:      dp.Error,
			})
		}
	}()

	return nil
}

// executeDaemonTool sends a tool call to the daemon process.
func (dm *DynamicPluginManager) executeDaemonTool(ctx context.Context, dp *DynamicPlugin, toolName string, input json.RawMessage) (string, error) {
	if dp.Process == nil {
		return "", fmt.Errorf("daemon process not running for plugin %q", dp.Plugin.Name)
	}

	request := map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  "tool/call",
		"id":      time.Now().UnixNano(),
		"params": map[string]interface{}{
			"name":  toolName,
			"input": input,
		},
	}

	response, err := dp.Process.Send(request)
	if err != nil {
		return "", fmt.Errorf("daemon call failed: %w", err)
	}

	// Check for JSON-RPC error
	if errObj, ok := response["error"]; ok {
		return "", fmt.Errorf("plugin error: %v", errObj)
	}

	// Extract result
	result, ok := response["result"]
	if !ok {
		return "", fmt.Errorf("no result in response")
	}

	switch v := result.(type) {
	case string:
		return v, nil
	default:
		data, _ := json.Marshal(v)
		return string(data), nil
	}
}

// executeSubprocessTool runs a tool as a one-shot subprocess.
func (dm *DynamicPluginManager) executeSubprocessTool(ctx context.Context, dp *DynamicPlugin, tool *PluginTool, input json.RawMessage) (string, error) {
	timeout := tool.Timeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	parts := splitCommand(tool.Command)
	if len(parts) == 0 {
		return "", fmt.Errorf("tool %q has empty command", tool.Name)
	}

	cmd := exec.CommandContext(ctx, parts[0], parts[1:]...) // #nosec G204 -- tool.Command comes from the plugin's own manifest, trusted like other plugin config
	cmd.Dir = dp.Path

	if input != nil {
		cmd.Stdin = io.NopCloser(
			io.NewSectionReader(
				readerAtFromBytes(input),
				0,
				int64(len(input)),
			),
		)
	}

	out, err := cmd.Output()
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return "", fmt.Errorf("tool %q timed out after %s", tool.Name, timeout)
		}
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return "", fmt.Errorf("tool %q failed: %s", tool.Name, string(exitErr.Stderr))
		}
		return "", fmt.Errorf("tool %q failed: %w", tool.Name, err)
	}

	return string(out), nil
}

// setFailed marks a plugin as failed.
func (dm *DynamicPluginManager) setFailed(name string, err error) {
	dm.mu.Lock()
	if dp, exists := dm.plugins[name]; exists {
		dp.State = StateFailed
		dp.Error = err.Error()
	}
	dm.mu.Unlock()

	dm.emitEvent(PluginEvent{
		Type:       "failed",
		PluginName: name,
		Timestamp:  time.Now(),
		Error:      err.Error(),
	})
}

// emitEvent sends an event to the event channel (non-blocking).
func (dm *DynamicPluginManager) emitEvent(event PluginEvent) {
	select {
	case dm.eventCh <- event:
	default:
		// Drop if channel is full
	}
}

// manifestV2ToPlugin converts a ManifestV2 to a Plugin struct.
func manifestV2ToPlugin(m *ManifestV2, path string) *Plugin {
	p := &Plugin{
		Name:        m.Name,
		Version:     m.Version,
		Description: m.Description,
		Author:      m.Author,
		Path:        path,
		Manifest: &ToolManifest{
			Name:           m.Name,
			Version:        m.Version,
			Description:    m.Description,
			Author:         m.Author,
			Permissions:    m.Permissions,
			MinHawkVersion: m.MinHawkVersion,
		},
	}

	for _, mt := range m.Tools {
		timeout := time.Duration(mt.TimeoutSeconds) * time.Second
		if timeout == 0 {
			timeout = 30 * time.Second
		}

		command := mt.Command
		if len(mt.Args) > 0 {
			for _, arg := range mt.Args {
				command += " " + arg
			}
		}

		p.Tools = append(p.Tools, PluginTool{
			Name:        mt.Name,
			Description: mt.Description,
			InputSchema: mt.InputSchema,
			Command:     command,
			Timeout:     timeout,
			PluginName:  m.Name,
		})
	}

	p.Manifest.Tools = m.Tools
	return p
}

// isFullURL checks if a string looks like a full URL.
func isFullURL(s string) bool {
	return len(s) > 8 && (s[:8] == "https://" || s[:7] == "http://")
}

// splitCommand splits a command string into parts (simple split on spaces).
func splitCommand(cmd string) []string {
	var parts []string
	var current strings.Builder
	var quote rune
	escaped := false
	flush := func() {
		if current.Len() > 0 {
			parts = append(parts, current.String())
			current.Reset()
		}
	}
	for _, c := range cmd {
		if escaped {
			current.WriteRune(c)
			escaped = false
			continue
		}
		if c == '\\' && quote != '\'' {
			escaped = true
			continue
		}
		if quote != 0 {
			if c == quote {
				quote = 0
			} else {
				current.WriteRune(c)
			}
			continue
		}
		switch c {
		case '\'', '"':
			quote = c
		case ' ', '\t', '\n', '\r':
			flush()
		default:
			current.WriteRune(c)
		}
	}
	if escaped {
		current.WriteByte('\\')
	}
	flush()
	return parts
}

// readerAtFromBytes wraps a byte slice as an io.ReaderAt.
type bytesReaderAt struct {
	data []byte
}

func (b *bytesReaderAt) ReadAt(p []byte, off int64) (int, error) {
	if off >= int64(len(b.data)) {
		return 0, io.EOF
	}
	n := copy(p, b.data[off:])
	if n < len(p) {
		return n, io.EOF
	}
	return n, nil
}

func readerAtFromBytes(data []byte) io.ReaderAt {
	return &bytesReaderAt{data: data}
}
