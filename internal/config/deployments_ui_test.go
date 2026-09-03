package config

import "testing"

func TestDeploymentRoutingLabel(t *testing.T) {
	t.Setenv("GRAYCODE_DEPLOYMENT_ROUTING", "")
	enabled := true
	if DeploymentRoutingLabel(Settings{DeploymentRouting: &enabled}) != "on" {
		t.Fatal("expected on")
	}
	disabled := false
	if DeploymentRoutingLabel(Settings{DeploymentRouting: &disabled}) != "off" {
		t.Fatal("expected off")
	}
}
