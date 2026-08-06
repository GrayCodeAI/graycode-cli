package sandbox

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/GrayCodeAI/hawk/internal/storage"
)

func TestDockerAvailable(t *testing.T) {
	// Just verify the function doesn't panic
	_ = DockerAvailable()
}

func TestDockerAvailable_UsesShortLivedCache(t *testing.T) {
	resetDockerAvailabilityCache()
	t.Cleanup(resetDockerAvailabilityCache)

	var calls atomic.Int32
	dockerAvailabilityProbe = func() bool {
		calls.Add(1)
		return true
	}

	if !DockerAvailable() {
		t.Fatal("expected cached probe to return true")
	}
	if !DockerAvailable() {
		t.Fatal("expected second cached probe to return true")
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("docker availability probe calls = %d, want 1", got)
	}
}

func TestContainerSandbox_New(t *testing.T) {
	cs := NewContainerSandbox("/tmp/test-project")
	if cs == nil {
		t.Fatal("expected non-nil sandbox")
	}
	if cs.projectDir != "/tmp/test-project" {
		t.Fatalf("expected projectDir=/tmp/test-project, got %s", cs.projectDir)
	}
	if cs.Running() {
		t.Fatal("new sandbox should not be running")
	}
}

func TestContainerSandbox_ContainerName(t *testing.T) {
	cs := NewContainerSandbox("/Users/test/my-project")
	name := cs.containerName()
	if name == "" {
		t.Fatal("expected non-empty container name")
	}
	if len(name) < 10 {
		t.Fatalf("container name too short: %s", name)
	}
}

func TestContainerSandbox_StopForceRemovesContainer(t *testing.T) {
	original := forceRemoveContainer
	t.Cleanup(func() { forceRemoveContainer = original })

	var gotID string
	forceRemoveContainer = func(_ context.Context, containerID string) error {
		gotID = containerID
		return nil
	}

	cs := NewContainerSandbox(t.TempDir())
	cs.running = true
	cs.containerID = "hawk-test-container"

	if err := cs.Stop(); err != nil {
		t.Fatal(err)
	}
	if cs.Running() {
		t.Fatal("container should be marked stopped")
	}
	if cs.ContainerID() != "" {
		t.Fatal("stopped container ID should be cleared")
	}
	if gotID != "hawk-test-container" {
		t.Fatalf("removed container = %q, want %q", gotID, "hawk-test-container")
	}
}

// TestContainerSandbox_DockerRunArgs_Hardened verifies the hardened run args
// are present regardless of userns availability.
func TestContainerSandbox_DockerRunArgs_Hardened(t *testing.T) {
	projectDir := t.TempDir()
	cs := NewContainerSandbox(projectDir)
	cs.SetImage("hawk:test")

	args := cs.dockerRunArgs("hawk-test", "/tmp/attach", "/tmp/cache")
	joined := strings.Join(args, " ")

	for _, want := range []string{
		"--network bridge",
		"--cap-drop ALL",
		"--security-opt no-new-privileges",
		"--pids-limit 256",
		"--read-only",
		"--tmpfs /tmp:rw,noexec,nosuid,nodev,size=64m",
		"-w " + projectDir,
		"--entrypoint sleep",
		"hawk:test infinity",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("docker run args missing %q:\n%s", want, joined)
		}
	}
}

func TestResolveImage_Default(t *testing.T) {
	img := resolveImage(t.TempDir())
	expected := "graycodeai/hawk-sandbox:" + sandboxImageTag
	if img != expected {
		t.Fatalf("expected default image %s, got %s", expected, img)
	}
}

