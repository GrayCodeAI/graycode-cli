//go:build windows

package sandbox

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"golang.org/x/sys/windows"
)

// WindowsACLSandbox provides unprivileged filesystem isolation on Windows via Access Control Lists (ACLs).
// Ports DSH's sandbox/sandbox-windows-acl.
type WindowsACLSandbox struct {
	mu             sync.Mutex
	projectDir     string
	readOnlyPaths  []string
	readWritePaths []string
	applied        bool
}

// NewWindowsACLSandbox creates a new Windows ACL sandbox for the specified project directory.
func NewWindowsACLSandbox(projectDir string) *WindowsACLSandbox {
	cleanDir := filepath.Clean(projectDir)
	return &WindowsACLSandbox{
		projectDir:     cleanDir,
		readWritePaths: []string{cleanDir},
	}
}

// AddReadOnlyPath adds a path that can be read but not modified.
func (s *WindowsACLSandbox) AddReadOnlyPath(path string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if path != "" {
		s.readOnlyPaths = append(s.readOnlyPaths, filepath.Clean(path))
	}
}

// AddReadWritePath adds a path that can be read and written.
func (s *WindowsACLSandbox) AddReadWritePath(path string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if path != "" {
		s.readWritePaths = append(s.readWritePaths, filepath.Clean(path))
	}
}

// Apply applies the Windows ACL rules to enforce read-only and read-write boundaries.
func (s *WindowsACLSandbox) Apply() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.projectDir == "" {
		return errors.New("windows_acl: projectDir cannot be empty")
	}

	// Verify project directory exists
	if _, err := os.Stat(s.projectDir); err != nil {
		return fmt.Errorf("windows_acl: project directory not accessible: %w", err)
	}

	// Apply read-only protections
	for _, p := range s.readOnlyPaths {
		if err := applyReadOnlyACL(p); err != nil {
			return fmt.Errorf("windows_acl: failed to set read-only ACL on %s: %w", p, err)
		}
	}

	// Apply read-write permissions
	for _, p := range s.readWritePaths {
		if err := applyReadWriteACL(p); err != nil {
			return fmt.Errorf("windows_acl: failed to set read-write ACL on %s: %w", p, err)
		}
	}

	s.applied = true
	return nil
}

// WindowsACLAvailable returns true if the platform supports native Windows ACL sandbox confinement.
func WindowsACLAvailable() bool {
	return true
}

func applyReadOnlyACL(path string) error {
	// SDDL: D:P(A;OICI;GRGX;;;WD) -> Protect DACL, Allow GenericRead/GenericExecute to Everyone (WD)
	sd, err := windows.SecurityDescriptorFromString("D:P(A;OICI;GRGX;;;WD)")
	if err != nil {
		return nil
	}
	dacl, _, err := sd.DACL()
	if err != nil || dacl == nil {
		return nil
	}

	return windows.SetNamedSecurityInfo(
		path,
		windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION,
		nil,
		nil,
		dacl,
		nil,
	)
}

func applyReadWriteACL(path string) error {
	// SDDL: D:P(A;OICI;GA;;;WD) -> Protect DACL, Allow GenericAll to Everyone (WD)
	sd, err := windows.SecurityDescriptorFromString("D:P(A;OICI;GA;;;WD)")
	if err != nil {
		return nil
	}
	dacl, _, err := sd.DACL()
	if err != nil || dacl == nil {
		return nil
	}

	return windows.SetNamedSecurityInfo(
		path,
		windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION,
		nil,
		nil,
		dacl,
		nil,
	)
}
