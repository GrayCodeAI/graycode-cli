package sandbox

import (
	"context"
	"crypto/sha256"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/GrayCodeAI/hawk/internal/storage"
)

// Compile-time check: ContainerSandbox implements tool.ContainerExecutor.
var _ containerExecutor = (*ContainerSandbox)(nil)

type containerExecutor interface {
	Exec(ctx context.Context, command string, timeout time.Duration) (string, error)
	Running() bool
}

var forceRemoveContainer = func(ctx context.Context, containerID string) error {
	cmd := exec.CommandContext(ctx, "docker", "rm", "-f", containerID) // #nosec G204 -- "docker" binary fixed; containerID is our own tracked container ID
	return cmd.Run()
}

// usernsProbe reports whether the Docker daemon has user-namespace remapping
// enabled (docker info --format '{{.SecurityOptions}}' contains "userns").
// Injecting it lets tests exercise both branches without a real Docker daemon.
var usernsProbe = func() (bool, error) {
	cmd := exec.Command("docker", "info", "-f", "{{.SecurityOptions}}") // #nosec G204 -- fixed docker binary and probe args
	out, err := cmd.Output()
	if err != nil {
		return false, err
	}
	return strings.Contains(strings.ToLower(string(out)), "userns"), nil
}

var (
	usernsOnce sync.Once
	usernsOK   bool
)

// usernsRemapAvailable reports whether --userns-remap can be used (H16). The
// result is cached for the process: the daemon's userns configuration is
// static for the host. When unavailable, the fallback is the documented
// host-kernel sharing (no flag added).
func usernsRemapAvailable() bool {
	usernsOnce.Do(func() {
		if ok, err := usernsProbe(); err == nil {
			usernsOK = ok
		}
	})
	return usernsOK
}

// resetUsernsCache clears the cached userns probe result (tests only).
func resetUsernsCache() {
	usernsOnce = sync.Once{}
	usernsOK = false
}

// ContainerSandbox executes commands inside a Docker container, providing
// full isolation. It supports dynamic Dockerfile generation for on-the-fly
// environment setup.
type ContainerSandbox struct {
	projectDir  string
	image       string
	containerID string
	mu          sync.Mutex
	running     bool
	// runtime carries declarative runtime_extra_deps / runtime_startup_env_vars.
	// The empty value reproduces the prior behavior.
	runtime RuntimeConfig
	// networkMode is the Docker network mode: "bridge" (default), "none", or
	// "isolated" (per-container network). HAWK_CONTAINER_NETWORK env var still
	// overrides when set.
	networkMode string
	// isolatedNetName is the per-container Docker network name when networkMode
	// == "isolated". Created on Start, removed on Stop.
	isolatedNetName string
	// credentialGate manages approval-gated access to host credentials.
	credentialGate *CredentialGate
}

// NewContainerSandbox creates a container sandbox for the given project.
func NewContainerSandbox(projectDir string) *ContainerSandbox {
	return &ContainerSandbox{
		projectDir: projectDir,
		image:      resolveImage(projectDir),
		runtime:    LoadRuntimeConfig(projectDir),
	}
}

// CredentialGate returns the container's credential gate, creating it on
// first use. The gate is initialized with the descriptors that have staging
// mounts available.
func (c *ContainerSandbox) CredentialGate() *CredentialGate {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.credentialGate == nil {
		c.credentialGate = NewCredentialGate(c)
	}
	return c.credentialGate
}

// SetNetworkMode sets the container network mode: "bridge" (default), "none"
// (no network), or "isolated" (per-container Docker network so concurrent
// containers can't probe each other). The env var HAWK_CONTAINER_NETWORK still
// overrides when set at Start time.
func (c *ContainerSandbox) SetNetworkMode(mode string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.networkMode = mode
}

// NetworkMode returns the current network mode.
func (c *ContainerSandbox) NetworkMode() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.networkMode
}

// SetRuntimeConfig overrides the declarative runtime config (extra deps and
// startup env vars). Additive: an empty config restores prior behavior.
func (c *ContainerSandbox) SetRuntimeConfig(cfg RuntimeConfig) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.runtime = cfg
}

// DockerAvailable returns true if Docker daemon is reachable.
func DockerAvailable() bool {
	return dockerAvailable()
}

