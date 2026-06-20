package config

// DeploymentRoutingLabel returns a short on/off label for the config hub.
func DeploymentRoutingLabel(settings Settings) string {
	if DeploymentRoutingEnabled(settings) {
		return "on"
	}
	return "off"
}

// SaveProjectOrGlobalDeploymentRouting persists the flag to user settings.
func SaveProjectOrGlobalDeploymentRouting(enabled bool) error {
	val := "false"
	if enabled {
		val = "true"
	}
	return SetGlobalSetting("deployment_routing", val)
}
