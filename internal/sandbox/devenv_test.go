package sandbox

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestDevEnvManager_GetOrBuild_CachesOnSameHash(t *testing.T) {
	dir := t.TempDir()
	dockerfile := filepath.Join(dir, "Dockerfile")
	if err := os.WriteFile(dockerfile, []byte("FROM alpine:latest\nRUN echo hello"), 0o644); err != nil {
		t.Fatal(err)
	}

	buildCount := 0
	mgr := NewDevEnvManager(dir)
	mgr.buildFn = func(ctx context.Context, df, tag string) error {
		buildCount++
		return nil
	}

	ctx := context.Background()

	tag1, err := mgr.GetOrBuild(ctx, dockerfile)
	if err != nil {
		t.Fatalf("first GetOrBuild: %v", err)
	}
	if tag1 == "" {
		t.Fatal("expected non-empty tag")
	}
	if buildCount != 1 {
		t.Fatalf("expected 1 build, got %d", buildCount)
	}

	// Second call should use cache
	tag2, err := mgr.GetOrBuild(ctx, dockerfile)
	if err != nil {
		t.Fatalf("second GetOrBuild: %v", err)
	}
	if tag2 != tag1 {
		t.Fatalf("expected same tag %q, got %q", tag1, tag2)
	}
	if buildCount != 1 {
		t.Fatalf("expected still 1 build (cached), got %d", buildCount)
	}
}

func TestDevEnvManager_GetOrBuild_RebuildsOnChange(t *testing.T) {
	dir := t.TempDir()
	dockerfile := filepath.Join(dir, "Dockerfile")
	if err := os.WriteFile(dockerfile, []byte("FROM alpine:latest\nRUN echo v1"), 0o644); err != nil {
		t.Fatal(err)
	}

	buildCount := 0
	mgr := NewDevEnvManager(dir)
	mgr.buildFn = func(ctx context.Context, df, tag string) error {
		buildCount++
		return nil
	}

	ctx := context.Background()

	tag1, err := mgr.GetOrBuild(ctx, dockerfile)
	if err != nil {
		t.Fatalf("first GetOrBuild: %v", err)
	}

	// Modify Dockerfile
	if writeErr := os.WriteFile(dockerfile, []byte("FROM alpine:latest\nRUN echo v2"), 0o644); writeErr != nil {
		t.Fatal(writeErr)
	}

	tag2, err := mgr.GetOrBuild(ctx, dockerfile)
	if err != nil {
		t.Fatalf("second GetOrBuild: %v", err)
	}
	if tag2 == tag1 {
		t.Fatal("expected different tag after Dockerfile change")
	}
	if buildCount != 2 {
		t.Fatalf("expected 2 builds, got %d", buildCount)
	}
}

func TestDevEnvManager_IsStale_NoCache(t *testing.T) {
	dir := t.TempDir()
	mgr := NewDevEnvManager(dir)

	// No cache entry should be stale
	if !mgr.IsStale(dir) {
		t.Fatal("expected IsStale=true for uncached project")
	}
}

func TestDevEnvManager_Invalidate(t *testing.T) {
	dir := t.TempDir()
	subDir := filepath.Join(dir, "myproject")
	os.MkdirAll(subDir, 0o755)
	dockerfile := filepath.Join(subDir, "Dockerfile")
	if err := os.WriteFile(dockerfile, []byte("FROM alpine:latest"), 0o644); err != nil {
		t.Fatal(err)
	}

	mgr := NewDevEnvManager(dir)
	mgr.buildFn = func(ctx context.Context, df, tag string) error { return nil }

	ctx := context.Background()
	mgr.GetOrBuild(ctx, dockerfile)

	// Manually set the cache key so Invalidate can find it
	mgr.mu.Lock()
	cached := mgr.imageCache["myproject"]
	mgr.imageCache["myproject"] = cached
	mgr.mu.Unlock()

	mgr.Invalidate(subDir)

	mgr.mu.Lock()
	entry := mgr.imageCache["myproject"]
	mgr.mu.Unlock()

	if !entry.Stale {
		t.Fatal("expected cache entry to be stale after Invalidate")
	}
}

func TestDevEnvManager_GetOrBuild_ErrorOnMissingDockerfile(t *testing.T) {
	dir := t.TempDir()
	mgr := NewDevEnvManager(dir)

	ctx := context.Background()
	_, err := mgr.GetOrBuild(ctx, filepath.Join(dir, "nonexistent"))
	if err == nil {
		t.Fatal("expected error for missing Dockerfile")
	}
}
