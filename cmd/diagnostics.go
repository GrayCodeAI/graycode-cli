package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	graycodeconfig "github.com/GrayCodeAI/graycode-cli/internal/config"
	"github.com/GrayCodeAI/graycode-cli/internal/intelligence/memory"
	"github.com/GrayCodeAI/graycode-cli/internal/resilience/health"
	"github.com/GrayCodeAI/graycode-cli/internal/session"
	"github.com/GrayCodeAI/graycode-cli/internal/storage"
	"github.com/GrayCodeAI/graycode-cli/internal/tool"
	"github.com/GrayCodeAI/graycode-cli/internal/ui/icons"
)

func doctorReport(settings graycodeconfig.Settings) string {
	// Diagnostics must report the requested/effective selection even when its
	// credential is missing; readiness and health sections explain why it is
	// not yet usable. Hiding it as auto/default makes misconfiguration harder
	// to diagnose.
	selection := resolveSelection(settings)
	modelName, providerName := selection.Model, selection.Provider
	if providerName == "" {
		providerName = "auto"
	}
	if modelName == "" {
		modelName = "default"
	}

	cwd, _ := os.Getwd()
	var b strings.Builder
	b.WriteString("Graycode doctor\n")
	b.WriteString(fmt.Sprintf("Version: %s\n", version))
	b.WriteString(fmt.Sprintf("Go version: %s\n", runtime.Version()))
	b.WriteString(fmt.Sprintf("Directory: %s\n", cwd))
	b.WriteString(fmt.Sprintf("Provider: %s\n", providerName))
	b.WriteString(fmt.Sprintf("Model: %s\n", modelName))

	// Binary size
	if exe, err := os.Executable(); err == nil {
		if info, err := os.Stat(exe); err == nil {
			b.WriteString(fmt.Sprintf("Binary size: %.1f MB\n", float64(info.Size())/(1024*1024)))
		}
	}

	// Ecosystem versions
	b.WriteString("\nEcosystem versions:\n")
	for _, component := range []struct{ directory, product string }{
		{directory: "graycode-router", product: "GraycodeRouter"},
		{directory: "harrier", product: "Harrier"},
		{directory: "shrike", product: "Shrike"},
		{directory: "kestrel", product: "Kestrel"},
		{directory: "merlin", product: "Merlin"},
		{directory: "swift", product: "Swift"},
	} {
		versionFile := filepath.Join(filepath.Dir(cwd), component.directory, "VERSION")
		// #nosec G304 -- versionFile is built from a fixed sibling-repo list
		if data, err := os.ReadFile(versionFile); err == nil {
			b.WriteString(fmt.Sprintf("  %s (%s): %s\n", component.product, component.directory, strings.TrimSpace(string(data))))
		} else {
			b.WriteString(fmt.Sprintf("  %s (%s): not checked out\n", component.product, component.directory))
		}
	}
	b.WriteString("\n" + graycodeconfig.FormatEcosystemPanel(context.Background(), providerName, modelName) + "\n")
	b.WriteString("\n" + graycodeconfig.FormatCatalogHealth(graycodeconfig.CatalogHealthReport(context.Background())) + "\n")
	preflight := graycodeconfig.EnginePreflightReportWithSettings(context.Background(), settings, graycodeconfig.EnginePreflightOptions{})
	b.WriteString("\n" + graycodeconfig.FormatEnginePreflight(preflight) + "\n")
	b.WriteString("\n" + graycodeconfig.CredentialStorageStatus(context.Background()).Formatted + "\n")
	if deployReport, err := graycodeconfig.DeploymentStatusReportWithSettings(context.Background(), settings, modelName); err == nil {
		b.WriteString("\n" + deployReport + "\n")
	}
	b.WriteString("\n" + envSummaryWithSelection(providerName, modelName, false) + "\n")
	b.WriteString("\nGit:\n")
	if branch := branchSummary(); branch != "" {
		for _, line := range strings.Split(branch, "\n") {
			b.WriteString("  " + line + "\n")
		}
	}

	// Project instructions
	if md := graycodeconfig.LoadAgentsMD(); md != "" {
		b.WriteString("\nProject instructions: found\n")
	} else {
		b.WriteString("\nProject instructions: not found (consider creating AGENTS.md)\n")
	}

	// Installed skills (graycode ships none; skills come from user/marketplace installs)
	skillsDir := filepath.Join(storage.StateDir(), "skills")
	if entries, err := os.ReadDir(skillsDir); err == nil && len(entries) > 0 {
		b.WriteString(fmt.Sprintf("Installed skills: %d\n", len(entries)))
	} else {
		b.WriteString("Installed skills: none (install with `graycode skills install`)\n")
	}

	b.WriteString(fmt.Sprintf("Configured MCP servers: %d\n", len(settings.MCPServers)+len(mcpServers)))
	b.WriteString(fmt.Sprintf("Built-in tools: %d\n", len(allTools())))

	// Session recovery status
	recoveryCandidates := session.ScanForRecovery()
	if len(recoveryCandidates) > 0 {
		b.WriteString(fmt.Sprintf("\nInterrupted sessions: %d (run graycode recover)\n", len(recoveryCandidates)))
	} else {
		b.WriteString("\nInterrupted sessions: none\n")
	}

	b.WriteString("\n" + healthCheckReport(settings, providerName) + "\n")
	return strings.TrimRight(b.String(), "\n")
}

