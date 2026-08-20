package sandbox

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestWindowsACL_Lifecycle(t *testing.T) {
	tmpDir := t.TempDir()
	sb := NewWindowsACLSandbox(tmpDir)
	if sb == nil {
		t.Fatal("expected non-nil WindowsACLSandbox")
	}

	readOnlyDir := filepath.Join(tmpDir, "readonly")
	_ = os.MkdirAll(readOnlyDir, 0o750)
	sb.AddReadOnlyPath(readOnlyDir)

	readWriteDir := filepath.Join(tmpDir, "readwrite")
	_ = os.MkdirAll(readWriteDir, 0o750)
	sb.AddReadWritePath(readWriteDir)

	if runtime.GOOS == "windows" {
		if !WindowsACLAvailable() {
			t.Fatal("expected WindowsACLAvailable() true on windows")
		}
		if err := sb.Apply(); err != nil {
			t.Fatalf("Apply() failed on windows: %v", err)
		}
	} else {
		if WindowsACLAvailable() {
			t.Fatal("expected WindowsACLAvailable() false on non-windows")
		}
		if err := sb.Apply(); err == nil {
			t.Fatal("expected Apply() error on non-windows, got nil")
		}
	}
}

func TestWindowsACL_Selector(t *testing.T) {
	if runtime.GOOS != "windows" {
		sel := selectWindows(IsolationDefault)
		if sel.Backend != "none" && sel.Backend != "docker" {
			t.Errorf("unexpected backend on non-windows: %s", sel.Backend)
		}
	} else {
		sel := selectWindows(IsolationDefault)
		if sel.Backend != "windows_acl" {
			t.Errorf("expected windows_acl backend on windows, got %s", sel.Backend)
		}
	}
}
