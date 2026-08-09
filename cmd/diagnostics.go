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

	hawkconfig "github.com/GrayCodeAI/hawk/internal/config"
	"github.com/GrayCodeAI/hawk/internal/intelligence/memory"
	"github.com/GrayCodeAI/hawk/internal/resilience/health"
	"github.com/GrayCodeAI/hawk/internal/session"
	"github.com/GrayCodeAI/hawk/internal/storage"
	"github.com/GrayCodeAI/hawk/internal/tool"
	"github.com/GrayCodeAI/hawk/internal/ui/icons"
)

func doctorReport(settings hawkconfig.Settings) string {
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
	b.WriteString("Hawk doctor\n")
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
	for _, repo := range []string{"eyrie", "yaad", "tok", "sight", "inspect", "trace"} {
		versionFile := filepath.Join("external", repo, "VERSION")
		// #nosec G304 -- versionFile built from a fixed internal repo-relative list
		if data, err := os.ReadFile(versionFile); err == nil {
			b.WriteString(fmt.Sprintf("  %s: %s\n", repo, strings.TrimSpace(string(data))))
		} else {
			b.WriteString(fmt.Sprintf("  %s: not checked out\n", repo))
		}
	}
	b.WriteString("\n" + hawkconfig.FormatEcosystemPanel(context.Background(), providerName, modelName) + "\n")
	b.WriteString("\n" + hawkconfig.FormatCatalogHealth(hawkconfig.CatalogHealthReport(context.Background())) + "\n")
	preflight := hawkconfig.EnginePreflightReportWithSettings(context.Background(), settings, hawkconfig.EnginePreflightOptions{})
	b.WriteString("\n" + hawkconfig.FormatEnginePreflight(preflight) + "\n")
	b.WriteString("\n" + hawkconfig.CredentialStorageStatus(context.Background()).Formatted + "\n")
	if deployReport, err := hawkconfig.DeploymentStatusReportWithSettings(context.Background(), settings, modelName); err == nil {
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
	if md := hawkconfig.LoadAgentsMD(); md != "" {
		b.WriteString("\nProject instructions: found\n")
	} else {
		b.WriteString("\nProject instructions: not found (consider creating AGENTS.md)\n")
	}

	// Installed skills (hawk ships none; skills come from user/marketplace installs)
	skillsDir := filepath.Join(storage.StateDir(), "skills")
	if entries, err := os.ReadDir(skillsDir); err == nil && len(entries) > 0 {
		b.WriteString(fmt.Sprintf("Installed skills: %d\n", len(entries)))
	} else {
		b.WriteString("Installed skills: none (install with `hawk skills install`)\n")
	}

	b.WriteString(fmt.Sprintf("Configured MCP servers: %d\n", len(settings.MCPServers)+len(mcpServers)))
	b.WriteString(fmt.Sprintf("Built-in tools: %d\n", len(allTools())))

	// Session recovery status
	recoveryCandidates := session.ScanForRecovery()
	if len(recoveryCandidates) > 0 {
		b.WriteString(fmt.Sprintf("\nInterrupted sessions: %d (run hawk recover)\n", len(recoveryCandidates)))
	} else {
		b.WriteString("\nInterrupted sessions: none\n")
	}

	b.WriteString("\n" + healthCheckReport(settings, providerName) + "\n")
	return strings.TrimRight(b.String(), "\n")
}

func healthCheckReport(settings hawkconfig.Settings, provider string) string {
	registry := health.NewRegistry()

	registry.Register("api_key", providerCredentialHealthChecker(provider))

	// Settings validation
	registry.Register("config", func(ctx context.Context) health.Check {
		result := hawkconfig.ValidateSettings(settings)
		if result.Valid {
			return health.Check{Name: "config", Status: health.Healthy, Message: "Configuration valid"}
		}
		return health.Check{Name: "config", Status: health.Degraded, Message: result.Error()}
	})

	// Yaad memory bridge check
	bridge := memory.NewYaadBridge()
	if bridge.Ready() {
		registry.Register("yaad", func(ctx context.Context) health.Check {
			_, _, err := bridge.SearchByType("convention", 1)
			if err != nil {
				return health.Check{Name: "yaad", Status: health.Unhealthy, Message: "Yaad bridge initialized but query failed"}
			}
			return health.Check{Name: "yaad", Status: health.Healthy, Message: "Yaad memory bridge operational"}
		})
	} else {
		registry.Register("yaad", func(ctx context.Context) health.Check {
			return health.Check{Name: "yaad", Status: health.Degraded, Message: "Yaad not initialized (~/.yaad/data/ not writable)"}
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
		if providerID != "" && hawkconfig.HasStoredCredentialForProvider(ctx, providerID) {
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
		provider = strings.TrimSpace(hawkconfig.ActiveGateway(ctx))
		if provider == "" || strings.EqualFold(provider, "auto") {
			provider = strings.TrimSpace(hawkconfig.EffectiveSelection(ctx, hawkconfig.SelectionOptions{}).Provider)
		}
	}
	return hawkconfig.ActiveProviderID(provider)
}

func settingsSummary(settings hawkconfig.Settings) string {
	return configCommandSummary(settings)
}

func mcpConfigSummary(settings hawkconfig.Settings) string {
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
