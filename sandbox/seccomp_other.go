//go:build !linux

package sandbox

import "fmt"

// DefaultSeccompProfile returns nil on non-Linux platforms.
func DefaultSeccompProfile() []byte { return nil }

// ApplySeccomp returns an error on non-Linux platforms.
func ApplySeccomp() error {
	return fmt.Errorf("seccomp: not available on this platform")
}