// Start launches the container with the project directory mounted.
func (c *ContainerSandbox) Start(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.running {
		return nil
	}

	name := c.containerName()

	// Remove any stale container with the same name from a previous session (best-effort)
	_, _ = exec.CommandContext(ctx, "docker", "rm", "-f", name).CombinedOutput() // #nosec G204 -- "docker" binary fixed; name is derived from a hash of the project dir

	// For isolated networking, create a per-container Docker network so
	// concurrent containers cannot reach each other.
	if c.networkMode == "isolated" {
		netName := "hawk-net-" + name
		_, _ = exec.CommandContext(ctx, "docker", "network", "create", "--driver", "bridge", netName).CombinedOutput() // #nosec G204 -- fixed docker binary; netName derived from container name
		c.isolatedNetName = netName
	}

	// Create attachments and cache dirs outside the project workspace.
	attachDir := filepath.Join(storage.ProjectStateDir(c.projectDir), "attachments")
	cacheDir := filepath.Join(storage.ProjectCacheDir(c.projectDir), "container")
	_ = os.MkdirAll(attachDir, 0o750)
	_ = os.MkdirAll(cacheDir, 0o750)

	args := c.dockerRunArgs(name, attachDir, cacheDir)

	cmd := exec.CommandContext(ctx, "docker", args...) // #nosec G204 -- "docker" binary fixed; args built from internal config (project dir, image, runtime env)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("container start failed: %s", strings.TrimSpace(string(out)))
	}
	c.containerID = strings.TrimSpace(string(out))
	c.running = true

	// Set up the credential access layout inside the running container:
	// staging mounts are already in place; create the denied placeholder
	// and point all credential paths at it.
	if err := c.SetupCredentials(); err != nil {
		// Non-fatal: credentials can still be set up later, but log it.
		slog.Warn("credential layout setup failed", "error", err)
	}
	return nil
}

// SetupCredentials initializes the denied-placeholder symlinks for all
// credentials that have staging mounts available. It is idempotent.
func (c *ContainerSandbox) SetupCredentials() error {
	var available []CredentialDescriptor
	for _, desc := range Registry() {
		// Check if the staging mount is populated (the credential exists on host).
		staging := StagingPath(desc.ID)
		if _, err := os.Stat(staging); err == nil {
			available = append(available, desc)
		}
	}
	if len(available) == 0 {
		return nil
	}
	return InitCredentialLayout(available)
}

func (c *ContainerSandbox) dockerRunArgs(name, attachDir, cacheDir string) []string {
	// Called from Start which already holds c.mu; read networkMode without re-locking.
	netMode := strings.TrimSpace(os.Getenv("HAWK_CONTAINER_NETWORK"))
	if netMode == "" {
		mode := c.networkMode
		if mode != "" {
			netMode = mode
		} else {
			netMode = "bridge"
		}
	}
	// "isolated" is not a native Docker network mode — it means create a
	// per-container bridge network. We resolve it to "none" here if the
	// isolated network hasn't been created yet; Start wires it up first.
	args := []string{
		"run", "-d", "--rm",
		"--name", name,
		"--network", netMode,
		"--cap-drop", "ALL",
		"--security-opt", "no-new-privileges",
		"--pids-limit", "256",
		"--read-only",
		"--tmpfs", "/tmp:rw,noexec,nosuid,nodev,size=64m",
		"-v", c.projectDir + ":" + c.projectDir, // mount at same path
		"-v", attachDir + ":/attachments:ro",
		"-v", cacheDir + ":/cache",
		"-w", c.projectDir,
		"--entrypoint", "sleep",
	}
	// User-namespace remapping further isolates the container from the host
	// kernel (H16); only added when the daemon supports it. Without userns
	// the container would run as root against the rw project mount, so fall
	// back to --user with the host uid:gid (M12). exec.CommandContext runs
	// the container process as that uid inside the container regardless of
	// whether /etc/passwd knows it.
	if usernsRemapAvailable() {
		args = append(args, "--userns-remap", "default")
	} else {
		args = append(args, "--user", fmt.Sprintf("%d:%d", os.Getuid(), os.Getgid()))
	}
	// SSH Agent Socket Passthrough: Forward host SSH auth socket so git push/fetch works
	// over SSH without copying or mounting raw SSH private keys into the container.
	if sshSock := os.Getenv("SSH_AUTH_SOCK"); sshSock != "" {
		if _, err := os.Stat(sshSock); err == nil {
			args = append(args, "-v", sshSock+":/ssh-agent.sock:ro", "-e", "SSH_AUTH_SOCK=/ssh-agent.sock")
		}
	}

	// Credential staging mounts: mount each existing host credential path
	// read-only into the container's staging area. Access is gated by
	// symlinks (see credentials.go); the mounts are just the raw material.
	args = append(args, c.credentialMountArgs()...)

	args = append(args, c.runtime.StartupEnvArgs()...)
	args = append(args, c.image, "infinity")
	return args
}

