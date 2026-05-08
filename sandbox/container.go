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
)

// Compile-time check: ContainerSandbox implements tool.ContainerExecutor.
var _ containerExecutor = (*ContainerSandbox)(nil)

type containerExecutor interface {
	Exec(ctx context.Context, command string, timeout time.Duration) (string, error)
	Running() bool
}

// ContainerSandbox executes commands inside a Docker container, providing
// full isolation. It supports dynamic Dockerfile generation for on-the-fly
// environment setup (inspired by herm's DevEnv pattern).
type ContainerSandbox struct {
	projectDir  string
	image       string
	containerID string
	mu          sync.Mutex
	running     bool
}

// NewContainerSandbox creates a container sandbox for the given project.
func NewContainerSandbox(projectDir string) *ContainerSandbox {
	return &ContainerSandbox{
		projectDir: projectDir,
		image:      resolveImage(projectDir),
	}
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

	args := []string{
		"run", "-d", "--rm",
		"--name", c.containerName(),
		"-v", c.projectDir + ":/workspace",
		"-w", "/workspace",
		c.image,
		"sleep", "infinity",
	}

	cmd := exec.CommandContext(ctx, "docker", args...)
	out, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("container start failed: %w", err)
	}
	c.containerID = strings.TrimSpace(string(out))
	c.running = true
	return nil
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

	cmd := exec.CommandContext(ctx, "docker", "exec", id, "sh", "-c", command)
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
	cmd := exec.Command("docker", "stop", c.containerID)
	cmd.Run()
	c.running = false
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

// BuildFromDockerfile builds a new image from a Dockerfile in the project.
// Returns the image tag that can be used for subsequent Start calls.
func (c *ContainerSandbox) BuildFromDockerfile(ctx context.Context, dockerfile string) (string, error) {
	hash := sha256.Sum256([]byte(dockerfile))
	tag := fmt.Sprintf("hawk-sandbox:%x", hash[:6])

	dfPath := filepath.Join(c.projectDir, ".hawk", "Dockerfile")
	if err := os.MkdirAll(filepath.Dir(dfPath), 0755); err != nil {
		return "", err
	}
	if err := os.WriteFile(dfPath, []byte(dockerfile), 0644); err != nil {
		return "", err
	}

	cmd := exec.CommandContext(ctx, "docker", "build", "-t", tag, "-f", dfPath, c.projectDir)
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
	c.Stop()
	return c.Start(ctx)
}

func (c *ContainerSandbox) containerName() string {
	base := filepath.Base(c.projectDir)
	hash := sha256.Sum256([]byte(c.projectDir))
	return fmt.Sprintf("hawk-%s-%x", base, hash[:4])
}

func resolveImage(projectDir string) string {
	dfPath := filepath.Join(projectDir, ".hawk", "Dockerfile")
	if _, err := os.Stat(dfPath); err == nil {
		content, err := os.ReadFile(dfPath)
		if err == nil && len(content) > 0 {
			hash := sha256.Sum256(content)
			return fmt.Sprintf("hawk-sandbox:%x", hash[:6])
		}
	}
	return "ubuntu:24.04"
}
