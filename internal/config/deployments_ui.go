package config

import (
	"encoding/json"
	"fmt"
	"os"
)

// DeploymentRoutingLabel returns a short on/off label for the config hub.
func DeploymentRoutingLabel(settings Settings) string {
	if DeploymentRoutingEnabled(settings) {
		return "on"
	}
	return "off"
}

// SaveProjectOrGlobalDeploymentRouting persists the flag to project settings when present.
func SaveProjectOrGlobalDeploymentRouting(enabled bool) error {
	projectPath := projectSettingsPath()
	if _, err := os.Stat(projectPath); err == nil {
		var s Settings
		data, err := os.ReadFile(projectPath)
		if err != nil {
			return err
		}
		if json.Unmarshal(data, &s) != nil {
			return fmt.Errorf("parse project settings")
		}
		s.DeploymentRouting = &enabled
		out, err := json.MarshalIndent(s, "", "  ")
		if err != nil {
			return err
		}
		return os.WriteFile(projectPath, append(out, '\n'), 0o644)
	}
	val := "false"
	if enabled {
		val = "true"
	}
	return SetGlobalSetting("deployment_routing", val)
}