func TestEnsureImageAlreadyLocal(t *testing.T) {
	original := dockerImageCommand
	t.Cleanup(func() { dockerImageCommand = original })

	var calls int
	dockerImageCommand = func(_ context.Context, args ...string) ([]byte, error) {
		calls++
		if strings.Join(args, " ") != "image inspect "+defaultHawkImage() {
			t.Fatalf("unexpected Docker command: %v", args)
		}
		return nil, nil
	}

	cs := NewContainerSandbox(t.TempDir())
	result, err := cs.EnsureImage(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if result != ImageAlreadyLocal {
		t.Fatalf("result = %q, want %q", result, ImageAlreadyLocal)
	}
	if calls != 1 {
		t.Fatalf("Docker calls = %d, want 1", calls)
	}
}

func TestEnsureImagePullsPublicImage(t *testing.T) {
	original := dockerImageCommand
	t.Cleanup(func() { dockerImageCommand = original })

	var calls []string
	dockerImageCommand = func(_ context.Context, args ...string) ([]byte, error) {
		command := strings.Join(args, " ")
		calls = append(calls, command)
		if strings.HasPrefix(command, "image inspect ") {
			return nil, errors.New("missing")
		}
		if command == "pull "+defaultHawkImage() {
			return []byte("pulled"), nil
		}
		return nil, errors.New("unexpected command")
	}

	cs := NewContainerSandbox(t.TempDir())
	result, err := cs.EnsureImage(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if result != ImagePulled {
		t.Fatalf("result = %q, want %q", result, ImagePulled)
	}
	if len(calls) != 3 {
		t.Fatalf("Docker calls = %v, want public inspect + local inspect + pull", calls)
	}
}

func TestEnsureImageBuildsBundledFallback(t *testing.T) {
	original := dockerImageCommand
	t.Cleanup(func() { dockerImageCommand = original })

	var calls []string
	dockerImageCommand = func(_ context.Context, args ...string) ([]byte, error) {
		command := strings.Join(args, " ")
		calls = append(calls, command)
		switch {
		case strings.HasPrefix(command, "image inspect "):
			return nil, errors.New("missing")
		case strings.HasPrefix(command, "pull "):
			return []byte("registry unavailable"), errors.New("pull failed")
		case strings.HasPrefix(command, "build -t "+localHawkImage()+" "):
			return []byte("built"), nil
		default:
			return nil, errors.New("unexpected command")
		}
	}

	cs := NewContainerSandbox(t.TempDir())
	result, err := cs.EnsureImage(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if result != ImageBuilt {
		t.Fatalf("result = %q, want %q", result, ImageBuilt)
	}
	if cs.Image() != localHawkImage() {
		t.Fatalf("image = %q, want %q", cs.Image(), localHawkImage())
	}
	if len(calls) != 4 {
		t.Fatalf("Docker calls = %v, want public inspect + local inspect + pull + build", calls)
	}
}

func TestResolveImage_WithDockerfile(t *testing.T) {
	dir := t.TempDir()
	dockerDir := storage.ProjectStateDir(dir)
	if err := mkdirAll(dockerDir); err != nil {
		t.Fatal(err)
	}
	if err := writeFile(filepath.Join(dockerDir, "Dockerfile"), "FROM node:20\nRUN npm install"); err != nil {
		t.Fatal(err)
	}
	img := resolveImage(dir)
	if img == "ubuntu:24.04" {
		t.Fatal("expected custom image tag, got default")
	}
	if !contains(img, "hawk-sandbox:") {
		t.Fatalf("expected hawk-sandbox tag, got %s", img)
	}
}

func mkdirAll(path string) error {
	return os.MkdirAll(path, 0o755)
}

func writeFile(path, content string) error {
	return os.WriteFile(path, []byte(content), 0o644)
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(s) > 0 && containsStr(s, sub))
}

func containsStr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

// TestContainerSandbox_DockerRunArgs_UserFallback verifies that without
// userns remapping the container runs as the host uid:gid instead of root
// (M12), and that userns remapping suppresses the --user fallback.
func TestContainerSandbox_DockerRunArgs_UserFallback(t *testing.T) {
	original := usernsProbe
	t.Cleanup(func() { usernsProbe = original; resetUsernsCache() })

	projectDir := t.TempDir()
	cs := NewContainerSandbox(projectDir)
	cs.SetImage("hawk:test")

	// userns unavailable -> --user fallback with host uid:gid.
	resetUsernsCache()
	usernsProbe = func() (bool, error) { return false, nil }
	args := strings.Join(cs.dockerRunArgs("hawk-test", "/tmp/attach", "/tmp/cache"), " ")
	wantUser := fmt.Sprintf("--user %d:%d", os.Getuid(), os.Getgid())
	if !strings.Contains(args, wantUser) {
		t.Fatalf("expected %q in run args without userns, got:\n%s", wantUser, args)
	}
	if strings.Contains(args, "--userns-remap") {
		t.Fatalf("userns-remap must not be added when unavailable:\n%s", args)
	}

	// userns available -> --userns-remap, no --user fallback.
	resetUsernsCache()
	usernsProbe = func() (bool, error) { return true, nil }
	args = strings.Join(cs.dockerRunArgs("hawk-test", "/tmp/attach", "/tmp/cache"), " ")
	if !strings.Contains(args, "--userns-remap default") {
		t.Fatalf("expected --userns-remap default in run args, got:\n%s", args)
	}
	if strings.Contains(args, "--user ") {
		t.Fatalf("--user fallback must not be added when userns is available:\n%s", args)
	}
}

// TestUsernsRemapAvailable_UsesProbeAndCache verifies the userns-remap probe
// (H16) is consulted once and cached for the process lifetime.
func TestUsernsRemapAvailable_UsesProbeAndCache(t *testing.T) {
	original := usernsProbe
	t.Cleanup(func() { usernsProbe = original; resetUsernsCache() })
	resetUsernsCache()

	var calls int
	usernsProbe = func() (bool, error) {
		calls++
		return true, nil
	}

	if !usernsRemapAvailable() {
		t.Fatal("expected probe to report userns available")
	}
	// Second call must be served from cache.
	if !usernsRemapAvailable() {
		t.Fatal("expected cached userns availability")
	}
	if calls != 1 {
		t.Fatalf("userns probe calls = %d, want 1 (cached)", calls)
	}
}

// TestUsernsRemapAvailable_FalseOnProbeError verifies an unavailable Docker
// daemon does not enable userns remapping.
func TestUsernsRemapAvailable_FalseOnProbeError(t *testing.T) {
	original := usernsProbe
	t.Cleanup(func() { usernsProbe = original; resetUsernsCache() })
	resetUsernsCache()

	usernsProbe = func() (bool, error) {
		return false, errors.New("docker unreachable")
	}
	if usernsRemapAvailable() {
		t.Error("expected userns remapping unavailable when docker cannot be probed")
	}
}

func TestDefaultHawkImageDigestOverride(t *testing.T) {
	prev := sandboxImageDigestOverride
	sandboxImageDigestOverride = "abc123digest"
	defer func() { sandboxImageDigestOverride = prev }()

	got := defaultHawkImage()
	wantRepo := sandboxImageRepository + "@sha256:"
	if !strings.HasPrefix(got, wantRepo) {
		t.Fatalf("defaultHawkImage=%q, want prefix %q", got, wantRepo)
	}
	if !strings.HasSuffix(got, "abc123digest") {
		t.Fatalf("defaultHawkImage=%q, want digest suffix", got)
	}
}
