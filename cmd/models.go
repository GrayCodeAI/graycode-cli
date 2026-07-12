package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/GrayCodeAI/eyrie/runtime"
	hawkconfig "github.com/GrayCodeAI/hawk/internal/config"
	"github.com/spf13/cobra"
)

var (
	modelsListJSON bool
	modelsListLive bool
	modelsListRaw  bool
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
		modelName, _ := effectiveModelAndProvider(settings)
		if len(args) > 0 {
			modelName = args[0]
		}
		report, err := hawkconfig.DeploymentStatusReport(ctx, modelName)
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
		modelName := args[0]
		out, err := hawkconfig.RoutingPreviewJSON(context.Background(), modelName)
		if err != nil {
			return err
		}
		cmd.Println(out)
		return nil
	},
}

var modelsListCmd = &cobra.Command{
	Use:   "list [provider]",
	Short: "List models from the eyrie catalog cache (or live provider API)",
	RunE: func(cmd *cobra.Command, args []string) error {
		providerName := ""
		if len(args) > 0 {
			providerName = args[0]
		}
		ctx := context.Background()
		var models []runtime.ModelEntry
		var err error
		if modelsListLive {
			if providerName == "" {
				return fmt.Errorf("provider required with --live (e.g. hawk models list canopywave --live --json)")
			}
			models, err = runtime.ListModels(ctx, runtime.ListModelsOpts{
				ProviderID: hawkconfig.ActiveProviderID(providerName),
				Source:     runtime.ListSourceLive,
			})
		} else {
			models, err = hawkconfig.FetchModelsForProvider(providerName)
		}
		if err != nil {
			return err
		}
		if modelsListJSON || modelsListRaw {
			if modelsListRaw {
				raw := make([]json.RawMessage, 0, len(models))
				for _, m := range models {
					if len(m.LiveMetadata) > 0 {
						raw = append(raw, m.LiveMetadata)
					}
				}
				if len(raw) == 0 && modelsListLive {
					for _, m := range models {
						b, merr := json.Marshal(m)
						if merr != nil {
							return merr
						}
						raw = append(raw, b)
					}
				}
				out, merr := json.MarshalIndent(raw, "", "  ")
				if merr != nil {
					return merr
				}
				cmd.Println(string(out))
				return nil
			}
			out, merr := json.MarshalIndent(models, "", "  ")
			if merr != nil {
				return merr
			}
			cmd.Println(string(out))
			return nil
		}
		cmd.Printf("%d models", len(models))
		if providerName != "" {
			cmd.Printf(" for provider %q", providerName)
		}
		cmd.Println()
		rows := make([]modelTableRow, len(models))
		for i, m := range models {
			rows[i] = modelTableRowFromRuntimeEntry(m)
		}
		printModelTablePlain(rows)
		return nil
	},
}

func init() {
	modelsListCmd.Flags().BoolVar(&modelsListJSON, "json", false, "Print full catalog entries as JSON (includes live_metadata when cached)")
	modelsListCmd.Flags().BoolVar(&modelsListLive, "live", false, "Fetch directly from provider API instead of cache")
	modelsListCmd.Flags().BoolVar(&modelsListRaw, "raw", false, "With --json, print only provider live_metadata objects (same shape as /v1/models data[] items)")
	modelsCmd.AddCommand(modelsRefreshCmd)
	modelsCmd.AddCommand(modelsListCmd)
	modelsCmd.AddCommand(modelsStatusCmd)
	modelsCmd.AddCommand(modelsRoutingPreviewCmd)
	rootCmd.AddCommand(modelsCmd)
}
