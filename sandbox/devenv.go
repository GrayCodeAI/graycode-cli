// Package sandbox provides sandbox mode for isolated command execution.
// devenv.go adds dynamic Dockerfile caching per project.
package sandbox

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
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

// DevEnvManager caches Docker images per-project based on Dockerfile content hashes.
type DevEnvManager struct {
	projectDir string
	imageCache map[string]CachedImage
	mu         sync.Mutex
	// buildFn is the function called to build a Docker image. Defaults to actual Docker build.
	// Can be overridden in tests.
	buildFn func(ctx context.Context, dockerfile, tag string) error
}

// NewDevEnvManager creates a new DevEnvManager for the given project directory.
func NewDevEnvManager(projectDir string) *DevEnvManager {
	return &DevEnvManager{
		projectDir: projectDir,
		imageCache: make(map[string]CachedImage),
		buildFn:    defaultBuildFn,
	}
}

// defaultBuildFn is a placeholder build function. In production this would invoke `docker build`.
func defaultBuildFn(ctx context.Context, dockerfile, tag string) error {
	// In a real implementation, this would run:
	//   docker build -t <tag> -f <dockerfile> <context>
	return nil
}

// GetOrBuild returns the cached image tag if the Dockerfile content hash matches,
// otherwise rebuilds and caches the new image.
func (d *DevEnvManager) GetOrBuild(ctx context.Context, dockerfile string) (string, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	hash, err := hashDockerfile(dockerfile)
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

	tag := fmt.Sprintf("hawk-devenv-%s:%s", key, hash[:12])

	if err := d.buildFn(ctx, dockerfile, tag); err != nil {
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

// hashDockerfile computes a SHA-256 hash of the Dockerfile contents.
func hashDockerfile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	h := sha256.Sum256(data)
	return fmt.Sprintf("%x", h), nil
}
