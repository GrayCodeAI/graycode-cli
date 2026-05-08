package cmd

import (
	"context"
	"fmt"
	"os/exec"
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

		// Ensure image exists locally, pull if needed (like herm)
		image := cs.Image()
		pullCtx, pullCancel := context.WithTimeout(context.Background(), 300*time.Second)
		defer pullCancel()
		checkCmd := exec.CommandContext(pullCtx, "docker", "image", "inspect", image)
		if checkCmd.Run() != nil {
			// Image not available locally — pull it
			pullCmd := exec.CommandContext(pullCtx, "docker", "pull", image)
			if err := pullCmd.Run(); err != nil {
				// Fall back to ubuntu:24.04 if custom image can't be pulled
				cs.SetImage("ubuntu:24.04")
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
