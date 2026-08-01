package sandbox

import (
	"context"
	"crypto/sha256"
	"fmt"
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
}

// NewContainerSandbox creates a container sandbox for the given project.
func NewContainerSandbox(projectDir string) *ContainerSandbox {
	return &ContainerSandbox{
		projectDir: projectDir,
		image:      resolveImage(projectDir),
		runtime:    LoadRuntimeConfig(projectDir),
	}
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
	return nil
}

func (c *ContainerSandbox) dockerRunArgs(name, attachDir, cacheDir string) []string {
	args := []string{
		"run", "-d", "--rm",
		"--name", name,
		"--network", "none",
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
	args = append(args, c.runtime.StartupEnvArgs()...)
	args = append(args, c.image, "infinity")
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
