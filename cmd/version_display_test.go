package cmd

import (
	"os"
	"path/filepath"
	"testing"

	hawkconfig "github.com/GrayCodeAI/hawk/internal/config"
	"github.com/GrayCodeAI/hawk/internal/provider/gateway"
)

func TestDisplayVersion_FromVERSIONFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "VERSION"), []byte("0.1.0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)
	SetVersion("dev")
	if got := DisplayVersion(); got != "0.1.0" {
		t.Fatalf("DisplayVersion() = %q, want 0.1.0", got)
	}
}

func TestDisplayVersion_ReleaseBuild(t *testing.T) {
	SetVersion("1.4.2")
	if got := DisplayVersion(); got != "1.4.2" {
		t.Fatalf("DisplayVersion() = %q, want 1.4.2", got)
	}
}

func TestChatConnectionStatus_NoCredentials(t *testing.T) {
	hawkconfig.InvalidateConfigUICache()
	store := &gateway.MapStore{}
	gateway.SetDefaultStore(store)
	t.Cleanup(func() {
		gateway.SetDefaultStore(nil)
		hawkconfig.InvalidateConfigUICache()
	})

	m := chatModel{session: nil}
	got := m.chatConnectionStatus()
	if got != "" {
		t.Fatalf("connection status = %q, want empty when unconfigured", got)
	}
}

func TestChatBottomRightStatus_NoCredentials(t *testing.T) {
	hawkconfig.InvalidateConfigUICache()
	store := &gateway.MapStore{}
	gateway.SetDefaultStore(store)
	t.Cleanup(func() {
		gateway.SetDefaultStore(nil)
		hawkconfig.InvalidateConfigUICache()
	})

	m := chatModel{inputIndicator: &InputIndicator{}}
	got := m.chatBottomRightStatus()
	if got != "" {
		t.Fatalf("status = %q, want empty when no keys", got)
	}
}
