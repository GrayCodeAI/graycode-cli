package cmd

import (
	"context"
	"fmt"

	hawkconfig "github.com/GrayCodeAI/hawk/internal/config"
	"github.com/spf13/cobra"
)

var pathStrict bool

var pathCmd = &cobra.Command{
	Use:   "path",
	Short: "Developer path readiness (setup, security, sandbox, ecosystem)",
	Long: `Check whether hawk is configured on the developer path:
API keys in OS secret store, model selected, no secrets on disk,
Docker isolation when available, and eyrie/yaad/tok integration.

Built for individual developers first — teams and enterprise later.

See docs/DEVELOPER-PATH.md and docs/SECURITY-DEVELOPER.md.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()
		report := hawkconfig.EvaluateDeveloperPath(ctx)
		cmd.Println(hawkconfig.FormatDeveloperPathReport(ctx))

		if pathStrict {
			for _, c := range report.Checks {
				if c.Section == "Sandbox" && c.Name == "docker" && c.Status == hawkconfig.PathWarn {
					return fmt.Errorf("strict mode: start Docker for isolated Bash")
				}
			}
		}
		if !report.Ready {
			return fmt.Errorf("developer path not ready — %s", report.NextStep)
		}
		return nil
	},
}

func init() {
	pathCmd.Flags().BoolVar(&pathStrict, "strict", false, "Also require Docker for Bash isolation")
	rootCmd.AddCommand(pathCmd)
}
