package cmd

import (
	"context"
	"fmt"
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
