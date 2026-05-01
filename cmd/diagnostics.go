package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	hawkconfig "github.com/GrayCodeAI/hawk/config"
	"github.com/GrayCodeAI/hawk/health"
	"github.com/GrayCodeAI/hawk/session"
)

func doctorReport(settings hawkconfig.Settings) string {
	provider, modelName := effectiveModelAndProvider(settings)
	if provider == "" {
		provider = "auto"
	}
	if modelName == "" {
		modelName = "default"
	}

	cwd, _ := os.Getwd()
	var b strings.Builder
	b.WriteString("Hawk doctor\n")
	b.WriteString(fmt.Sprintf("Version: %s\n", version))
	b.WriteString(fmt.Sprintf("Directory: %s\n", cwd))
	b.WriteString(fmt.Sprintf("Provider: %s\n", provider))
	b.WriteString(fmt.Sprintf("Model: %s\n", modelName))
	b.WriteString("\n" + envSummary(provider, modelName) + "\n")
	b.WriteString("\nGit:\n")
	if branch := branchSummary(); branch != "" {
		for _, line := range strings.Split(branch, "\n") {
			b.WriteString("  " + line + "\n")
		}
	}
	if hawkconfig.LoadHawkMD() != "" {
		b.WriteString("\nProject instructions: found\n")
	} else {
		b.WriteString("\nProject instructions: not found\n")
	}
	b.WriteString(fmt.Sprintf("Configured MCP servers: %d\n", len(settings.MCPServers)+len(mcpServers)))
	b.WriteString(fmt.Sprintf("Built-in tools: %d\n", len(baseTools())))
	b.WriteString("\n" + healthCheckReport(settings, provider) + "\n")
	return strings.TrimRight(b.String(), "\n")
}

func healthCheckReport(settings hawkconfig.Settings, provider string) string {
	registry := health.NewRegistry()

	// API key check
	apiKey := ""
	switch provider {
	case "anthropic":
		apiKey = os.Getenv("ANTHROPIC_API_KEY")
	case "openai":
		apiKey = os.Getenv("OPENAI_API_KEY")
	case "google":
		apiKey = os.Getenv("GOOGLE_API_KEY")
	case "openrouter":
		apiKey = os.Getenv("OPENROUTER_API_KEY")
	case "grok":
		apiKey = os.Getenv("XAI_API_KEY")
	case "canopywave":
		apiKey = os.Getenv("CANOPYWAVE_API_KEY")
	case "opencodego":
		apiKey = os.Getenv("OPENCODEGO_API_KEY")
	}
	registry.Register("api_key", health.APIKeyChecker(provider, apiKey))

	// Settings validation
	registry.Register("config", func(ctx context.Context) health.Check {
		result := hawkconfig.ValidateSettings(settings)
		if result.Valid {
			return health.Check{Name: "config", Status: health.Healthy, Message: "Configuration valid"}
		}
		return health.Check{Name: "config", Status: health.Degraded, Message: result.Error()}
	})

	results := registry.Run(context.Background())
	var b strings.Builder
	b.WriteString("Health checks:\n")
	for _, check := range results {
		status := "✓"
		if check.Status == health.Unhealthy {
			status = "✗"
		} else if check.Status == health.Degraded {
			status = "!"
		}
		b.WriteString(fmt.Sprintf("  %s %s: %s\n", status, check.Name, check.Message))
	}
	return strings.TrimRight(b.String(), "\n")
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
	tools := baseTools()
	var b strings.Builder
	b.WriteString(fmt.Sprintf("Built-in tools (%d):\n", len(tools)))
	for _, t := range tools {
		b.WriteString(fmt.Sprintf("  %s - %s\n", t.Name(), t.Description()))
	}
	return strings.TrimRight(b.String(), "\n")
}