func healthCheckReport(settings graycodeconfig.Settings, provider string) string {
	registry := health.NewRegistry()

	registry.Register("api_key", providerCredentialHealthChecker(provider))

	// Settings validation
	registry.Register("config", func(ctx context.Context) health.Check {
		result := graycodeconfig.ValidateSettings(settings)
		if result.Valid {
			return health.Check{Name: "config", Status: health.Healthy, Message: "Configuration valid"}
		}
		return health.Check{Name: "config", Status: health.Degraded, Message: result.Error()}
	})

	// Harrier memory bridge check
	bridge := memory.NewHarrierBridge()
	if bridge.Ready() {
		registry.Register("harrier", func(ctx context.Context) health.Check {
			_, _, err := bridge.SearchByType("convention", 1)
			if err != nil {
				return health.Check{Name: "harrier", Status: health.Unhealthy, Message: "Harrier bridge initialized but query failed"}
			}
			return health.Check{Name: "harrier", Status: health.Healthy, Message: "Harrier memory bridge operational"}
		})
	} else {
		registry.Register("harrier", func(ctx context.Context) health.Check {
			return health.Check{Name: "harrier", Status: health.Degraded, Message: "Harrier not initialized (~/.harrier/data/ not writable)"}
		})
	}

	// Lefthook installation check
	registry.Register("lefthook", func(ctx context.Context) health.Check {
		start := time.Now()
		_, err := exec.LookPath("lefthook")
		if err != nil {
			return health.Check{Name: "lefthook", Status: health.Degraded, Message: "lefthook not installed (git hooks not active)", Duration: time.Since(start)}
		}
		return health.Check{Name: "lefthook", Status: health.Healthy, Message: "lefthook installed", Duration: time.Since(start)}
	})

	// Config syntax check
	registry.Register("config_syntax", func(ctx context.Context) health.Check {
		start := time.Now()
		globalPath := storage.SettingsPath()
		// #nosec G304 -- globalPath is the internal settings path, not external input
		if data, err := os.ReadFile(globalPath); err == nil {
			var raw json.RawMessage
			if err := json.Unmarshal(data, &raw); err != nil {
				return health.Check{Name: "config_syntax", Status: health.Unhealthy, Message: fmt.Sprintf("global settings.json parse error: %v", err), Duration: time.Since(start)}
			}
		}
		return health.Check{Name: "config_syntax", Status: health.Healthy, Message: "config files parse OK", Duration: time.Since(start)}
	})

	results := registry.Run(context.Background())
	var b strings.Builder
	b.WriteString("Health checks:\n")
	for _, check := range results {
		status := icons.CheckBold() + " "
		if check.Status == health.Unhealthy {
			status = icons.CloseThick() + " "
		} else if check.Status == health.Degraded {
			status = icons.Alert() + " "
		}
		b.WriteString(fmt.Sprintf("  %s %s: %s\n", status, check.Name, check.Message))
	}
	return strings.TrimRight(b.String(), "\n")
}

