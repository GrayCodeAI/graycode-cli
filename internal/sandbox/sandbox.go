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
	"sync"
)

var (
	seatbeltTmpFiles   []string
	seatbeltTmpFilesMu sync.Mutex
)

// Config describes sandbox configuration.
type Config struct {
	Enabled      bool     `json:"enabled"`
	Type         string   `json:"type"` // "namespace", "docker", "chroot", "seatbelt", "none"
	AllowNetwork bool     `json:"allow_network"`
	AllowWrite   bool     `json:"allow_write"`
	Security     Security `json:"tier"` // security posture (strict / workspace / off)
	ReadOnlyDirs []string `json:"read_only_dirs"`
	WritableDirs []string `json:"writable_dirs"`
	MaxMemoryMB  int      `json:"max_memory_mb"`
	MaxCPUPct    int      `json:"max_cpu_pct"`
}

// DefaultConfig returns a default sandbox configuration. The new
// default is TierWorkspace (allow workspace writes, deny process
// exec) which is safer than the legacy TierOff behavior. Users who
// need the full legacy behavior (process exec + writes) can set
// Tier=TierOff in their config.
func DefaultConfig() *Config {
	return &Config{
		Enabled:      true,
		Type:         "auto",
		AllowNetwork: true,
		AllowWrite:   true, // legacy field; security takes precedence
		Security:     SecurityWorkspace,
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
			_ = os.MkdirAll(filepath.Dir(dest), 0o750)
			_ = copyFile(bin, dest)
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
		// Fail closed: a disabled sandbox must not silently fall back to host
		// execution. Only an explicit security=off opt-out allows running on
		// the host; anything else is a misconfiguration (e.g. no backend).
		if s.config.Security != SecurityOff {
			return nil, fmt.Errorf("sandbox is disabled and not explicitly opted out; set security=off to allow host execution")
		}
		return exec.CommandContext(ctx, "bash", "-c", command), nil // #nosec G204 -- intentional host execution behind explicit security=off opt-out
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
		return nil, fmt.Errorf("no sandbox backend available; install a supported backend (docker, unshare, sandbox-exec) or use --sandbox off to explicitly disable sandboxing")
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
		// Hardening (H16): drop all capabilities, forbid privilege
		// escalation, make the rootfs read-only (bind mounts below keep
		// their own :rw/:ro flags, so /workspace stays writable) and give
		// /tmp scratch space.
		"--cap-drop", "ALL",
		"--security-opt", "no-new-privileges",
		"--read-only",
		"--tmpfs", "/tmp:rw,noexec,nosuid,nodev,size=64m",
		"-v", fmt.Sprintf("%s:/workspace:rw", workDir),
		"-w", "/workspace",
		"--memory", fmt.Sprintf("%dm", s.config.MaxMemoryMB),
		"--cpus", fmt.Sprintf("%.2f", float64(s.config.MaxCPUPct)/100.0),
	}
	// `--userns-remap` is a daemon configuration flag, not accepted by
	// `docker run`. Run as the invoking host UID/GID in either daemon mode.
	args = append(args, "--user", fmt.Sprintf("%d:%d", os.Getuid(), os.Getgid()))
	if !s.config.AllowNetwork {
		args = append(args, "--network", "none")
	}
	// Pin to a specific stable version; update periodically for security patches.
	args = append(args, "alpine:3.21.3", "sh", "-c", command)
	return exec.CommandContext(ctx, "docker", args...), nil // #nosec G204 -- "docker" binary and args built from internal config/constants
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
	return exec.CommandContext(ctx, "unshare", args...), nil // #nosec G204 -- fixed sandbox executable
}

// runChroot runs a command in a chroot.
func (s *Sandbox) runChroot(ctx context.Context, command string) (*exec.Cmd, error) {
	return exec.CommandContext(ctx, "chroot", s.root, "bash", "-c", command), nil // #nosec G204 -- "chroot" binary is fixed; s.root is a sandbox temp dir we created
}

