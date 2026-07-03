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
	// Default to container-first and let bootContainerCmd probe Docker asynchronously
	// after the TUI is already visible.
	return true
}

// bootContainerCmd starts the container in the background and sends status
// updates to the TUI (async boot with progress feedback).
// Retries up to 3 times with exponential backoff (0s, 2s, 4s) before
// falling back to host mode.
func bootContainerCmd(projectDir string) tea.Cmd {
	const maxAttempts = 3
	backoff := []time.Duration{0, 2 * time.Second, 4 * time.Second}

	return func() tea.Msg {
		for attempt := 0; attempt < maxAttempts; attempt++ {
			if attempt > 0 {
				time.Sleep(backoff[attempt])
			}

			cs := sandbox.NewContainerSandbox(projectDir)

			if !sandbox.DockerAvailable() {
				if attempt < maxAttempts-1 {
					continue
				}
				return containerStatusMsg{
					status: "docker not running",
					err:    fmt.Errorf("docker is not running: start Docker and try again"),
				}
			}

			// Check image is local
			image := cs.Image()
			imgCtx, imgCancel := context.WithTimeout(context.Background(), 10*time.Second)
			checkCmd := exec.CommandContext(imgCtx, "docker", "image", "inspect", image)
			imgErr := checkCmd.Run()
			imgCancel()
			if imgErr != nil {
				if attempt < maxAttempts-1 {
					continue
				}
				return containerStatusMsg{
					status: "image missing",
					err: fmt.Errorf(
						"container image %s is not local — run: docker pull %s\nOr restart with --no-container for host mode",
						image, image,
					),
				}
			}

			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			if err := cs.Start(ctx); err != nil {
				cancel()
				if attempt < maxAttempts-1 {
					continue
				}
				cancel()
				return containerStatusMsg{
					status: "start failed",
					err:    fmt.Errorf("container start failed: %w", err),
				}
			}
			cancel()

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

		return containerStatusMsg{
			status: "retry exhausted",
			err:    fmt.Errorf("container failed after %d attempts — restart with --no-container for host mode", maxAttempts),
		}
	}
}
