package cmd

import (
	"context"
	"fmt"

	hawkconfig "github.com/GrayCodeAI/hawk/internal/config"
	"github.com/spf13/cobra"
)

var soloStrict bool

var soloCmd = &cobra.Command{
	Use:   "solo",
	Short: "Developer path readiness (setup, security, sandbox, ecosystem)",
	Long: `Check whether hawk is configured on the developer path:
API keys in OS secret store, model selected, no secrets on disk,
Docker isolation when available, and eyrie/yaad/tok integration.

See docs/DEVELOPER-PATH.md and docs/SECURITY-SOLO.md.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()
		report := hawkconfig.EvaluateSoloPath(ctx)
		cmd.Println(hawkconfig.FormatSoloPathReport(ctx))

		if soloStrict {
			for _, c := range report.Checks {
				if c.Section == "Sandbox" && c.Name == "docker" && c.Status == hawkconfig.SoloWarn {
					return fmt.Errorf("solo strict: start Docker for isolated Bash")
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
	soloCmd.Flags().BoolVar(&soloStrict, "strict", false, "Also require Docker for Bash isolation")
	rootCmd.AddCommand(soloCmd)
}
