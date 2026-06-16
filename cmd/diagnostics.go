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

	"github.com/GrayCodeAI/eyrie/catalog"
	"github.com/GrayCodeAI/eyrie/credentials"
	eyrieruntime "github.com/GrayCodeAI/eyrie/runtime"
	"github.com/GrayCodeAI/eyrie/setup"
	hawkconfig "github.com/GrayCodeAI/hawk/internal/config"
	"github.com/GrayCodeAI/hawk/internal/intelligence/memory"
	"github.com/GrayCodeAI/hawk/internal/plugin"
	"github.com/GrayCodeAI/hawk/internal/resilience/health"
	"github.com/GrayCodeAI/hawk/internal/session"
	"github.com/GrayCodeAI/hawk/internal/ui/icons"
)

func doctorReport(settings hawkconfig.Settings) string {
	modelName, providerName := effectiveModelAndProvider(settings)
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
		if data, err := os.ReadFile(versionFile); err == nil {
			b.WriteString(fmt.Sprintf("  %s: %s\n", repo, strings.TrimSpace(string(data))))
		} else {
			b.WriteString(fmt.Sprintf("  %s: not checked out\n", repo))
		}
	}
	b.WriteString("\n" + hawkconfig.FormatEcosystemPanel(context.Background(), provider, modelName) + "\n")
	b.WriteString("\n" + hawkconfig.FormatCatalogHealth(hawkconfig.CatalogHealthReport(context.Background())) + "\n")
	b.WriteString("\n" + eyrieruntime.FormatPreflightReport(eyrieruntime.Preflight(context.Background())) + "\n")
	b.WriteString("\n" + credentials.FormatStorageReport(credentials.StorageReportFor(context.Background())) + "\n")
	if deployReport, err := hawkconfig.DeploymentStatusReport(context.Background(), modelName); err == nil {
		b.WriteString("\n" + deployReport + "\n")
	}
	_ = hawkconfig.MigrateProviderConfig()
	b.WriteString("\n" + envSummaryWithSelection(provider, modelName, false) + "\n")
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

	// Bundled skills
	bundledDir := plugin.BundledSkillsDir()
	if _, err := os.Stat(bundledDir); err == nil {
		entries, _ := os.ReadDir(bundledDir)
		b.WriteString(fmt.Sprintf("Bundled skills: %d extracted\n", len(entries)))
	} else {
		b.WriteString("Bundled skills: not yet extracted\n")
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

	b.WriteString("\n" + healthCheckReport(settings, provider) + "\n")
	return strings.TrimRight(b.String(), "\n")
}

func healthCheckReport(settings hawkconfig.Settings, provider string) string {
	registry := health.NewRegistry()

	ctx := context.Background()
	apiKeyEnv := primaryAPIKeyEnvForProvider(ctx, provider)
	apiKey := credentials.LookupSecret(ctx, apiKeyEnv)
	registry.Register("api_key", health.APIKeyChecker(provider, apiKey))

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
		home, _ := os.UserHomeDir()
		globalPath := filepath.Join(home, ".hawk", "settings.json")
		if data, err := os.ReadFile(globalPath); err == nil {
			var raw json.RawMessage
			if err := json.Unmarshal(data, &raw); err != nil {
				return health.Check{Name: "config_syntax", Status: health.Unhealthy, Message: fmt.Sprintf("global settings.json parse error: %v", err), Duration: time.Since(start)}
			}
		}
		if data, err := os.ReadFile(".hawk/settings.json"); err == nil {
			var raw json.RawMessage
			if err := json.Unmarshal(data, &raw); err != nil {
				return health.Check{Name: "config_syntax", Status: health.Unhealthy, Message: fmt.Sprintf("project settings.json parse error: %v", err), Duration: time.Since(start)}
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

func primaryAPIKeyEnvForProvider(ctx context.Context, provider string) string {
	provider = strings.TrimSpace(provider)
	if provider == "" || provider == "auto" {
		provider = strings.TrimSpace(hawkconfig.ActiveProvider(ctx))
	}
	if provider == "" {
		return ""
	}
	compiled, err := setup.LoadCompiledCatalog(ctx)
	if err != nil || compiled == nil {
		return ""
	}
	return catalog.PrimaryAPIKeyEnvForProvider(compiled, provider)
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
	tools := allTools()
	var b strings.Builder
	b.WriteString(fmt.Sprintf("Built-in tools (%d):\n", len(tools)))
	for _, t := range tools {
		b.WriteString(fmt.Sprintf("  %s - %s\n", t.Name(), t.Description()))
	}
	return strings.TrimRight(b.String(), "\n")
}
