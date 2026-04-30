// Package sandbox provides sandbox mode for isolated command execution.
// This uses namespace/container isolation where available.
package sandbox

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

// Config describes sandbox configuration.
type Config struct {
	Enabled      bool     `json:"enabled"`
	Type         string   `json:"type"` // "namespace", "docker", "chroot", "none"
	AllowNetwork bool     `json:"allow_network"`
	AllowWrite   bool     `json:"allow_write"`
	ReadOnlyDirs []string `json:"read_only_dirs"`
	WritableDirs []string `json:"writable_dirs"`
	MaxMemoryMB  int      `json:"max_memory_mb"`
	MaxCPUPct    int      `json:"max_cpu_pct"`
}

// DefaultConfig returns a default sandbox configuration.
func DefaultConfig() *Config {
	return &Config{
		Enabled:      false,
		Type:         "none",
		AllowNetwork: true,
		AllowWrite:   true,
		MaxMemoryMB:  512,
		MaxCPUPct:    50,
	}
}

// Sandbox provides isolated execution environment.
type Sandbox struct {
	config *Config
	root   string
}

// New creates a new sandbox.
func New(config *Config) (*Sandbox, error) {
	if config == nil {
		config = DefaultConfig()
	}

	s := &Sandbox{config: config}
	if config.Enabled {
		if err := s.setup(); err != nil {
			return nil, err
		}
	}
	return s, nil
}

// IsAvailable checks if sandboxing is available on this system.
func IsAvailable() bool {
	switch runtime.GOOS {
	case "linux":
		// Check for namespace support
		if _, err := os.Stat("/proc/self/ns"); err == nil {
			return true
		}
		// Check for docker
		if _, err := exec.LookPath("docker"); err == nil {
			return true
		}
	case "darwin":
		// macOS has limited sandboxing via sandbox-exec
		if _, err := exec.LookPath("sandbox-exec"); err == nil {
			return true
		}
	}
	return false
}

// setup prepares the sandbox environment.
func (s *Sandbox) setup() error {
	// Create temp root for chroot/namespaces
	root, err := os.MkdirTemp("", "hawk-sandbox-*")
	if err != nil {
		return err
	}
	s.root = root

	switch s.config.Type {
	case "chroot":
		return s.setupChroot()
	case "namespace":
		return s.setupNamespace()
	case "docker":
		return nil // Docker doesn't need local setup
	}
	return nil
}

// setupChroot prepares a chroot environment.
func (s *Sandbox) setupChroot() error {
	// Copy essential binaries
	binaries := []string{"/bin/sh", "/bin/bash", "/usr/bin/env"}
	for _, bin := range binaries {
		if _, err := os.Stat(bin); err == nil {
			dest := filepath.Join(s.root, bin)
			os.MkdirAll(filepath.Dir(dest), 0o755)
			copyFile(bin, dest)
		}
	}
	return nil
}

// setupNamespace prepares a namespace environment.
func (s *Sandbox) setupNamespace() error {
	// Namespaces are created per-command, no pre-setup needed
	return nil
}

// Run executes a command in the sandbox.
func (s *Sandbox) Run(ctx context.Context, command string) (*exec.Cmd, error) {
	if !s.config.Enabled {
		return exec.CommandContext(ctx, "bash", "-c", command), nil
	}

	switch s.config.Type {
	case "docker":
		return s.runDocker(ctx, command)
	case "namespace":
		return s.runNamespace(ctx, command)
	case "chroot":
		return s.runChroot(ctx, command)
	default:
		return exec.CommandContext(ctx, "bash", "-c", command), nil
	}
}

// runDocker runs a command in a Docker container.
func (s *Sandbox) runDocker(ctx context.Context, command string) (*exec.Cmd, error) {
	args := []string{
		"run", "--rm",
		"-v", fmt.Sprintf("%s:/workspace", s.config.ReadOnlyDirs[0]),
		"-w", "/workspace",
		"--memory", fmt.Sprintf("%dm", s.config.MaxMemoryMB),
		"--cpus", fmt.Sprintf("%d", s.config.MaxCPUPct/100),
	}
	if !s.config.AllowNetwork {
		args = append(args, "--network", "none")
	}
	args = append(args, "alpine:latest", "sh", "-c", command)
	return exec.CommandContext(ctx, "docker", args...), nil
}

// runNamespace runs a command in a Linux namespace.
func (s *Sandbox) runNamespace(ctx context.Context, command string) (*exec.Cmd, error) {
	args := []string{
		"--fork",
		"--pid",
		"--mount-proc",
	}
	if !s.config.AllowNetwork {
		args = append(args, "--net")
	}
	args = append(args, "sh", "-c", command)
	return exec.CommandContext(ctx, "unshare", args...), nil
}

// runChroot runs a command in a chroot.
func (s *Sandbox) runChroot(ctx context.Context, command string) (*exec.Cmd, error) {
	return exec.CommandContext(ctx, "chroot", s.root, "bash", "-c", command), nil
}

// Close cleans up sandbox resources.
func (s *Sandbox) Close() error {
	if s.root != "" {
		return os.RemoveAll(s.root)
	}
	return nil
}

// copyFile copies a file from src to dst.
func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0o755)
}
