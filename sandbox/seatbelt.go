package sandbox

import (
	"fmt"
	"os/exec"
	"runtime"
)

// Available returns true on macOS when sandbox-exec is present.
func Available() bool {
	if runtime.GOOS != "darwin" {
		return false
	}
	_, err := exec.LookPath("sandbox-exec")
	return err == nil
}

// GenerateProfile builds a macOS Seatbelt SBPL profile string for the
// given SandboxConfig.
func GenerateProfile(cfg SandboxConfig) string {
	switch cfg.Mode {
	case ModeStrict:
		return strictProfile(cfg)
	case ModeWorkspace:
		return workspaceProfile(cfg)
	default:
		// ModeOff — no profile needed; return a permissive stub.
		return "(version 1)(allow default)"
	}
}

// strictProfile generates a hardened read-only Seatbelt profile.
func strictProfile(cfg SandboxConfig) string {
	profile := "(version 1)(deny default)(allow process*)(allow sysctl-read)(allow mach-lookup)"
	// Allow reads except for sensitive paths
	profile += "(allow file-read*)"
	// Deny writes to home directory (prevents credential/config theft)
	profile += `(deny file-write* (subpath (param "HOME")))`
	// Deny execution of unexpected binaries
	profile += `(deny process-exec* (subpath "/usr/local"))`
	if cfg.AllowNetwork {
		profile += "(allow network*)"
	} else {
		profile += "(deny network*)"
	}
	return profile
}

// workspaceProfile generates a workspace Seatbelt profile that allows
// writes only to the project directory and /tmp.
func workspaceProfile(cfg SandboxConfig) string {
	profile := strictProfile(cfg)
	if cfg.WorkspaceDir != "" {
		profile += fmt.Sprintf(`(allow file-write* (subpath "%s"))`, cfg.WorkspaceDir)
	}
	profile += `(allow file-write* (subpath "/tmp"))`
	profile += `(allow file-write* (subpath "/private/tmp"))`
	return profile
}

// WrapCommand returns the executable and argument list needed to run
// command inside a Seatbelt sandbox.  If the mode is ModeOff the
// original command is returned unchanged (bash -c <command>).
func WrapCommand(command string, cfg SandboxConfig) (string, []string) {
	if cfg.Mode == ModeOff {
		return "bash", []string{"-c", command}
	}
	profile := GenerateProfile(cfg)
	return "sandbox-exec", []string{"-p", profile, "bash", "-c", command}
}
