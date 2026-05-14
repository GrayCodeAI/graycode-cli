package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// DevEnvTool allows the agent to read, write, and build Docker environments
// dynamically. When the agent needs a tool that isn't installed, it can modify
// the Dockerfile and rebuild the container on-the-fly (inspired by herm).
type DevEnvTool struct{}

func (DevEnvTool) Name() string      { return "DevEnv" }
func (DevEnvTool) RiskLevel() string { return "medium" }
func (DevEnvTool) Aliases() []string { return []string{"devenv", "dev_env"} }
func (DevEnvTool) Description() string {
	return "Manage the container development environment. Read, write, or build a project Dockerfile for the sandboxed execution environment."
}

func (DevEnvTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"action": map[string]interface{}{
				"type":        "string",
				"enum":        []string{"read", "write", "build"},
				"description": "Action: read (view current Dockerfile), write (replace Dockerfile), build (rebuild image and hot-swap container)",
			},
			"dockerfile": map[string]interface{}{
				"type":        "string",
				"description": "Dockerfile content (required for 'write' action)",
			},
		},
		"required": []string{"action"},
	}
}

func (DevEnvTool) Execute(ctx context.Context, input json.RawMessage) (string, error) {
	var p struct {
		Action     string `json:"action"`
		Dockerfile string `json:"dockerfile"`
	}
	if err := json.Unmarshal(input, &p); err != nil {
		return "", err
	}

	workDir, _ := os.Getwd()
	dfPath := filepath.Join(workDir, ".hawk", "Dockerfile")

	switch p.Action {
	case "read":
		content, err := os.ReadFile(dfPath)
		if err != nil {
			if os.IsNotExist(err) {
				return "No Dockerfile found at .hawk/Dockerfile. Use action='write' to create one.", nil
			}
			return "", err
		}
		return string(content), nil

	case "write":
		if p.Dockerfile == "" {
			return "", fmt.Errorf("dockerfile content is required for write action")
		}
		if !strings.Contains(p.Dockerfile, "FROM") {
			return "", fmt.Errorf("dockerfile must contain a FROM instruction")
		}

		if err := os.MkdirAll(filepath.Dir(dfPath), 0755); err != nil {
			return "", err
		}

		// Back up existing Dockerfile
		if _, err := os.Stat(dfPath); err == nil {
			_ = os.Rename(dfPath, dfPath+".old")
		}

		if err := os.WriteFile(dfPath, []byte(p.Dockerfile), 0644); err != nil {
			return "", err
		}
		return fmt.Sprintf("Dockerfile written to %s. Use action='build' to rebuild the container.", dfPath), nil

	case "build":
		content, err := os.ReadFile(dfPath)
		if err != nil {
			return "", fmt.Errorf("no Dockerfile at %s — use action='write' first", dfPath)
		}
		// The actual build is delegated to the ContainerSandbox.BuildFromDockerfile
		// which is wired up at the session level. Here we just validate.
		if !strings.Contains(string(content), "FROM") {
			return "", fmt.Errorf("invalid Dockerfile: missing FROM instruction")
		}
		return fmt.Sprintf("Build requested for Dockerfile (%d bytes). Container will be rebuilt and hot-swapped.", len(content)), nil

	default:
		return "", fmt.Errorf("unknown action: %s (use read, write, or build)", p.Action)
	}
}