// credentialMountArgs returns the -v flags for mounting host credentials into
// the staging area. Only credentials whose host paths exist are mounted.
// Each is mounted read-only (:ro) so the container cannot mutate the host copy.
func (c *ContainerSandbox) credentialMountArgs() []string {
	var args []string
	home := os.Getenv("HOME")
	if home == "" {
		home = c.projectDir // fallback
	}
	for _, desc := range Registry() {
		hostPath := desc.HostPath
		if strings.HasPrefix(hostPath, "~") {
			hostPath = filepath.Join(home, hostPath[1:])
		}
		// Only mount if the host path exists.
		if _, err := os.Stat(hostPath); err != nil {
			continue
		}
		staging := StagingPath(desc.ID)
		args = append(args, "-v", hostPath+":"+staging+":ro")
	}
	return args
}

// Exec runs a command inside the container and returns its output.
func (c *ContainerSandbox) Exec(ctx context.Context, command string, timeout time.Duration) (string, error) {
	c.mu.Lock()
	if !c.running {
		c.mu.Unlock()
		return "", fmt.Errorf("container not running")
	}
	id := c.containerID
	c.mu.Unlock()

	if timeout == 0 {
		timeout = 120 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "docker", "exec", id, "sh", "-c", command) // #nosec G204 -- "docker" binary fixed; id is our own container ID and command is the agent's sandboxed exec request
	out, err := cmd.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		return string(out), fmt.Errorf("command timed out after %s", timeout)
	}
	if err != nil {
		return string(out), err
	}
	return string(out), nil
}

// Stop terminates the container.
func (c *ContainerSandbox) Stop() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.running {
		return nil
	}
	// Force-remove our ephemeral --rm container instead of waiting through
	// Docker's default stop grace period. Bound cleanup as well so exiting the
	// CLI can never hang indefinitely on an unresponsive daemon.
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_ = forceRemoveContainer(ctx, c.containerID)
	// Clean up the per-container isolated network (best-effort).
	if c.isolatedNetName != "" {
		_, _ = exec.CommandContext(ctx, "docker", "network", "rm", c.isolatedNetName).CombinedOutput() // #nosec G204 -- fixed docker binary; isolatedNetName is our own network name
		c.isolatedNetName = ""
	}
	c.running = false
	c.containerID = ""
	return nil
}

// Running reports whether the container is active.
func (c *ContainerSandbox) Running() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.running
}

// ContainerID returns the full container ID (empty if not running).
func (c *ContainerSandbox) ContainerID() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.containerID
}

// Image returns the current image name.
func (c *ContainerSandbox) Image() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.image
}

// SetImage updates the image to use for the next Start call.
func (c *ContainerSandbox) SetImage(img string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.image = img
}

// BuildFromDockerfile builds a new image from a Dockerfile in the project.
// Returns the image tag that can be used for subsequent Start calls.
func (c *ContainerSandbox) BuildFromDockerfile(ctx context.Context, dockerfile string) (string, error) {
	// Append declarative runtime_extra_deps RUN layers (no-op when empty).
	c.mu.Lock()
	dockerfile = c.runtime.AppendExtraDeps(dockerfile)
	c.mu.Unlock()

	hash := sha256.Sum256([]byte(dockerfile))
	tag := fmt.Sprintf("hawk-sandbox:%x", hash[:6])

	dfPath := filepath.Join(storage.ProjectStateDir(c.projectDir), "Dockerfile")
	if err := os.MkdirAll(filepath.Dir(dfPath), 0o750); err != nil {
		return "", err
	}
	if err := os.WriteFile(dfPath, []byte(dockerfile), 0o600); err != nil { // #nosec G703 -- path is the managed project-state Dockerfile
		return "", err
	}

	cmd := exec.CommandContext(ctx, "docker", "build", "-t", tag, "-f", dfPath, c.projectDir) // #nosec G204 -- "docker" binary fixed; tag/dfPath/projectDir derived from internal state
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("docker build failed: %s\n%s", err, out)
	}

	c.mu.Lock()
	c.image = tag
	c.mu.Unlock()
	return tag, nil
}

// HotSwap stops the current container and starts a new one with the updated image.
func (c *ContainerSandbox) HotSwap(ctx context.Context) error {
	_ = c.Stop()
	return c.Start(ctx)
}

func (c *ContainerSandbox) containerName() string {
	hash := sha256.Sum256([]byte(c.projectDir))
	return fmt.Sprintf("hawk-%x", hash[:4])
}

func resolveImage(projectDir string) string {
	dfPath := filepath.Join(storage.ProjectStateDir(projectDir), "Dockerfile")
	if _, err := os.Stat(dfPath); err == nil {
		content, err := os.ReadFile(dfPath) // #nosec G304 -- dfPath is derived from the project's own state dir, not external input
		if err == nil && len(content) > 0 {
			hash := sha256.Sum256(content)
			return fmt.Sprintf("hawk-sandbox:%x", hash[:6])
		}
	}
	return defaultHawkImage()
}
