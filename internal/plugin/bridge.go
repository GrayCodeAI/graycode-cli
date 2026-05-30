package plugin

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// PluginBridge wraps a shell-based bridge plugin, executing an external CLI
// binary as a subprocess. It mirrors the trace sessioncapture bridge pattern:
// it degrades gracefully when the binary is not found in PATH.
type PluginBridge struct {
	manifest *Manifest
	bin      string
	ready    bool
}

// NewPluginBridge locates the bridge command for a manifest and returns a bridge.
// Returns a non-ready bridge if the command is not found in PATH.
func NewPluginBridge(m *Manifest) *PluginBridge {
	pb := &PluginBridge{manifest: m}
	if m == nil || m.Bridge == nil || m.Bridge.Command == "" {
		return pb
	}
	path, err := exec.LookPath(m.Bridge.Command)
	if err != nil {
		return pb
	}
	pb.bin = path
	pb.ready = true
	return pb
}

// Ready reports whether the bridge command is available.
func (pb *PluginBridge) Ready() bool {
	return pb.ready
}

// Name returns the plugin name backing this bridge.
func (pb *PluginBridge) Name() string {
	if pb.manifest == nil {
		return ""
	}
	return pb.manifest.Name
}

// Run executes the bridge command with the manifest's default args plus the
// per-call args, merging the manifest's environment variables into the process
// environment. Returns trimmed stdout, or an error including stderr on failure.
func (pb *PluginBridge) Run(ctx context.Context, args ...string) (string, error) {
	if !pb.ready {
		return "", fmt.Errorf("bridge command %q not found in PATH", pb.commandName())
	}

	bridge := pb.manifest.Bridge
	fullArgs := make([]string, 0, len(bridge.Args)+len(args))
	fullArgs = append(fullArgs, bridge.Args...)
	fullArgs = append(fullArgs, args...)

	cmd := exec.CommandContext(ctx, pb.bin, fullArgs...)

	// Merge extra environment variables onto the inherited environment.
	if len(bridge.Env) > 0 {
		env := os.Environ()
		for k, v := range bridge.Env {
			env = append(env, k+"="+v)
		}
		cmd.Env = env
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("%s: %s", err, strings.TrimSpace(stderr.String()))
	}
	return strings.TrimSpace(stdout.String()), nil
}

func (pb *PluginBridge) commandName() string {
	if pb.manifest == nil || pb.manifest.Bridge == nil {
		return ""
	}
	return pb.manifest.Bridge.Command
}
