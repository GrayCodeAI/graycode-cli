package sandbox

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/GrayCodeAI/hawk/internal/storage"
)

func TestDockerAvailable(t *testing.T) {
	// Just verify the function doesn't panic
	_ = DockerAvailable()
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

func TestResolveImage_Default(t *testing.T) {
	img := resolveImage(t.TempDir())
	expected := defaultHawkImage()
	if img != expected {
		t.Fatalf("expected default image %s, got %s", expected, img)
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
