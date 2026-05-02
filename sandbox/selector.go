package sandbox

import (
	"os/exec"
	"runtime"
)

// IsolationLevel represents the desired sandbox strength.
type IsolationLevel string

const (
	IsolationDefault   IsolationLevel = "default"
	IsolationEnhanced  IsolationLevel = "enhanced"
	IsolationContainer IsolationLevel = "container"
	IsolationMaximum   IsolationLevel = "maximum"
	IsolationOff       IsolationLevel = "off"
)

// SandboxSelection represents the chosen sandbox backend.
type SandboxSelection struct {
	Backend string // "landlock", "seatbelt", "nsjail", "bwrap", "docker", "none"
	Reason  string // why this was selected
}

// SelectSandbox automatically chooses the best available sandbox backend
// for the current platform and requested isolation level.
//
// macOS: seatbelt (always available)
// Linux: landlock+seccomp > nsjail > bubblewrap > docker > none
func SelectSandbox(level IsolationLevel, projectDir string) SandboxSelection {
	if level == IsolationOff {
		return SandboxSelection{Backend: "none", Reason: "sandbox disabled by user"}
	}

	switch runtime.GOOS {
	case "darwin":
		return selectMacOS(level)
	case "linux":
		return selectLinux(level)
	default:
		return SandboxSelection{Backend: "none", Reason: "unsupported platform: " + runtime.GOOS}
	}
}

func selectMacOS(level IsolationLevel) SandboxSelection {
	// macOS only has seatbelt (sandbox-exec)
	mode := "workspace"
	if level == IsolationEnhanced || level == IsolationMaximum {
		mode = "strict"
	}
	return SandboxSelection{
		Backend: "seatbelt",
		Reason:  "macOS sandbox-exec (" + mode + " mode)",
	}
}

func selectLinux(level IsolationLevel) SandboxSelection {
	switch level {
	case IsolationMaximum:
		if dockerAvailable() {
			return SandboxSelection{Backend: "docker", Reason: "maximum isolation via container"}
		}
	case IsolationContainer:
		if dockerAvailable() {
			return SandboxSelection{Backend: "docker", Reason: "container isolation requested"}
		}
	}

	// Default/enhanced: try the lightest effective sandbox
	if LandlockAvailable() {
		return SandboxSelection{Backend: "landlock", Reason: "Linux Landlock + seccomp (zero overhead)"}
	}
	if nsjailAvailable() {
		return SandboxSelection{Backend: "nsjail", Reason: "nsjail (namespaces + seccomp + cgroups)"}
	}
	if bwrapAvailable() {
		return SandboxSelection{Backend: "bwrap", Reason: "bubblewrap (user namespaces)"}
	}
	if dockerAvailable() {
		return SandboxSelection{Backend: "docker", Reason: "Docker container (fallback)"}
	}

	return SandboxSelection{Backend: "none", Reason: "no sandbox backend available"}
}

func nsjailAvailable() bool {
	_, err := exec.LookPath("nsjail")
	return err == nil
}

func bwrapAvailable() bool {
	_, err := exec.LookPath("bwrap")
	return err == nil
}

func dockerAvailable() bool {
	_, err := exec.LookPath("docker")
	return err == nil
}
