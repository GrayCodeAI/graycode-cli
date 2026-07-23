package config

import (
	"context"
	"fmt"
	"strings"

	"github.com/GrayCodeAI/hawk/internal/intelligence/memory"
	"github.com/GrayCodeAI/tok"
)

// EcosystemReport is the structured view of the ecosystem panel.
type EcosystemReport struct {
	Eyre  EcosystemEyrie `json:"eyrie"`
	Yaad  EcosystemYaad  `json:"yaad"`
	Tok   EcosystemTok   `json:"tok"`
}

type EcosystemEyrie struct {
	CatalogExists bool   `json:"catalog_exists"`
	ModelCount    int    `json:"model_count,omitempty"`
	Ready         bool   `json:"ready"`
	Provider      string `json:"provider,omitempty"`
	RoutingSource string `json:"routing_source,omitempty"`
	RoutingStages int    `json:"routing_stages,omitempty"`
}

type EcosystemYaad struct {
	Ready     bool   `json:"ready"`
	Status    string `json:"status,omitempty"`
}

type EcosystemTok struct {
	Embedded bool `json:"embedded"`
	SampleTokens int `json:"sample_tokens"`
}

// BuildEcosystemReport returns a structured ecosystem report.
func BuildEcosystemReport(ctx context.Context, provider, model string) EcosystemReport {
	var r EcosystemReport

	// eyrie
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

	// yaad
	bridge := memory.NewYaadBridge()
	r.Yaad.Ready = bridge.Ready()
	if r.Yaad.Ready {
		first := strings.Split(memory.YaadStatus(), "\n")[0]
		r.Yaad.Status = first
	}

	// tok
	r.Tok.Embedded = true
	r.Tok.SampleTokens = tok.EstimateTokens("hawk context compression pipeline")

	return r
}

// FormatEcosystemPanel summarizes eyrie, yaad, and tok integration for doctor and status output.
func FormatEcosystemPanel(ctx context.Context, provider, model string) string {
	var b strings.Builder
	b.WriteString("Ecosystem (eyrie · yaad · tok):\n")

	// eyrie — LLM provider layer
	cat := CatalogHealthReport(ctx)
	eyrieLine := "  eyrie: "
	if cat.Exists {
		eyrieLine += fmt.Sprintf("catalog %d models", cat.Models)
	} else {
		eyrieLine += "catalog missing (run hawk models refresh)"
	}
	pre := EnginePreflightReport(ctx)
	if pre.Ready {
		eyrieLine += " · locally ready"
	} else {
		eyrieLine += " · setup incomplete"
	}
	if strings.TrimSpace(provider) != "" && provider != "auto" {
		eyrieLine += fmt.Sprintf(" · provider %s", provider)
	}
	if dep, err := EngineDeploymentSummary(ctx, model); err == nil {
		if dep.RoutingStages > 0 {
			eyrieLine += fmt.Sprintf(" · routing %s (%d stages)", dep.RoutingSource, dep.RoutingStages)
		} else {
			eyrieLine += fmt.Sprintf(" · routing %s", dep.RoutingSource)
		}
	}
	b.WriteString(eyrieLine + "\n")

	// yaad — persistent memory graph
	bridge := memory.NewYaadBridge()
	if bridge.Ready() {
		first := strings.Split(memory.YaadStatus(), "\n")[0]
		b.WriteString("  yaad: " + first + " · bridge ready\n")
	} else {
		b.WriteString("  yaad: not initialized · memory ops skipped (~/.yaad/data/)\n")
	}

	// tok — token counting and context compression (always embedded)
	sample := tok.EstimateTokens("hawk context compression pipeline")
	b.WriteString(fmt.Sprintf("  tok: embedded · token/compress pipeline OK (sample=%d tokens)\n", sample))

	return strings.TrimRight(b.String(), "\n")
}
