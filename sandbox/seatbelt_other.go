//go:build !darwin

package sandbox

import (
	"context"
	"fmt"
	"os/exec"
)

// SeatbeltPolicy describes the permissions for a macOS seatbelt sandbox profile.
// This is a stub on non-darwin platforms.
type SeatbeltPolicy struct {
	AllowNetwork  bool
	AllowWrite    bool
	ReadablePaths []string
	WritablePaths []string
	AllowProcess  bool
	AllowSysctl   bool
}

// GenerateSeatbeltProfile is a stub on non-darwin platforms.
func GenerateSeatbeltProfile(policy *SeatbeltPolicy) string {
	return ""
}

// DefaultHawkPolicy is a stub on non-darwin platforms.
func DefaultHawkPolicy(workDir string) *SeatbeltPolicy {
	return &SeatbeltPolicy{}
}

// RunSeatbelted is not available on non-darwin platforms.
func RunSeatbelted(ctx context.Context, command string, policy *SeatbeltPolicy) (*exec.Cmd, error) {
	return nil, fmt.Errorf("seatbelt sandboxing is only available on macOS")
}

// SeatbeltAvailable always returns false on non-darwin platforms.
func SeatbeltAvailable() bool {
	return false
}
