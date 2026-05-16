//go:build darwin

// Package sandbox provides sandbox mode for isolated command execution.
package sandbox

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// SeatbeltPolicy describes the permissions for a macOS seatbelt sandbox profile.
type SeatbeltPolicy struct {
	AllowNetwork bool     // allow outbound/inbound network access
	AllowWrite   bool     // allow filesystem writes (to WritablePaths only)
	ReadablePaths []string // paths allowed for file-read*
	WritablePaths []string // paths allowed for file-write*
	AllowProcess bool     // allow spawning child processes (process-exec*)
	AllowSysctl  bool     // allow sysctl-read
}

// GenerateSeatbeltProfile generates a valid Apple sandbox-exec SBPL
// (Scheme-Based Profile Language) string from the given policy.
func GenerateSeatbeltProfile(policy *SeatbeltPolicy) string {
	var b strings.Builder

	b.WriteString("(version 1)\n")
	b.WriteString("(deny default)\n")

	// Always allow basic mach-lookup for system functionality.
	b.WriteString("(allow mach-lookup)\n")

	// Sysctl read for basic system queries.
	if policy.AllowSysctl {
		b.WriteString("(allow sysctl-read)\n")
	}

	// Process execution.
	if policy.AllowProcess {
		b.WriteString("(allow process-exec*)\n")
		b.WriteString("(allow process-fork)\n")
	}

	// Network access.
	if policy.AllowNetwork {
		b.WriteString("(allow network*)\n")
	}

	// Readable paths.
	for _, p := range policy.ReadablePaths {
		_, _ = fmt.Fprintf(&b, "(allow file-read* (subpath \"%s\"))\n", p)
	}

	// Writable paths (only if AllowWrite is true).
	if policy.AllowWrite {
		for _, p := range policy.WritablePaths {
		_, _ = fmt.Fprintf(&b, "(allow file-write* (subpath \"%s\"))\n", p)
		}
	}

	return b.String()
}

// DefaultHawkPolicy creates a sensible default SeatbeltPolicy for hawk
// operations in the given working directory.
func DefaultHawkPolicy(workDir string) *SeatbeltPolicy {
	home := os.Getenv("HOME")
	gopath := os.Getenv("GOPATH")
	if gopath == "" {
		gopath = filepath.Join(home, "go")
	}

	hawkDir := filepath.Join(home, ".hawk")

	readPaths := []string{
		workDir,
		"/usr",
		"/bin",
		"/Library",
		"/System",
		"/dev",
		"/tmp",
		"/private/tmp",
		hawkDir,
		gopath,
	}

	writePaths := []string{
		workDir,
		"/tmp",
		"/private/tmp",
		"/dev/null",
		hawkDir,
	}

	return &SeatbeltPolicy{
		AllowNetwork:  true,
		AllowWrite:    true,
		ReadablePaths: readPaths,
		WritablePaths: writePaths,
		AllowProcess:  true,
		AllowSysctl:   true,
	}
}

// RunSeatbelted creates an exec.Cmd that runs the given command inside a
// macOS seatbelt sandbox using the provided policy. The profile is written
// to a temporary file and passed to sandbox-exec via the -f flag.
func RunSeatbelted(ctx context.Context, command string, policy *SeatbeltPolicy) (*exec.Cmd, error) {
	if runtime.GOOS != "darwin" {
		return nil, fmt.Errorf("seatbelt sandboxing is only available on macOS")
	}

	profile := GenerateSeatbeltProfile(policy)

	// Write the profile to a temp file.
	tmpFile, err := os.CreateTemp("", "hawk-seatbelt-*.sb")
	if err != nil {
		return nil, fmt.Errorf("failed to create seatbelt profile temp file: %w", err)
	}

	if _, err := tmpFile.WriteString(profile); err != nil {
		_ = tmpFile.Close()
		_ = os.Remove(tmpFile.Name())
		return nil, fmt.Errorf("failed to write seatbelt profile: %w", err)
	}
	_ = tmpFile.Close()

	// Build the sandbox-exec command.
	cmd := exec.CommandContext(ctx, "sandbox-exec", "-f", tmpFile.Name(), "bash", "-c", command)

	// Pass through environment, ensuring HOME is set.
	cmd.Env = os.Environ()

	return cmd, nil
}

// SeatbeltAvailable returns true on macOS when sandbox-exec is present.
func SeatbeltAvailable() bool {
	if runtime.GOOS != "darwin" {
		return false
	}
	_, err := exec.LookPath("sandbox-exec")
	return err == nil
}
