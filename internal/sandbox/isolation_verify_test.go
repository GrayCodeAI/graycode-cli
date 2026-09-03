package sandbox

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func dockerAvailableQuick(t *testing.T) bool {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "docker", "info")
	cmd.Stdout = nil
	cmd.Stderr = nil
	if err := cmd.Run(); err != nil {
		t.Skipf("docker not ready: %v", err)
	}
	return true
}

// TestVerify_ContainerDoesNotExposeHostGraycodeHome checks Docker isolation when available.
// The project dir is mounted; ~/.graycode on the host must not be readable inside the container.
func TestVerify_ContainerDoesNotExposeHostGraycodeHome(t *testing.T) {
	if !dockerAvailableQuick(t) {
		return
	}
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	graycodeEnv := filepath.Join(home, ".graycode", "env")
	if _, statErr := os.Stat(graycodeEnv); statErr != nil {
		// Create a marker file so we can detect accidental host mount exposure.
		_ = os.MkdirAll(filepath.Dir(graycodeEnv), 0o700)
		if writeErr := os.WriteFile(graycodeEnv, []byte("export VERIFY_GRAYCODE_HOME_SECRET=1\n"), 0o600); writeErr != nil {
			t.Fatal(writeErr)
		}
		t.Cleanup(func() { _ = os.Remove(graycodeEnv) })
	}

	projectDir := t.TempDir()
	cs := NewContainerSandbox(projectDir)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	if _, imageErr := cs.EnsureImage(ctx); imageErr != nil {
		t.Fatalf("sandbox image: %v", imageErr)
	}
	if startErr := cs.Start(ctx); startErr != nil {
		t.Fatalf("container start: %v", startErr)
	}
	t.Cleanup(func() { _ = cs.Stop() })

	out, err := cs.Exec(ctx, "cat "+graycodeEnv, 30*time.Second)
	if err == nil && strings.Contains(out, "VERIFY_GRAYCODE_HOME_SECRET") {
		t.Fatalf("container could read host ~/.graycode/env:\n%s", out)
	}
	// Expected: file missing or permission denied inside container.
}
