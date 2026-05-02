package sandbox

import (
	"os/exec"
	"strings"
)

// GVisorAvailable returns true if Docker is available with the runsc (gVisor) runtime.
func GVisorAvailable() bool {
	if !dockerAvailable() {
		return false
	}
	out, err := exec.Command("docker", "info", "--format", "{{.Runtimes}}").Output()
	if err != nil {
		return false
	}
	return strings.Contains(string(out), "runsc")
}

// GVisorDockerArgs returns additional Docker args to use gVisor runtime.
// This provides VM-class isolation without actual VMs.
func GVisorDockerArgs() []string {
	if GVisorAvailable() {
		return []string{"--runtime=runsc"}
	}
	return nil
}
