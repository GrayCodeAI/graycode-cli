package cmd

import (
	"context"
	"encoding/json"

	hawkconfig "github.com/GrayCodeAI/hawk/internal/config"
	"github.com/spf13/cobra"
)

var ecosystemJSON bool

var ecosystemCmd = &cobra.Command{
	Use:   "ecosystem",
	Short: "Show eyrie, harrier, and shrike integration status",
	Long:  "Print the ecosystem panel summarizing LLM provider (eyrie), memory graph (harrier), and token pipeline (shrike). Same block as the top of hawk doctor.",
	RunE: func(cmd *cobra.Command, args []string) error {
		settings, err := loadEffectiveSettings()
		if err != nil {
			return err
		}
		modelName, providerName := effectiveModelAndProvider(settings)
		if providerName == "" {
			providerName = "auto"
		}
		if ecosystemJSON {
			report := hawkconfig.BuildEcosystemReport(context.Background(), providerName, modelName)
			enc := json.NewEncoder(cmd.OutOrStdout())
			enc.SetIndent("", "  ")
			return enc.Encode(report)
		}
		cmd.Println(hawkconfig.FormatEcosystemPanel(context.Background(), providerName, modelName))
		return nil
	},
}

func init() {
	ecosystemCmd.Flags().BoolVar(&ecosystemJSON, "json", false, "output ecosystem report as JSON")
	rootCmd.AddCommand(ecosystemCmd)
}
