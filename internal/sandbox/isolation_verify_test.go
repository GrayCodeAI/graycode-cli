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

func dockerImageAvailable(t *testing.T, image string) bool {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "docker", "image", "inspect", image)
	cmd.Stdout = nil
	cmd.Stderr = nil
	if err := cmd.Run(); err != nil {
		t.Skipf("docker image %q not available locally: %v", image, err)
	}
	return true
}

// TestVerify_ContainerDoesNotExposeHostHawkHome checks Docker isolation when available.
// The project dir is mounted; ~/.hawk on the host must not be readable inside the container.
func TestVerify_ContainerDoesNotExposeHostHawkHome(t *testing.T) {
	if !dockerAvailableQuick(t) {
		return
	}
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	hawkEnv := filepath.Join(home, ".hawk", "env")
	if _, statErr := os.Stat(hawkEnv); statErr != nil {
		// Create a marker file so we can detect accidental host mount exposure.
		_ = os.MkdirAll(filepath.Dir(hawkEnv), 0o700)
		if writeErr := os.WriteFile(hawkEnv, []byte("export VERIFY_HAWK_HOME_SECRET=1\n"), 0o600); writeErr != nil {
			t.Fatal(writeErr)
		}
		t.Cleanup(func() { _ = os.Remove(hawkEnv) })
	}

	projectDir := t.TempDir()
	cs := NewContainerSandbox(projectDir)
	if !dockerImageAvailable(t, cs.image) {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	if startErr := cs.Start(ctx); startErr != nil {
		t.Fatalf("container start: %v", startErr)
	}
	t.Cleanup(func() { _ = cs.Stop() })

	out, err := cs.Exec(ctx, "cat "+hawkEnv, 30*time.Second)
	if err == nil && strings.Contains(out, "VERIFY_HAWK_HOME_SECRET") {
		t.Fatalf("container could read host ~/.hawk/env:\n%s", out)
	}
	// Expected: file missing or permission denied inside container.
}
