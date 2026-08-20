package sandbox

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"time"
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
	case "windows":
		return selectWindows(level)
	default:
		return SandboxSelection{Backend: "none", Reason: "unsupported platform: " + runtime.GOOS}
	}
}

func selectWindows(level IsolationLevel) SandboxSelection {
	switch level {
	case IsolationMaximum, IsolationContainer:
		if dockerAvailable() {
			return SandboxSelection{Backend: "docker", Reason: "container isolation via Docker"}
		}
	}

	if WindowsACLAvailable() {
		return SandboxSelection{
			Backend: "windows_acl",
			Reason:  "Windows native Access Control Entries (zero overhead)",
		}
	}
	if dockerAvailable() {
		return SandboxSelection{Backend: "docker", Reason: "Docker container (fallback)"}
	}
	return SandboxSelection{Backend: "none", Reason: "no sandbox backend available"}
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

const (
	dockerAvailabilityTTL = 2 * time.Second
	dockerProbeTimeout    = 1500 * time.Millisecond
)

var (
	dockerAvailabilityMu      sync.Mutex
	dockerAvailabilityChecked time.Time
	dockerAvailabilityCached  bool
	dockerAvailabilityProbe   = probeDockerAvailable
)

func dockerAvailable() bool {
	dockerAvailabilityMu.Lock()
	defer dockerAvailabilityMu.Unlock()
	if !dockerAvailabilityChecked.IsZero() && time.Since(dockerAvailabilityChecked) < dockerAvailabilityTTL {
		return dockerAvailabilityCached
	}
	dockerAvailabilityCached = dockerAvailabilityProbe()
	dockerAvailabilityChecked = time.Now()
	return dockerAvailabilityCached
}

// ResetDockerAvailabilityCache clears the cached daemon probe result so the
// next availability check performs a fresh docker info probe.
func ResetDockerAvailabilityCache() {
	dockerAvailabilityMu.Lock()
	defer dockerAvailabilityMu.Unlock()
	dockerAvailabilityChecked = time.Time{}
	dockerAvailabilityCached = false
}

// dockerCandidates are the locations to check for the docker CLI. LookPath
// covers PATH; the explicit OrbStack paths cover installs where the CLI lives
// in ~/.orbstack/bin but the test/daemon subprocess PATH or HOME omits it.
func dockerCandidates() []string {
	if p, err := exec.LookPath("docker"); err == nil {
		return []string{p}
	}
	// OrbStack installs; check both $HOME (production) and /Users/$USER (test
	// harness overrides HOME to a temp dir but preserves USER).
	for _, home := range []string{os.Getenv("HOME"), filepath.Join("/Users", os.Getenv("USER"))} {
		if home == "" {
			continue
		}
		candidate := filepath.Join(home, ".orbstack", "bin", "docker")
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return []string{candidate}
		}
	}
	return nil
}

func probeDockerAvailable() bool {
	candidates := dockerCandidates()
	if len(candidates) == 0 {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), dockerProbeTimeout)
	defer cancel()
	// candidates come from our own path discovery (PATH lookups and the
	// hardcoded OrbStack install location), never from user input.
	cmd := exec.CommandContext(ctx, candidates[0], "info") // #nosec G204 -- candidate is internally derived, not external input
	cmd.Stdout = nil
	cmd.Stderr = nil
	return cmd.Run() == nil
}

func resetDockerAvailabilityCache() {
	ResetDockerAvailabilityCache()
}
