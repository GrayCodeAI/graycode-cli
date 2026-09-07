package config

import (
	"context"
	"fmt"
	"strings"

	"github.com/GrayCodeAI/graycode-cli/internal/intelligence/memory"
	"github.com/GrayCodeAI/graycode-cli/internal/theme"
	"github.com/GrayCodeAI/graycode-cli/internal/token"
)

// EcosystemReport is the structured view of the ecosystem panel.
type EcosystemReport struct {
	Eyre    EcosystemGraycodeRouter `json:"graycode-router"`
	Harrier EcosystemHarrier        `json:"harrier"`
	Shrike  EcosystemShrike         `json:"shrike"`
}

type EcosystemGraycodeRouter struct {
	CatalogExists bool   `json:"catalog_exists"`
	ModelCount    int    `json:"model_count,omitempty"`
	Ready         bool   `json:"ready"`
	Provider      string `json:"provider,omitempty"`
	RoutingSource string `json:"routing_source,omitempty"`
	RoutingStages int    `json:"routing_stages,omitempty"`
}

type EcosystemHarrier struct {
	Ready  bool   `json:"ready"`
	Status string `json:"status,omitempty"`
}

type EcosystemShrike struct {
	Embedded     bool `json:"embedded"`
	SampleTokens int  `json:"sample_tokens"`
}

// BuildEcosystemReport returns a structured ecosystem report.
func BuildEcosystemReport(ctx context.Context, provider, model string) EcosystemReport {
	var r EcosystemReport

	// graycode-router
	cat := CatalogHealthReport(ctx)
	r.Eyre.CatalogExists = cat.Exists
	r.Eyre.ModelCount = cat.Models
	pre := EnginePreflightReport(ctx)
	r.Eyre.Ready = pre.Ready
	if strings.TrimSpace(provider) != "" && provider != "auto" {
		r.Eyre.Provider = provider
	}
	if dep, err := EngineDeploymentSummary(ctx, model); err == nil {
		r.Eyre.RoutingSource = dep.RoutingSource
		r.Eyre.RoutingStages = dep.RoutingStages
	}

	// harrier
	bridge := memory.NewHarrierBridge()
	r.Harrier.Ready = bridge.Ready()
	if r.Harrier.Ready {
		first := strings.Split(memory.HarrierStatus(), "\n")[0]
		r.Harrier.Status = first
	}

	// shrike
	r.Shrike.Embedded = true
	r.Shrike.SampleTokens = token.CountTokensFast("graycode context compression pipeline")

	return r
}

// FormatEcosystemPanel summarizes graycode-router, harrier, and shrike integration for doctor and status output.
func FormatEcosystemPanel(ctx context.Context, provider, model string) string {
	var b strings.Builder
	b.WriteString(theme.Tint("Ecosystem (graycode-router · harrier · shrike):", theme.ReportInfo) + "\n")

	// graycode-router — LLM provider layer
	cat := CatalogHealthReport(ctx)
	graycodeRouterLine := "  " + theme.Tint("graycode-router:", theme.ReportMuted) + " "
	if cat.Exists {
		graycodeRouterLine += theme.Tint(fmt.Sprintf("catalog %d models", cat.Models), theme.ReportInfo)
	} else {
		graycodeRouterLine += theme.Tint("catalog missing (run graycode models refresh)", theme.ReportWarn)
	}
	pre := EnginePreflightReport(ctx)
	if pre.Ready {
		graycodeRouterLine += " · " + theme.Tint("locally ready", theme.ReportSuccess)
	} else {
		graycodeRouterLine += " · " + theme.Tint("setup incomplete", theme.ReportWarn)
	}
	if strings.TrimSpace(provider) != "" && provider != "auto" {
		graycodeRouterLine += " · " + theme.Tint("provider "+provider, theme.ReportInfo)
	}
	if dep, err := EngineDeploymentSummary(ctx, model); err == nil {
		if dep.RoutingStages > 0 {
			graycodeRouterLine += " · " + theme.Tint(fmt.Sprintf("routing %s (%d stages)", dep.RoutingSource, dep.RoutingStages), theme.ReportInfo)
		} else {
			graycodeRouterLine += " · " + theme.Tint("routing "+dep.RoutingSource, theme.ReportInfo)
		}
	}
	b.WriteString(graycodeRouterLine + "\n")

	// harrier — persistent memory graph
	bridge := memory.NewHarrierBridge()
	if bridge.Ready() {
		first := strings.Split(memory.HarrierStatus(), "\n")[0]
		b.WriteString("  " + theme.Tint("harrier:", theme.ReportMuted) + " " + theme.Tint(first, theme.ReportInfo) + " · " + theme.Tint("bridge ready", theme.ReportSuccess) + "\n")
	} else {
		b.WriteString("  " + theme.Tint("harrier:", theme.ReportMuted) + " " + theme.Tint("not initialized", theme.ReportWarn) + " · memory ops skipped (~/.harrier/data/)\n")
	}

	// shrike — token counting and context compression (always embedded)
	sample := token.CountTokensFast("graycode context compression pipeline")
	b.WriteString("  " + theme.Tint("shrike:", theme.ReportMuted) + " " + theme.Tint("embedded", theme.ReportInfo) + " · " + theme.Tint("token/compress pipeline OK", theme.ReportSuccess) + fmt.Sprintf(" (sample=%d tokens)", sample) + "\n")
	return strings.TrimRight(b.String(), "\n")
}
