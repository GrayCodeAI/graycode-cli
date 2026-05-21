package config

import (
	"github.com/GrayCodeAI/hawk/internal/provider/routing"

	eycatalog "github.com/GrayCodeAI/eyrie/catalog"
)

const defaultPackProvider = "anthropic"

func packRole(provider string, tier eycatalog.ModelTier, temperature float64, maxTokens int, purpose string) ModelRole {
	return ModelRole{
		Provider:    provider,
		Model:       routing.PreferredModelForTier(provider, tier, ""),
		Temperature: temperature,
		MaxTokens:   maxTokens,
		Purpose:     purpose,
	}
}

func anthropicPackModels(haikuTier, sonnetTier, opusTier eycatalog.ModelTier) map[string]ModelRole {
	p := defaultPackProvider
	return map[string]ModelRole{
		"code":      packRole(p, sonnetTier, 0.2, 4096, "code generation and editing"),
		"chat":      packRole(p, sonnetTier, 0.7, 2048, "interactive conversation"),
		"summarize": packRole(p, haikuTier, 0.3, 1024, "summarization"),
		"review":    packRole(p, sonnetTier, 0.1, 4096, "code review"),
		"plan":      packRole(p, opusTier, 0.4, 8192, "complex planning and architecture"),
		"debug":     packRole(p, opusTier, 0.2, 4096, "debugging complex issues"),
	}
}
