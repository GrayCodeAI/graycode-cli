package cmd

import (
	"context"
	"fmt"
	"strings"

	hawkconfig "github.com/GrayCodeAI/hawk/internal/config"
	"github.com/GrayCodeAI/hawk/internal/engine"
	"github.com/GrayCodeAI/hawk/internal/status"
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
	snapshot.HawkVersion = version
	snapshot.Workspace = status.Workspace()
	snapshot.GitBranch = engine.InspectGitBranch("").Branch
	settings := hawkconfig.LoadGlobalSettings()
	selection := hawkconfig.EffectiveSelection(context.Background(), hawkconfig.SelectionOptions{})
	snapshot.Model = strings.TrimSpace(selection.Model)
	snapshot.Provider = strings.TrimSpace(selection.Provider)
	if snapshot.Model == "" {
		snapshot.Model = strings.TrimSpace(settings.Model)
	}
	if snapshot.Provider == "" {
		snapshot.Provider = strings.TrimSpace(settings.Provider)
	}
	snapshot.Permission.SandboxMode = settings.Sandbox
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
	return snapshot
}

func formatStatusSnapshot(s status.Snapshot) string {
	return fmt.Sprintf("Hawk status\nSchema: %s\nWorkspace: %s\nGit branch: %s\nProvider: %s\nModel: %s\nAutonomy tier: %s\nSandbox: %s\nPermission rules: %d\nSecrets redacted: %t\n",
		s.SchemaVersion, s.Workspace, s.GitBranch, s.Provider, s.Model,
		s.Permission.AutonomyTier, s.Permission.SandboxMode,
		s.Permission.EffectiveRules, s.Permission.SecretRedacted)
}

func init() {
	statusCmd.Flags().BoolVar(&statusJSON, "json", false, "output the status snapshot as JSON")
	rootCmd.AddCommand(statusCmd)
}
