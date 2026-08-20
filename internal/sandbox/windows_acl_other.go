//go:build !windows

package sandbox

import (
	"errors"
)

// WindowsACLSandbox is a stub on non-Windows platforms.
type WindowsACLSandbox struct {
	projectDir     string
	readOnlyPaths  []string
	readWritePaths []string
}

// NewWindowsACLSandbox returns a stub sandbox on non-Windows systems.
func NewWindowsACLSandbox(projectDir string) *WindowsACLSandbox {
	return &WindowsACLSandbox{projectDir: projectDir}
}

// Apply always returns an error on non-Windows platforms.
func (s *WindowsACLSandbox) Apply() error {
	return errors.New("windows_acl: not available on non-Windows platforms")
}

// AddReadOnlyPath is a no-op on non-Windows platforms.
func (s *WindowsACLSandbox) AddReadOnlyPath(path string) {
	if path != "" {
		s.readOnlyPaths = append(s.readOnlyPaths, path)
	}
}

// AddReadWritePath is a no-op on non-Windows platforms.
func (s *WindowsACLSandbox) AddReadWritePath(path string) {
	if path != "" {
		s.readWritePaths = append(s.readWritePaths, path)
	}
}

// WindowsACLAvailable always returns false on non-Windows platforms.
func WindowsACLAvailable() bool { return false }
