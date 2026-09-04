package cmd

import (
	"context"
	"encoding/json"
	"fmt"

	graycodeconfig "github.com/GrayCodeAI/graycode-cli/internal/config"
	"github.com/spf13/cobra"
)

var (
	pathStrict bool
	pathJSON   bool
)

var pathCmd = &cobra.Command{
	Use:   "path",
	Short: "Developer path readiness (setup, security, sandbox, ecosystem)",
	Long: `Check whether graycode is configured on the developer path:
API keys in OS secret store, model selected, no secrets on disk,
mandatory Docker isolation, and eyrie/harrier/shrike integration.

Built for individual developers first — teams and enterprise later.

See docs/DEVELOPER-PATH.md and docs/SECURITY-DEVELOPER.md.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()
		report := graycodeconfig.EvaluateDeveloperPath(ctx)

		if pathJSON {
			enc := json.NewEncoder(cmd.OutOrStdout())
			enc.SetIndent("", "  ")
			return enc.Encode(report)
		}

		cmd.Println(graycodeconfig.FormatDeveloperPathReport(ctx))

		if pathStrict {
			for _, c := range report.Checks {
				if c.Section == "Sandbox" && c.Name == "docker" && c.Status == graycodeconfig.PathWarn {
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
	pathCmd.Flags().BoolVar(&pathStrict, "strict", false, "compatibility flag; Docker isolation is always required")
	pathCmd.Flags().BoolVar(&pathJSON, "json", false, "output readiness report as JSON")
	rootCmd.AddCommand(pathCmd)
}
