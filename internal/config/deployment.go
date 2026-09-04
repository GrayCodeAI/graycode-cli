package config

// DeploymentRoutingEnabled delegates deployment-routing policy ownership to GraycodeRouter runtime.
func DeploymentRoutingEnabled(s Settings) bool {
	engine, err := newGraycodeRouterEngine()
	return err == nil && engine.DeploymentRoutingEnabled(s.DeploymentRouting)
}
