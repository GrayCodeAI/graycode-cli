package cmd

import (
	"context"

	hawkconfig "github.com/GrayCodeAI/hawk/internal/config"
	"github.com/spf13/cobra"
)

var ecosystemCmd = &cobra.Command{
	Use:   "ecosystem",
	Short: "Show eyrie, yaad, and tok integration status",
	Long:  "Print the ecosystem panel summarizing LLM provider (eyrie), memory graph (yaad), and token pipeline (tok). Same block as the top of hawk doctor.",
	RunE: func(cmd *cobra.Command, args []string) error {
		settings, err := loadEffectiveSettings()
		if err != nil {
			return err
		}
		model, provider := effectiveModelAndProvider(settings)
		if provider == "" {
			provider = "auto"
		}
		cmd.Println(hawkconfig.FormatEcosystemPanel(context.Background(), provider, model))
		return nil
	},
}

func init() {
	rootCmd.AddCommand(ecosystemCmd)
}
