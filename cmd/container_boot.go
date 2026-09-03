package cmd

import (
	"context"
	"fmt"
	"os"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/GrayCodeAI/graycode-cli/internal/engine"
	"github.com/GrayCodeAI/graycode-cli/internal/sandbox"
)

// containerStatusMsg carries container lifecycle updates to the TUI.
type containerStatusMsg struct {
	status  string
	ready   bool
	err     error
	sandbox *sandbox.ContainerSandbox
}

var dockerAvailable = sandbox.DockerAvailable

// shouldUseContainer is intentionally unconditional: Graycode agent command
// execution is Docker-only and never falls back to the host.
func shouldUseContainer() bool {
	return true
}

// startRequiredContainer starts Graycode's mandatory Docker sandbox. It fails
// closed with an actionable error; there is deliberately no host fallback.
// Egress is restricted to a domain allowlist via NetworkProxy by default; set
// GRAYCODE_DISABLE_EGRESS_PROXY=1 to opt out (unrestricted bridge egress).
func startRequiredContainer(projectDir string) (*sandbox.ContainerSandbox, error) {
	var cs *sandbox.ContainerSandbox
	if os.Getenv("GRAYCODE_DISABLE_EGRESS_PROXY") == "1" {
		cs = sandbox.NewContainerSandbox(projectDir)
	} else {
		cs = sandbox.NewContainerSandboxWithEgressProxy(projectDir)
	}
	sandbox.ResetDockerAvailabilityCache()
	if !dockerAvailable() {
		return nil, fmt.Errorf("docker is required but is not running — start Docker and retry")
	}

	imageCtx, imageCancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer imageCancel()
	if _, err := cs.EnsureImage(imageCtx); err != nil {
		return nil, fmt.Errorf("sandbox image unavailable: %w", err)
	}

	startCtx, startCancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer startCancel()
	if err := cs.Start(startCtx); err != nil {
		return nil, fmt.Errorf("container start failed: %w", err)
	}
	return cs, nil
}

// attachRequiredContainer starts and binds the mandatory Docker sandbox to a
// headless or REPL session. Callers own the returned sandbox and must stop it.
func attachRequiredContainer(sess *engine.Session, projectDir string) (*sandbox.ContainerSandbox, error) {
	if sess == nil {
		return nil, fmt.Errorf("docker container required: session is unavailable")
	}
	// Keep IsolationProfile in sync with Docker-required execution (single story).
	sess.ApplyIsolationProfile(engine.IsolationContainer)
	cs, err := startRequiredContainer(projectDir)
	if err != nil {
		return nil, fmt.Errorf("docker container required: %w", err)
	}
	sess.SetContainerExecutor(cs)
	return cs, nil
}

// bootContainerCmd starts the container in the background and sends status
// updates to the TUI (async boot with progress feedback).
// Retries up to 3 times with exponential backoff (0s, 2s, 4s) before
// stopping in a retryable error state.
func bootContainerCmd(projectDir string) tea.Cmd {
	const maxAttempts = 3
	backoff := []time.Duration{0, 2 * time.Second, 4 * time.Second}

	return func() tea.Msg {
		for attempt := 0; attempt < maxAttempts; attempt++ {
			if attempt > 0 {
				time.Sleep(backoff[attempt])
			}

			cs, err := startRequiredContainer(projectDir)
			if err != nil {
				if attempt < maxAttempts-1 {
					continue
				}
				return containerStatusMsg{status: "container required", err: err}
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

		return containerStatusMsg{
			status: "retry exhausted",
			err:    fmt.Errorf("docker container failed after %d attempts — press r to retry", maxAttempts),
		}
	}
}
