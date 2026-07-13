package config

// DeploymentRoutingEnabled delegates deployment-routing policy ownership to Eyrie runtime.
func DeploymentRoutingEnabled(s Settings) bool {
	engine, err := newEyrieEngine()
	return err == nil && engine.DeploymentRoutingEnabled(s.DeploymentRouting)
}
