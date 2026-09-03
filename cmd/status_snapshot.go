package cmd

import (
	"context"
	"fmt"
	"strings"

	graycodeconfig "github.com/GrayCodeAI/graycode-cli/internal/config"
	"github.com/GrayCodeAI/graycode-cli/internal/engine"
	"github.com/GrayCodeAI/graycode-cli/internal/plugin"
	"github.com/GrayCodeAI/graycode-cli/internal/sandbox"
	"github.com/GrayCodeAI/graycode-cli/internal/status"
	"github.com/spf13/cobra"
)

var statusJSON bool

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show a redacted runtime status snapshot",
	RunE: func(cmd *cobra.Command, _ []string) error {
		snapshot := buildStatusSnapshot()
		if statusJSON {
			data, err := snapshot.JSON()
			if err != nil {
				return err
			}
			_, err = cmd.OutOrStdout().Write(append(data, '\n'))
			return err
		}
		cmd.Print(formatStatusSnapshot(snapshot))
		return nil
	},
}

func buildStatusSnapshot() status.Snapshot {
	snapshot := status.New()
	snapshot.GraycodeVersion = version
	snapshot.Workspace = status.Workspace()
	snapshot.GitBranch = engine.InspectGitBranch("").Branch
	settings := graycodeconfig.LoadGlobalSettings()
	selection := graycodeconfig.EffectiveSelection(context.Background(), graycodeconfig.SelectionOptions{})
	snapshot.Model = strings.TrimSpace(selection.Model)
	snapshot.Provider = strings.TrimSpace(selection.Provider)
	if snapshot.Model == "" {
		snapshot.Model = strings.TrimSpace(settings.Model)
	}
	if snapshot.Provider == "" {
		snapshot.Provider = strings.TrimSpace(settings.Provider)
	}
	snapshot.Permission.SandboxMode = settings.Sandbox
	// Resolve the native confinement backend the way execution would, so the
	// snapshot shows the real isolation technology (seatbelt/landlock/docker…)
	// rather than only the requested policy label.
	if sel := sandbox.SelectSandbox(sandbox.IsolationDefault, snapshot.Workspace); sel.Backend != "" {
		snapshot.Permission.SandboxBackend = sel.Backend
	}
	snapshot.Permission.EffectiveRules = len(settings.AllowedTools) + len(settings.DisallowedTools) + len(settings.AutoAllow)
	if settings.AutonomyExplicit {
		snapshot.Permission.AutonomyTier = fmt.Sprintf("%d", settings.Autonomy)
		switch settings.Autonomy {
		case 0:
			snapshot.Permission.Mode = "ask"
		case 4:
			snapshot.Permission.Mode = "yolo"
		default:
			snapshot.Permission.Mode = "auto"
		}
	}
	snapshot.MCP.Configured = len(settings.MCPServers)
	snapshot.MCP.State = "not_loaded"
	snapshot.Skills.State = "discovery_deferred"
	if entries, err := plugin.DefaultRegistry.List(context.Background(), snapshot.Workspace); err == nil {
		snapshot.Skills.Configured = len(entries)
		snapshot.Skills.State = "available"
	}
	if engine.ProjectTrust(snapshot.Workspace).Blocked {
		snapshot.Warnings = append(snapshot.Warnings, "project automation is blocked until this folder is trusted")
	}
	return snapshot
}

func formatStatusSnapshot(s status.Snapshot) string {
	backend := ""
	if s.Permission.SandboxBackend != "" {
		backend = " (" + s.Permission.SandboxBackend + ")"
	}
	return fmt.Sprintf("Graycode status\nSchema: %s\nWorkspace: %s\nGit branch: %s\nProvider: %s\nModel: %s\nAutonomy tier: %s\nSandbox: %s%s\nPermission rules: %d\nMCP: %d configured (%s)\nSkills: %d (%s)\nSecrets redacted: %t\n",
		s.SchemaVersion, s.Workspace, s.GitBranch, s.Provider, s.Model,
		s.Permission.AutonomyTier, s.Permission.SandboxMode, backend,
		s.Permission.EffectiveRules, s.MCP.Configured, s.MCP.State,
		s.Skills.Configured, s.Skills.State, s.Permission.SecretRedacted)
}

func init() {
	statusCmd.Flags().BoolVar(&statusJSON, "json", false, "output the status snapshot as JSON")
	rootCmd.AddCommand(statusCmd)
}
