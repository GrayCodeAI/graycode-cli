package cmd

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/GrayCodeAI/hawk/internal/sandbox"
)

// containerStatusMsg carries container lifecycle updates to the TUI.
type containerStatusMsg struct {
	status  string
	ready   bool
	err     error
	sandbox *sandbox.ContainerSandbox
}

// shouldUseContainer determines if hawk should run in container mode.
// Default: container-first when Docker is available. Opt out with --no-container
// or HAWK_NO_CONTAINER=1 (useful on low-memory hosts where docker pull/build
// can trigger jetsam kills).
func shouldUseContainer() bool {
	if noContainer {
		return false
	}
	if containerMode {
		return true
	}
	if v := strings.TrimSpace(os.Getenv("HAWK_NO_CONTAINER")); v == "1" || strings.EqualFold(v, "true") {
		return false
	}
	return sandbox.DockerAvailable()
}

// bootContainerCmd starts the container in the background and sends status
// updates to the TUI (async boot with progress feedback).
func bootContainerCmd(projectDir string) tea.Cmd {
	return func() tea.Msg {
		cs := sandbox.NewContainerSandbox(projectDir)

		if !sandbox.DockerAvailable() {
			return containerStatusMsg{
				status: "docker not running",
				err:    fmt.Errorf("docker is not running: start Docker and try again"),
			}
		}

		// Only start when the image is already local. Pull/build during TUI
		// startup can spike memory (jetsam "killed" on 8GB Macs) and block chat.
		image := cs.Image()
		imgCtx, imgCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer imgCancel()
		checkCmd := exec.CommandContext(imgCtx, "docker", "image", "inspect", image)
		if checkCmd.Run() != nil {
			return containerStatusMsg{
				status: "image missing",
				err: fmt.Errorf(
					"container image %s is not local — run: docker pull %s\nOr restart with --no-container for host mode",
					image, image,
				),
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
