package sandbox

import (
	"context"
	_ "embed"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/GrayCodeAI/hawk/internal/storage"
)

const sandboxImageRepository = "graycodeai/hawk-sandbox"

//go:embed container_version
var rawSandboxImageTag string

//go:embed sandbox.Dockerfile
var bundledSandboxDockerfile string

var sandboxImageTag = strings.TrimSpace(rawSandboxImageTag)

// dockerImageCommand is replaceable in tests.
var dockerImageCommand = func(ctx context.Context, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, "docker", args...).CombinedOutput()
}

// ImageProvisionResult describes how EnsureImage satisfied the image contract.
type ImageProvisionResult string

const (
	ImageAlreadyLocal ImageProvisionResult = "local"
	ImagePulled       ImageProvisionResult = "pulled"
	ImageBuilt        ImageProvisionResult = "built"
)

func defaultHawkImage() string {
	return sandboxImageRepository + ":" + sandboxImageTag
}

func localHawkImage() string {
	return "hawk-sandbox:" + sandboxImageTag
}

// EnsureImage makes the selected sandbox image available without requiring a
// registry login. Hawk first uses a local image, then tries the public image,
// and finally builds the bundled sandbox Dockerfile locally through Docker.
func (c *ContainerSandbox) EnsureImage(ctx context.Context) (ImageProvisionResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	image := c.Image()
	inspectCtx, inspectCancel := context.WithTimeout(ctx, 10*time.Second)
	_, inspectErr := dockerImageCommand(inspectCtx, "image", "inspect", image)
	inspectCancel()
	if inspectErr == nil {
		return ImageAlreadyLocal, nil
	}

	// Project-specific images are rebuilt from their persisted Dockerfile.
	if image != defaultHawkImage() {
		dfPath := filepath.Join(storage.ProjectStateDir(c.projectDir), "Dockerfile")
		content, err := os.ReadFile(dfPath) // #nosec G304 -- projectStateDir is Hawk-managed state for this workspace
		if err != nil {
			return "", fmt.Errorf("sandbox image %s is unavailable and its Dockerfile cannot be read: %w", image, err)
		}
		buildCtx, buildCancel := context.WithTimeout(ctx, 10*time.Minute)
		defer buildCancel()
		if _, err := c.BuildFromDockerfile(buildCtx, string(content)); err != nil {
			return "", fmt.Errorf("building project sandbox image: %w", err)
		}
		return ImageBuilt, nil
	}

	// Reuse a previous no-registry fallback build before contacting Docker Hub.
	localImage := localHawkImage()
	localInspectCtx, localInspectCancel := context.WithTimeout(ctx, 10*time.Second)
	_, localInspectErr := dockerImageCommand(localInspectCtx, "image", "inspect", localImage)
	localInspectCancel()
	if localInspectErr == nil {
		c.SetImage(localImage)
		return ImageAlreadyLocal, nil
	}

	pullCtx, pullCancel := context.WithTimeout(ctx, 5*time.Minute)
	pullOut, pullErr := dockerImageCommand(pullCtx, "pull", image)
	pullCancel()
	if pullErr == nil {
		return ImagePulled, nil
	}

	buildCtx, buildCancel := context.WithTimeout(ctx, 10*time.Minute)
	defer buildCancel()
	buildOut, buildErr := buildBundledSandboxImage(buildCtx, localImage)
	if buildErr != nil {
		return "", fmt.Errorf(
			"pulling public sandbox image failed: %s; local Docker build failed: %s",
			commandOutput(pullOut, pullErr),
			commandOutput(buildOut, buildErr),
		)
	}
	c.SetImage(localImage)
	return ImageBuilt, nil
}

func buildBundledSandboxImage(ctx context.Context, image string) ([]byte, error) {
	buildDir, err := os.MkdirTemp("", "hawk-sandbox-image-*")
	if err != nil {
		return nil, err
	}
	defer func() { _ = os.RemoveAll(buildDir) }()

	dockerfilePath := filepath.Join(buildDir, "Dockerfile")
	if err := os.WriteFile(dockerfilePath, []byte(bundledSandboxDockerfile), 0o600); err != nil {
		return nil, err
	}
	return dockerImageCommand(ctx, "build", "-t", image, "-f", dockerfilePath, buildDir)
}

func commandOutput(output []byte, err error) string {
	if text := strings.TrimSpace(string(output)); text != "" {
		return text
	}
	if err != nil {
		return err.Error()
	}
	return "unknown Docker error"
}
