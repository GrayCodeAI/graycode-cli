package cmd

import (
	"context"
	"time"

	hawkconfig "github.com/GrayCodeAI/hawk/internal/config"
	"github.com/spf13/cobra"
)

var modelsCmd = &cobra.Command{
	Use:   "models",
	Short: "Deployment-aware model catalog (via eyrie)",
	Long: `Manage the eyrie model catalog used by hawk for models, pricing, and deployment routing.

The catalog is stored at ~/.eyrie/model_catalog.json (override with EYRIE_MODEL_CATALOG_PATH).
Hawk refreshes the catalog automatically on startup when the cache is missing, empty, or stale (disable with --no-auto-catalog-refresh or HAWK_AUTO_REFRESH_CATALOG=0).
Use 'hawk models refresh' for a manual refresh or full discover report.`,
}

var modelsRefreshCmd = &cobra.Command{
	Use:     "refresh",
	Aliases: []string{"update"},
	Short:   "Discover model catalog (eyrie remote + live provider APIs) into ~/.eyrie/model_catalog.json",
	RunE: func(cmd *cobra.Command, _ []string) error {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		summary, err := hawkconfig.RefreshModelCatalogV1(ctx)
		if err != nil {
			return err
		}
		cmd.Println(summary)
		return nil
	},
}

var modelsStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show cached catalog metadata and deployment routing status",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()
		cmd.Println(hawkconfig.FormatCatalogHealth(hawkconfig.CatalogHealthReport(ctx)))
		cmd.Println()
		settings, err := loadEffectiveSettings()
		if err != nil {
			return err
		}
		model, _ := effectiveModelAndProvider(settings)
		if len(args) > 0 {
			model = args[0]
		}
		report, err := hawkconfig.DeploymentStatusReport(ctx, model)
		if err != nil {
			return err
		}
		cmd.Println(report)
		return nil
	},
}

var modelsRoutingPreviewCmd = &cobra.Command{
	Use:   "routing-preview <model>",
	Short: "Print effective deployment routing JSON for a model",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		model := args[0]
		out, err := hawkconfig.RoutingPreviewJSON(context.Background(), model)
		if err != nil {
			return err
		}
		cmd.Println(out)
		return nil
	},
}

var modelsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List model IDs from the eyrie catalog cache",
	RunE: func(cmd *cobra.Command, args []string) error {
		provider := ""
		if len(args) > 0 {
			provider = args[0]
		}
		models, err := hawkconfig.FetchModelsForProvider(provider)
		if err != nil {
			return err
		}
		cmd.Printf("%d models", len(models))
		if provider != "" {
			cmd.Printf(" for provider %q", provider)
		}
		cmd.Println()
		for _, m := range models {
			name := m.DisplayName
			if name == "" {
				name = m.ID
			}
			cmd.Printf("  %s\n", name)
		}
		return nil
	},
}

func init() {
	modelsCmd.AddCommand(modelsRefreshCmd)
	modelsCmd.AddCommand(modelsListCmd)
	modelsCmd.AddCommand(modelsStatusCmd)
	modelsCmd.AddCommand(modelsRoutingPreviewCmd)
	rootCmd.AddCommand(modelsCmd)
}