func providerCredentialHealthChecker(provider string) health.Checker {
	return func(ctx context.Context) health.Check {
		start := time.Now()
		providerID := diagnosticsProvider(ctx, provider)
		name := "api_key"
		label := "Provider"
		if providerID != "" {
			name = providerID + "_api_key"
			label = providerID
		}

		status := health.Unhealthy
		message := label + " credential not configured"
		if providerID != "" && graycodeconfig.HasStoredCredentialForProvider(ctx, providerID) {
			status = health.Healthy
			message = label + " credential configured"
		}
		checkedAt := time.Now()
		return health.Check{
			Name:        name,
			Status:      status,
			Message:     message,
			LastChecked: checkedAt,
			Duration:    checkedAt.Sub(start),
		}
	}
}

func diagnosticsProvider(ctx context.Context, provider string) string {
	provider = strings.TrimSpace(provider)
	if provider == "" || strings.EqualFold(provider, "auto") {
		provider = strings.TrimSpace(graycodeconfig.ActiveGateway(ctx))
		if provider == "" || strings.EqualFold(provider, "auto") {
			provider = strings.TrimSpace(graycodeconfig.EffectiveSelection(ctx, graycodeconfig.SelectionOptions{}).Provider)
		}
	}
	return graycodeconfig.ActiveProviderID(provider)
}

func settingsSummary(settings graycodeconfig.Settings) string {
	return configCommandSummary(settings)
}

func mcpConfigSummary(settings graycodeconfig.Settings) string {
	if len(settings.MCPServers) == 0 && len(mcpServers) == 0 {
		return "No MCP servers configured."
	}
	var b strings.Builder
	b.WriteString("MCP servers:\n")
	for _, cfg := range settings.MCPServers {
		name := cfg.Name
		if name == "" {
			name = cfg.Command
		}
		b.WriteString(fmt.Sprintf("  %s: %s %s\n", name, cfg.Command, strings.Join(cfg.Args, " ")))
	}
	for _, cmd := range mcpServers {
		b.WriteString("  cli: " + cmd + "\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

func sessionsSummary() string {
	entries, err := session.List()
	if err != nil || len(entries) == 0 {
		return "No saved sessions."
	}
	var b strings.Builder
	b.WriteString("Saved sessions:\n")
	for _, e := range entries {
		cwd := e.CWD
		if cwd == "" {
			cwd = "-"
		}
		b.WriteString(fmt.Sprintf("  %s  %s  %s  %s\n", e.ID, e.UpdatedAt.Format("2006-01-02 15:04"), cwd, e.Preview))
	}
	return strings.TrimRight(b.String(), "\n")
}

func builtInToolsSummary() string {
	essential := essentialTools()
	optional := optionalTools()
	var b strings.Builder
	b.WriteString(fmt.Sprintf("Built-in tools (%d total: %d essential, %d optional):\n", len(essential)+len(optional), len(essential), len(optional)))
	b.WriteString("  Essential (loaded at startup):\n")
	for _, t := range essential {
		b.WriteString(fmt.Sprintf("    %s - %s\n", t.Name(), t.Description()))
	}
	b.WriteString("  Optional (lazy-loaded):\n")
	for _, t := range optional {
		b.WriteString(fmt.Sprintf("    %s - %s\n", t.Name(), t.Description()))
	}
	b.WriteString("\nIntent bundles:\n")
	for _, summary := range tool.IntentBundleSummary() {
		b.WriteString("  " + summary + "\n")
	}
	return strings.TrimRight(b.String(), "\n")
}
