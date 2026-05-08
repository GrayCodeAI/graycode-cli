package cmd

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/GrayCodeAI/hawk/sandbox"
)

// containerStatusMsg carries container lifecycle updates to the TUI.
type containerStatusMsg struct {
	status  string
	ready   bool
	err     error
	sandbox *sandbox.ContainerSandbox
}

// buildHawkImage builds the hawk container image from the bundled Dockerfile.
// It writes the Dockerfile to a temp dir and runs docker build.
func buildHawkImage(ctx context.Context, tag string) bool {
	dockerfile := `FROM ubuntu:24.04
RUN apt-get update && apt-get install -y --no-install-recommends \
    git curl wget jq tree ripgrep fd-find make gcc \
    python3 python3-pip nodejs npm ca-certificates openssh-client \
    && rm -rf /var/lib/apt/lists/* \
    && ln -sf /usr/bin/fdfind /usr/bin/fd
ENV TERM=xterm-256color LANG=C.UTF-8
`
	// Use platform-appropriate arch
	platform := runtime.GOARCH
	if platform == "arm64" {
		platform = "linux/arm64"
	} else {
		platform = "linux/amd64"
	}

	tmpDir, err := os.MkdirTemp("", "hawk-build-")
	if err != nil {
		return false
	}
	defer os.RemoveAll(tmpDir)

	dfPath := filepath.Join(tmpDir, "Dockerfile")
	if err := os.WriteFile(dfPath, []byte(dockerfile), 0644); err != nil {
		return false
	}

	cmd := exec.CommandContext(ctx, "docker", "build", "--platform", platform, "-t", tag, "-f", dfPath, tmpDir)
	return cmd.Run() == nil
}

// shouldUseContainer determines if hawk should run in container mode.
// Default: YES if Docker is available (like herm). User can override with --no-container.
func shouldUseContainer() bool {
	if noContainer {
		return false
	}
	if containerMode {
		return true
	}
	// Default: auto-detect. If Docker is running, use container mode.
	return sandbox.DockerAvailable()
}

// bootContainerCmd starts the container in the background and sends status
// updates to the TUI (herm-style async boot with progress feedback).
func bootContainerCmd(projectDir string) tea.Cmd {
	return func() tea.Msg {
		cs := sandbox.NewContainerSandbox(projectDir)

		if !sandbox.DockerAvailable() {
			return containerStatusMsg{
				status: "docker not running",
				err:    fmt.Errorf("Docker is not running. Start Docker and try again."),
			}
		}

		// Ensure image exists locally — pull or build as needed (like herm)
		image := cs.Image()
		imgCtx, imgCancel := context.WithTimeout(context.Background(), 300*time.Second)
		defer imgCancel()
		checkCmd := exec.CommandContext(imgCtx, "docker", "image", "inspect", image)
		if checkCmd.Run() != nil {
			// Image not available locally — try pull first
			pullCmd := exec.CommandContext(imgCtx, "docker", "pull", image)
			if pullCmd.Run() != nil {
				// Pull failed — build from bundled Dockerfile
				built := buildHawkImage(imgCtx, image)
				if !built {
					return containerStatusMsg{
						status: "image build failed",
						err:    fmt.Errorf("could not pull or build %s", image),
					}
				}
			}
		}

		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		if err := cs.Start(ctx); err != nil {
			return containerStatusMsg{
				status: "start failed",
				err:    fmt.Errorf("container start failed: %w", err),
			}
		}

		cid := cs.ContainerID()
		shortID := cid
		if len(shortID) > 12 {
			shortID = shortID[:12]
		}

		return containerStatusMsg{
			status:  shortID,
			ready:   true,
			sandbox: cs,
		}
	}
}
