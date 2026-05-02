package sandbox

import (
	"strings"
	"runtime"
	"testing"
)

// ---------------------------------------------------------------------------
// LandlockAvailable
// ---------------------------------------------------------------------------

func TestLandlockAvailable(t *testing.T) {
	got := LandlockAvailable()
	if runtime.GOOS != "linux" && got {
		t.Fatal("LandlockAvailable() returned true on non-Linux platform")
	}
	// On Linux the result depends on kernel version; just ensure no panic.
	t.Logf("LandlockAvailable() = %v (GOOS=%s)", got, runtime.GOOS)
}

// ---------------------------------------------------------------------------
// NewLandlockSandbox construction
// ---------------------------------------------------------------------------

func TestNewLandlockSandbox(t *testing.T) {
	s := NewLandlockSandbox("/home/user/project")
	if s == nil {
		t.Fatal("NewLandlockSandbox returned nil")
	}
	if s.projectDir != "/home/user/project" {
		t.Errorf("projectDir = %q, want %q", s.projectDir, "/home/user/project")
	}
}

func TestNewLandlockSandboxEmptyDir(t *testing.T) {
	s := NewLandlockSandbox("")
	if s == nil {
		t.Fatal("NewLandlockSandbox returned nil for empty dir")
	}
	if s.projectDir != "" {
		t.Errorf("projectDir = %q, want empty", s.projectDir)
	}
}

// ---------------------------------------------------------------------------
// AddReadOnlyPath / AddReadWritePath
// ---------------------------------------------------------------------------

func TestAddReadOnlyPath(t *testing.T) {
	s := NewLandlockSandbox("/project")
	// AddReadOnlyPath is a no-op on non-Linux; just ensure no panic.
	s.AddReadOnlyPath("/extra/ro")
}

func TestAddReadWritePath(t *testing.T) {
	s := NewLandlockSandbox("/project")
	// AddReadWritePath is a no-op on non-Linux; just ensure no panic.
	s.AddReadWritePath("/extra/rw")
}

// ---------------------------------------------------------------------------
// Apply on non-Linux
// ---------------------------------------------------------------------------

func TestApplyNonLinux(t *testing.T) {
	if runtime.GOOS == "linux" {
		t.Skip("this test only runs on non-Linux")
	}
	s := NewLandlockSandbox("/project")
	err := s.Apply()
	if err == nil {
		t.Fatal("Apply() should return error on non-Linux")
	}
	if !strings.Contains(err.Error(), "not available") {
		t.Errorf("unexpected error: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Seccomp stubs
// ---------------------------------------------------------------------------

func TestDefaultSeccompProfile(t *testing.T) {
	profile := DefaultSeccompProfile()
	if runtime.GOOS == "linux" {
		if len(profile) == 0 {
			t.Fatal("DefaultSeccompProfile returned empty on Linux")
		}
		// Each BPF instruction is 8 bytes.
		if len(profile)%8 != 0 {
			t.Fatalf("profile length %d not a multiple of 8", len(profile))
		}
		t.Logf("seccomp profile: %d instructions", len(profile)/8)
	} else {
		if profile != nil {
			t.Fatalf("DefaultSeccompProfile should return nil on %s", runtime.GOOS)
		}
	}
}

func TestApplySeccompNonLinux(t *testing.T) {
	if runtime.GOOS == "linux" {
		t.Skip("this test only runs on non-Linux")
	}
	err := ApplySeccomp()
	if err == nil {
		t.Fatal("ApplySeccomp should return error on non-Linux")
	}
	if !strings.Contains(err.Error(), "not available") {
		t.Errorf("unexpected error: %v", err)
	}
}
