package cmd

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
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

// buildHawkImage builds the hawk container image from the bundled Dockerfile.
// It writes the Dockerfile to a temp dir and runs docker build.
func buildHawkImage(ctx context.Context, tag string) bool {
	dockerfile := `FROM ubuntu:24.04
ENV DEBIAN_FRONTEND=noninteractive
RUN apt-get update && apt-get install -y --no-install-recommends \
    git curl wget jq tree ripgrep fd-find make gcc g++ \
    python3 python3-pip python3-venv \
    nodejs npm \
    ca-certificates openssh-client unzip xz-utils \
    && rm -rf /var/lib/apt/lists/* \
    && ln -sf /usr/bin/fdfind /usr/bin/fd
# Install Go
RUN curl -fsSL https://go.dev/dl/go1.26.1.linux-$(dpkg --print-architecture).tar.gz | tar -C /usr/local -xz
ENV PATH="/usr/local/go/bin:${PATH}"
ENV GOPATH="/root/go"
ENV PATH="${GOPATH}/bin:${PATH}"
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
	defer func() { _ = os.RemoveAll(tmpDir) }()

	dfPath := filepath.Join(tmpDir, "Dockerfile")
	if err := os.WriteFile(dfPath, []byte(dockerfile), 0o644); err != nil {
		return false
	}

	cmd := exec.CommandContext(ctx, "docker", "build", "--platform", platform, "-t", tag, "-f", dfPath, tmpDir)
	return cmd.Run() == nil
}

// shouldUseContainer determines if hawk should run in container mode.
// Default: container-first when Docker is available. Opt out with --no-container
// or HAWK_NO_CONTAINER=1 (useful on low-memory hosts where docker pull/build
// can trigger jetsam kills).
func shouldUseContainer() bool {
	if noContainer {
		return false
	}
	if v := strings.TrimSpace(os.Getenv("HAWK_NO_CONTAINER")); v == "1" || strings.EqualFold(v, "true") {
		return false
	}
	return true
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
