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
	"time"
)

// Config describes sandbox configuration.
type Config struct {
	Enabled      bool     `json:"enabled"`
	Type         string   `json:"type"` // "namespace", "docker", "chroot", "seatbelt", "none"
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
		Enabled:      true,
		Type:         "auto",
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
			_ = os.MkdirAll(filepath.Dir(dest), 0o755)
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

	// Auto-select the best available sandbox backend.
	sandboxType := s.config.Type
	if sandboxType == "" || sandboxType == "none" || sandboxType == "auto" {
		selection := SelectSandbox(IsolationDefault, "")
		sandboxType = selection.Backend
	}

	switch sandboxType {
	case "docker":
		return s.runDocker(ctx, command)
	case "namespace":
		return s.runNamespace(ctx, command)
	case "chroot":
		return s.runChroot(ctx, command)
	case "seatbelt":
		return s.runSeatbelt(ctx, command)
	default:
		return exec.CommandContext(ctx, "bash", "-c", command), nil
	}
}

// runDocker runs a command in a Docker container.
func (s *Sandbox) runDocker(ctx context.Context, command string) (*exec.Cmd, error) {
	workDir, _ := os.Getwd()
	if len(s.config.ReadOnlyDirs) > 0 {
		workDir = s.config.ReadOnlyDirs[0]
	}
	args := []string{
		"run", "--rm",
		"-v", fmt.Sprintf("%s:/workspace", workDir),
		"-w", "/workspace",
		"--memory", fmt.Sprintf("%dm", s.config.MaxMemoryMB),
		"--cpus", fmt.Sprintf("%.2f", float64(s.config.MaxCPUPct)/100.0),
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

// runSeatbelt runs a command in a macOS seatbelt sandbox using sandbox-exec.
func (s *Sandbox) runSeatbelt(ctx context.Context, command string) (*exec.Cmd, error) {
	workDir, _ := os.Getwd()
	if len(s.config.ReadOnlyDirs) > 0 {
		workDir = s.config.ReadOnlyDirs[0]
	}

	policy := DefaultHawkPolicy(workDir)
	policy.AllowNetwork = s.config.AllowNetwork
	policy.AllowWrite = s.config.AllowWrite

	// Add configured readable dirs.
	for _, d := range s.config.ReadOnlyDirs {
		policy.ReadablePaths = append(policy.ReadablePaths, d)
	}

	// Add configured writable dirs.
	for _, d := range s.config.WritableDirs {
		policy.WritablePaths = append(policy.WritablePaths, d)
	}

	return RunSeatbelted(ctx, command, policy)
}

// Available returns true if any sandbox mechanism is available on this system.
// This is a convenience alias for IsAvailable.
func Available() bool {
	return IsAvailable()
}

// WrapCommand wraps a shell command string with sandbox isolation based on
// the provided SandboxConfig. It returns the executable name and argument
// list suitable for exec.Command.
func WrapCommand(command string, cfg SandboxConfig) (string, []string) {
	switch runtime.GOOS {
	case "darwin":
		if SeatbeltAvailable() {
			workDir := cfg.WorkspaceDir
			if workDir == "" {
				workDir, _ = os.Getwd()
			}
			policy := DefaultHawkPolicy(workDir)
			policy.AllowNetwork = cfg.AllowNetwork
			// Write profile to temp file
			tmpFile, err := os.CreateTemp("", "hawk-seatbelt-*.sb")
			if err == nil {
				profile := GenerateSeatbeltProfile(policy)
				_, _ = tmpFile.WriteString(profile)
				_ = tmpFile.Close()
				// Schedule cleanup after command completes.
				go func() {
					time.Sleep(5 * time.Minute)
					_ = os.Remove(tmpFile.Name())
				}()
				return "sandbox-exec", []string{"-f", tmpFile.Name(), "bash", "-c", command}
			}
		}
	case "linux":
		// Use unshare if available
		if _, err := exec.LookPath("unshare"); err == nil {
			args := []string{"--fork", "--pid", "--mount-proc"}
			if !cfg.AllowNetwork {
				args = append(args, "--net")
			}
			args = append(args, "bash", "-c", command)
			return "unshare", args
		}
	}
	// Fallback: run without sandboxing
	return "bash", []string{"-c", command}
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
