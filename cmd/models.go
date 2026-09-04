package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	graycodeconfig "github.com/GrayCodeAI/graycode-cli/internal/config"
	"github.com/spf13/cobra"
)

var (
	modelsListJSON bool
	modelsListLive bool
	modelsListRaw  bool
)

var modelsCmd = &cobra.Command{
	Use:   "models",
	Short: "Deployment-aware model catalog (via graycode-router)",
	Long: `Manage the graycode-router model catalog used by graycode for models, pricing, and deployment routing.

The catalog is stored at ~/.graycode-router/model_catalog.json (override with GRAYCODE_ROUTER_MODEL_CATALOG_PATH).
Graycode refreshes the catalog automatically on startup when the cache is missing, empty, or stale (disable with --no-auto-catalog-refresh or GRAYCODE_AUTO_REFRESH_CATALOG=0).
Use 'graycode models refresh' for a manual refresh or full discover report.`,
}

var modelsRefreshCmd = &cobra.Command{
	Use:     "refresh",
	Aliases: []string{"update"},
	Short:   "Discover model catalog (graycode-router remote + live provider APIs) into ~/.graycode-router/model_catalog.json",
	RunE: func(cmd *cobra.Command, _ []string) error {
		settings, err := loadEffectiveSettings()
		if err != nil {
			return err
		}
		ctx, cancel := context.WithTimeout(cmd.Context(), 60*time.Second)
		defer cancel()
		summary, err := graycodeconfig.RefreshModelCatalogV1WithSettings(ctx, settings)
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
		ctx := cmd.Context()
		settings, err := loadEffectiveSettings()
		if err != nil {
			return err
		}
		cmd.Println(graycodeconfig.FormatCatalogHealth(graycodeconfig.CatalogHealthReport(ctx)))
		cmd.Println()
		modelName, _ := effectiveModelAndProvider(settings)
		if len(args) > 0 {
			modelName = args[0]
		}
		report, err := graycodeconfig.DeploymentStatusReportWithSettings(ctx, settings, modelName)
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
		settings, err := loadEffectiveSettings()
		if err != nil {
			return err
		}
		modelName := args[0]
		out, err := graycodeconfig.RoutingPreviewJSONWithSettings(cmd.Context(), settings, modelName)
		if err != nil {
			return err
		}
		cmd.Println(out)
		return nil
	},
}

var modelsListCmd = &cobra.Command{
	Use:   "list [provider]",
	Short: "List models from the graycode-router catalog cache (or live provider API)",
	RunE: func(cmd *cobra.Command, args []string) error {
		settings, err := loadEffectiveSettings()
		if err != nil {
			return err
		}
		providerName := ""
		if len(args) > 0 {
			providerName = args[0]
		}
		ctx := cmd.Context()
		var models []graycodeconfig.EngineModel
		if modelsListLive {
			if providerName == "" {
				return fmt.Errorf("provider required with --live (e.g. graycode models list canopywave --live --json)")
			}
			models, err = graycodeconfig.ListLiveEngineModelsWithSettings(ctx, settings, graycodeconfig.ActiveProviderID(providerName))
		} else {
			models, err = graycodeconfig.FetchModelsForProviderWithSettings(ctx, settings, providerName)
		}
		if err != nil {
			return err
		}
		if modelsListJSON || modelsListRaw {
			out, merr := marshalModelListJSON(models, modelsListRaw, modelsListLive)
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
			rows[i] = modelTableRowFromCatalogEntry(m)
		}
		printModelTablePlain(rows)
		return nil
	},
}

// modelListJSONEntry is Graycode's versioned command-output contract. Keep this
// separate from GraycodeRouter's host-facing Model DTO so engine-only fields can evolve
// without breaking users that consume `graycode models list --json`.
type modelListJSONEntry struct {
	ID               string          `json:"id"`
	InputPricePer1M  float64         `json:"input_price_per_1m"`
	OutputPricePer1M float64         `json:"output_price_per_1m"`
	ContextWindow    int             `json:"context_window"`
	MaxOutput        int             `json:"max_output"`
	ServerTools      []string        `json:"server_tools,omitempty"`
	DisplayName      string          `json:"display_name,omitempty"`
	Description      string          `json:"description,omitempty"`
	Owner            string          `json:"owner,omitempty"`
	LiveMetadata     json.RawMessage `json:"live_metadata,omitempty"`
}

func modelListJSONEntryFromEngine(m graycodeconfig.EngineModel) modelListJSONEntry {
	return modelListJSONEntry{
		ID:               m.ID,
		InputPricePer1M:  m.InputPricePer1M,
		OutputPricePer1M: m.OutputPricePer1M,
		ContextWindow:    m.ContextWindow,
		MaxOutput:        m.MaxOutputTokens,
		ServerTools:      append([]string(nil), m.Capabilities...),
		DisplayName:      m.DisplayName,
		Description:      m.Description,
		Owner:            m.Owner,
		LiveMetadata:     validModelLiveMetadata(m.LiveMetadata),
	}
}

func validModelLiveMetadata(raw json.RawMessage) json.RawMessage {
	metadata := json.RawMessage(strings.TrimSpace(string(raw)))
	if len(metadata) == 0 || !json.Valid(metadata) || string(metadata) == "null" {
		return nil
	}
	return append(json.RawMessage(nil), metadata...)
}

func marshalModelListJSON(models []graycodeconfig.EngineModel, rawOnly, live bool) ([]byte, error) {
	entries := make([]modelListJSONEntry, len(models))
	for i, model := range models {
		entries[i] = modelListJSONEntryFromEngine(model)
	}
	return marshalModelListEntriesJSON(entries, rawOnly, live)
}

func marshalModelListEntriesJSON(entries []modelListJSONEntry, rawOnly, live bool) ([]byte, error) {
	if !rawOnly {
		return json.MarshalIndent(entries, "", "  ")
	}

	raw := make([]json.RawMessage, 0, len(entries))
	for _, entry := range entries {
		metadata := json.RawMessage(strings.TrimSpace(string(entry.LiveMetadata)))
		if len(metadata) > 0 && json.Valid(metadata) && metadata[0] == '{' {
			raw = append(raw, append(json.RawMessage(nil), metadata...))
		}
	}
	// Preserve the original --raw contract: cached output contains only native
	// metadata, and mixed live output also omits rows without native metadata.
	if len(raw) > 0 || !live {
		return json.MarshalIndent(raw, "", "  ")
	}

	// Some provider APIs do not expose native model objects. Only in that live,
	// all-metadata-absent case, return stable compatibility rows instead of an
	// unhelpful empty result.
	for _, entry := range entries {
		entry.LiveMetadata = nil
		fallback, err := json.Marshal(entry)
		if err != nil {
			return nil, err
		}
		raw = append(raw, fallback)
	}
	return json.MarshalIndent(raw, "", "  ")
}

func init() {
	modelsListCmd.Flags().BoolVar(&modelsListJSON, "json", false, "Print full catalog entries as JSON (includes live_metadata when cached)")
	modelsListCmd.Flags().BoolVar(&modelsListLive, "live", false, "Fetch directly from provider API instead of cache")
	modelsListCmd.Flags().BoolVar(&modelsListRaw, "raw", false, "Print provider-native model objects (live fetch falls back to stable rows when none are available)")
	modelsCmd.AddCommand(modelsRefreshCmd)
	modelsCmd.AddCommand(modelsListCmd)
	modelsCmd.AddCommand(modelsStatusCmd)
	modelsCmd.AddCommand(modelsRoutingPreviewCmd)
	rootCmd.AddCommand(modelsCmd)
}
