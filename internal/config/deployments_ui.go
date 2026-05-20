package config

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/GrayCodeAI/eyrie/catalog"
	eyriecfg "github.com/GrayCodeAI/eyrie/config"
	"github.com/GrayCodeAI/eyrie/setup"
)

// DeploymentRow is one catalog deployment with local credential status.
type DeploymentRow struct {
	ID         string
	Name       string
	ProviderID string
	Configured bool
	Status     string
	EnvVars    []EnvVarStatus
}

// EnvVarStatus tracks whether an env var is set for a deployment.
type EnvVarStatus struct {
	Name string
	Set  bool
}

// ListDeploymentRows lists catalog deployments and whether hawk can use them now.
func ListDeploymentRows(ctx context.Context) ([]DeploymentRow, error) {
	PrepareCredentialDiscovery(ctx)
	compiled, err := loadEyrieCatalogV1(ctx, false)
	if err != nil {
		return nil, err
	}
	cfg := eyriecfg.LoadProviderConfig("")
	cfg = eyriecfg.EnsureDeploymentConfigV2(cfg)
	configured := setup.ConfiguredDeployments(cfg)
	discoveryEnv := eyriecfg.DiscoveryEnvMap(ctx)

	ids := make([]string, 0, len(compiled.DeploymentsByID))
	for id := range compiled.DeploymentsByID {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	out := make([]DeploymentRow, 0, len(ids))
	for _, id := range ids {
		dep := compiled.DeploymentsByID[id]
		row := DeploymentRow{
			ID:         id,
			Name:       dep.Name,
			ProviderID: dep.ProviderID,
			EnvVars:    envStatusForDeployment(id, dep, discoveryEnv),
		}
		dc := eyriecfg.DeploymentConfigFromEnv(dep, discoveryEnv)
		if eyriecfg.DeploymentConfigured(id, dep, dc) {
			row.Configured = true
			row.Status = "ready"
		} else if _, ok := configured[id]; ok {
			row.Status = "incomplete"
		} else {
			row.Status = "needs credentials"
		}
		out = append(out, row)
	}
	return out, nil
}

func envStatusForDeployment(deploymentID string, dep catalog.DeploymentV1, discoveryEnv map[string]string) []EnvVarStatus {
	known := deploymentEnvVars(deploymentID)
	if len(dep.EnvFallbacks) > 0 {
		for _, fb := range dep.EnvFallbacks {
			known = append(known, fb.Env...)
		}
	}
	var out []EnvVarStatus
	seen := map[string]bool{}
	for _, env := range known {
		if env == "" || seen[env] {
			continue
		}
		seen[env] = true
		set := strings.TrimSpace(discoveryEnv[env]) != ""
		if !set {
			set = strings.TrimSpace(os.Getenv(env)) != ""
		}
		out = append(out, EnvVarStatus{Name: env, Set: set})
	}
	return out
}

func deploymentEnvVars(id string) []string {
	return catalog.EnvVarsForDeployment(id)
}

// DeploymentRoutingLabel returns a short on/off label for the config hub.
func DeploymentRoutingLabel(settings Settings) string {
	if DeploymentRoutingEnabled(settings) {
		return "on"
	}
	return "off"
}

// ToggleDeploymentRouting flips deployment_routing in global settings.
func ToggleDeploymentRouting(settings Settings) (Settings, bool, error) {
	enabled := DeploymentRoutingEnabled(settings)
	next := !enabled
	settings.DeploymentRouting = &next
	if err := SaveProjectOrGlobalDeploymentRouting(next); err != nil {
		return settings, enabled, err
	}
	return settings, next, nil
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

// SyncProviderConfigFromEnv re-applies eyrie catalog + env into provider.json (deployments + routing).
func SyncProviderConfigFromEnv() (string, error) {
	result, err := ApplyEyrieCredentials(context.Background())
	if err != nil {
		return "", err
	}
	return FormatApplyCredentialsSummary(result), nil
}

// ProviderConfigJSON returns the current provider.json as indented JSON (routing included).
func ProviderConfigJSON() (string, error) {
	cfg := eyriecfg.LoadProviderConfig("")
	if cfg == nil {
		return "{}", nil
	}
	raw, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return "", err
	}
	return string(raw), nil
}