// runSeatbelt runs a command in a macOS seatbelt sandbox using sandbox-exec.
func (s *Sandbox) runSeatbelt(ctx context.Context, command string) (*exec.Cmd, error) {
	workDir, _ := os.Getwd()
	if len(s.config.ReadOnlyDirs) > 0 {
		workDir = s.config.ReadOnlyDirs[0]
	}

	policy := DefaultHawkPolicy(workDir, s.config.Security)
	policy.AllowNetwork = s.config.AllowNetwork
	// NOTE: AllowWrite is now set by DefaultHawkPolicy based on the
	// security (SecurityWorkspace → true, SecurityStrict → false). The legacy
	// Config.AllowWrite field is preserved for JSON backward compat
	// but no longer overrides the security.

	// Add configured readable dirs.
	policy.ReadablePaths = append(policy.ReadablePaths, s.config.ReadOnlyDirs...)

	// Add configured writable dirs.
	policy.WritablePaths = append(policy.WritablePaths, s.config.WritableDirs...)

	return RunSeatbelted(ctx, command, policy)
}

// Available returns true if any sandbox mechanism is available on this system.
// This is a convenience alias for IsAvailable.
func Available() bool {
	return IsAvailable()
}

// WrapCommand wraps a shell command string with sandbox isolation based on
// the provided SandboxConfig. It returns the executable name and argument
// list suitable for exec.Command, or an error if no sandbox backend is available.
func WrapCommand(command string, cfg SandboxConfig) (string, []string, error) {
	// Resolve the security once. Empty string (legacy callers that
	// don't know about Security) keeps the old SecurityOff behavior;
	// new callers can pass SecurityWorkspace to get the safer
	// default. This makes the new Config.Security=SecurityWorkspace
	// default effective through the legacy SandboxConfig path.
	security := cfg.Security
	if security == "" {
		security = SecurityOff
	}
	switch runtime.GOOS {
	case "darwin":
		if SeatbeltAvailable() {
			// Use a cached profile temp file per tier so repeated commands
			// reuse the same file instead of writing one per invocation.
			profilePath, err := getCachedProfile(security)
			if err == nil {
				return "sandbox-exec", []string{"-f", profilePath, "bash", "-c", command}, nil
			}
			// Fallback: write a fresh profile if caching fails.
			workDir := cfg.WorkspaceDir
			if workDir == "" {
				workDir, _ = os.Getwd()
			}
			policy := DefaultHawkPolicy(workDir, security)
			policy.AllowNetwork = cfg.AllowNetwork
			tmpFile, err := os.CreateTemp("", "hawk-seatbelt-*.sb")
			if err == nil {
				profile := GenerateSeatbeltProfile(policy)
				_, _ = tmpFile.WriteString(profile)
				_ = tmpFile.Close()
				seatbeltTmpFilesMu.Lock()
				seatbeltTmpFiles = append(seatbeltTmpFiles, tmpFile.Name())
				seatbeltTmpFilesMu.Unlock()
				return "sandbox-exec", []string{"-f", tmpFile.Name(), "bash", "-c", command}, nil
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
			return "unshare", args, nil
		}
	}
	// Fail-closed: refuse to run without sandboxing
	return "", nil, fmt.Errorf("no sandbox backend available; install a supported backend (docker, unshare, sandbox-exec) or use --sandbox off to explicitly disable sandboxing")
}

// Close cleans up sandbox resources.
func (s *Sandbox) Close() error {
	// Clean up any seatbelt temp files
	seatbeltTmpFilesMu.Lock()
	for _, f := range seatbeltTmpFiles {
		_ = os.Remove(f)
	}
	seatbeltTmpFiles = nil
	seatbeltTmpFilesMu.Unlock()

	if s.root != "" {
		return os.RemoveAll(s.root)
	}
	return nil
}

// copyFile copies a file from src to dst.
func copyFile(src, dst string) error {
	data, err := os.ReadFile(src) // #nosec G304 -- src is one of a fixed set of well-known binary paths (/bin/sh, /bin/bash, /usr/bin/env)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0o755) // #nosec G703,G306 -- dst is constructed beneath the private sandbox root; binary must remain executable
}
