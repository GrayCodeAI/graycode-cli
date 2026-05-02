//go:build !linux

package sandbox

import "fmt"

// LandlockSandbox is a stub on non-Linux platforms.
type LandlockSandbox struct {
	projectDir string
}

// NewLandlockSandbox returns a stub sandbox on non-Linux systems.
func NewLandlockSandbox(projectDir string) *LandlockSandbox {
	return &LandlockSandbox{projectDir: projectDir}
}

// Apply always returns an error on non-Linux platforms.
func (l *LandlockSandbox) Apply() error {
	return fmt.Errorf("landlock: not available on this platform")
}

// AddReadOnlyPath is a no-op on non-Linux platforms.
func (l *LandlockSandbox) AddReadOnlyPath(path string) {}

// AddReadWritePath is a no-op on non-Linux platforms.
func (l *LandlockSandbox) AddReadWritePath(path string) {}

// LandlockAvailable always returns false on non-Linux platforms.
func LandlockAvailable() bool { return false }
