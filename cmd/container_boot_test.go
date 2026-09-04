package cmd

import (
	"strings"
	"testing"

	"github.com/GrayCodeAI/graycode-cli/internal/engine"
)

func TestShouldUseContainerAlwaysTrue(t *testing.T) {
	t.Setenv("GRAYCODE_NO_CONTAINER", "1")
	if !shouldUseContainer() {
		t.Fatal("Graycode must require Docker even when the legacy opt-out variable is set")
	}
}

func TestAttachRequiredContainerFailsClosed(t *testing.T) {
	original := dockerAvailable
	dockerAvailable = func() bool { return false }
	t.Cleanup(func() { dockerAvailable = original })

	sess := engine.NewSession("", "test-model", "system", nil)
	container, err := attachRequiredContainer(sess, t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "docker container required") {
		t.Fatalf("attachRequiredContainer() error = %v, want Docker-required error", err)
	}
	if container != nil {
		t.Fatal("failed Docker startup must not return a container")
	}
	if !sess.ContainerRequired() {
		t.Fatal("failed Docker startup must leave the session fail-closed")
	}
}
