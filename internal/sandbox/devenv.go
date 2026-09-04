// Package sandbox provides sandbox mode for isolated command execution.
// devenv.go adds dynamic Dockerfile caching per project.
package sandbox

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"
)

// CachedImage holds metadata about a cached Docker image for a project.
type CachedImage struct {
	Tag         string
	ContentHash string
	BuiltAt     time.Time
	Stale       bool
}

// SwapRequest is sent when a container hot-swap is needed after a rebuild.
type SwapRequest struct {
	ImageTag   string
	Dockerfile string
	Workspace  string
}

// DevEnvManager caches Docker images per-project based on Dockerfile content hashes.
type DevEnvManager struct {
	projectDir string
	imageCache map[string]CachedImage
	mu         sync.Mutex
	// buildFn is the function called to build a Docker image. Defaults to actual Docker build.
	// Can be overridden in tests.
	buildFn func(ctx context.Context, dockerfile, tag string) error
	// OnSwapNeeded is called after a successful rebuild to request a container
	// hot-swap. The session should stop the old container and start a new one
	// with the given image tag. May be nil.
	OnSwapNeeded func(req SwapRequest)
	// runtime carries declarative runtime_extra_deps. When non-empty, the
	// extra-deps RUN layers are appended to the Dockerfile before building.
	runtime RuntimeConfig
}

// NewDevEnvManager creates a new DevEnvManager for the given project directory.
func NewDevEnvManager(projectDir string) *DevEnvManager {
	return &DevEnvManager{
		projectDir: projectDir,
		imageCache: make(map[string]CachedImage),
		buildFn:    defaultBuildFn,
		runtime:    LoadRuntimeConfig(projectDir),
	}
}

// SetRuntimeConfig overrides the declarative runtime config used when building
// images. Additive: an empty config restores prior behavior.
func (d *DevEnvManager) SetRuntimeConfig(cfg RuntimeConfig) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.runtime = cfg
}

// defaultBuildFn builds a Docker image from the given Dockerfile path,
// tagging it with the given tag. The build context is the directory
// containing the Dockerfile.
func defaultBuildFn(ctx context.Context, dockerfile, tag string) error {
	contextDir := filepath.Dir(dockerfile)
	cmd := exec.CommandContext(ctx, "docker", "build", "-t", tag, "-f", dockerfile, contextDir) // #nosec G204 -- "docker" binary fixed; tag/dockerfile/contextDir derived from internal build state
	cmd.Stdout = os.Stderr                                                                      // show build output on stderr
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("docker build failed: %w", err)
	}
	return nil
}

// GetOrBuild returns the cached image tag if the Dockerfile content hash matches,
// otherwise rebuilds and caches the new image.
func (d *DevEnvManager) GetOrBuild(ctx context.Context, dockerfile string) (string, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	// When extra deps are configured, materialize an augmented Dockerfile so the
	// extra-deps RUN layers participate in the build and the content hash.
	buildPath := dockerfile
	if !d.runtime.IsEmpty() {
		augmented, err := d.augmentDockerfile(dockerfile)
		if err != nil {
			return "", fmt.Errorf("augmenting dockerfile: %w", err)
		}
		buildPath = augmented
	}

	hash, err := hashDockerfile(buildPath)
	if err != nil {
		return "", fmt.Errorf("hashing dockerfile: %w", err)
	}

	key := filepath.Base(filepath.Dir(dockerfile))
	if key == "." || key == "" {
		key = "default"
	}

	if cached, ok := d.imageCache[key]; ok && !cached.Stale && cached.ContentHash == hash {
		return cached.Tag, nil
	}

	tag := fmt.Sprintf("graycode-devenv-%s:%s", key, hash[:12])

	if err := d.buildFn(ctx, buildPath, tag); err != nil {
		return "", fmt.Errorf("building image: %w", err)
	}

	d.imageCache[key] = CachedImage{
		Tag:         tag,
		ContentHash: hash,
		BuiltAt:     time.Now(),
		Stale:       false,
	}

	return tag, nil
}

// IsStale checks if the Dockerfile for the given project directory has changed since last build.
func (d *DevEnvManager) IsStale(projectDir string) bool {
	d.mu.Lock()
	defer d.mu.Unlock()

	key := filepath.Base(projectDir)
	cached, ok := d.imageCache[key]
	if !ok {
		return true
	}
	if cached.Stale {
		return true
	}

	// Check if Dockerfile has changed
	dockerfilePath := filepath.Join(projectDir, "Dockerfile")
	hash, err := hashDockerfile(dockerfilePath)
	if err != nil {
		return true
	}

	return hash != cached.ContentHash
}

// Invalidate marks the cache for the given project directory as stale.
func (d *DevEnvManager) Invalidate(projectDir string) {
	d.mu.Lock()
	defer d.mu.Unlock()

	key := filepath.Base(projectDir)
	if cached, ok := d.imageCache[key]; ok {
		cached.Stale = true
		d.imageCache[key] = cached
	}
}

// RebuildAndForceSwap forces a rebuild even if cached, then triggers
// the OnSwapNeeded callback. This is the hot-swap path.
func (d *DevEnvManager) RebuildAndForceSwap(ctx context.Context, dockerfilePath string) (string, error) {
	d.mu.Lock()
	key := filepath.Base(filepath.Dir(dockerfilePath))
	if key == "." || key == "" {
		key = "default"
	}
	// Invalidate to force rebuild.
	if cached, ok := d.imageCache[key]; ok {
		cached.Stale = true
		d.imageCache[key] = cached
	}
	d.mu.Unlock()

	tag, err := d.GetOrBuild(ctx, dockerfilePath)
	if err != nil {
		return "", err
	}

	if d.OnSwapNeeded != nil {
		d.OnSwapNeeded(SwapRequest{
			ImageTag:   tag,
			Dockerfile: dockerfilePath,
			Workspace:  d.projectDir,
		})
	}

	return tag, nil
}

// augmentDockerfile reads the Dockerfile at path, appends the runtime
// extra-deps RUN layers, and writes the result to a sibling
// "Dockerfile.graycode-runtime" file. Returns the path to the augmented file. The
// caller holds d.mu.
func (d *DevEnvManager) augmentDockerfile(path string) (string, error) {
	data, err := os.ReadFile(path) // #nosec G304 -- path is the project's own Dockerfile path passed by the caller
	if err != nil {
		return "", err
	}
	augmented := d.runtime.AppendExtraDeps(string(data))
	outPath := filepath.Join(filepath.Dir(path), "Dockerfile.graycode-runtime")
	if err := os.WriteFile(outPath, []byte(augmented), 0o600); err != nil { // #nosec G703 -- sibling output is derived from the caller's project Dockerfile
		return "", err
	}
	return outPath, nil
}

// hashDockerfile computes a SHA-256 hash of the Dockerfile contents.
func hashDockerfile(path string) (string, error) {
	data, err := os.ReadFile(path) // #nosec G304 -- path is the project's own Dockerfile path passed by the caller
	if err != nil {
		return "", err
	}
	h := sha256.Sum256(data)
	return fmt.Sprintf("%x", h), nil
}
