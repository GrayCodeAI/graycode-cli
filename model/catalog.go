package model

// ModelInfo describes a known LLM model.
type ModelInfo struct {
	Name        string  `json:"name"`
	Provider    string  `json:"provider"`
	ContextSize int     `json:"context_size"`
	InputPrice  float64 `json:"input_price_per_million"`
	OutputPrice float64 `json:"output_price_per_million"`
	Description string  `json:"description,omitempty"`
	Recommended bool    `json:"recommended,omitempty"`
}

// Catalog is the registry of known models.
var Catalog = []ModelInfo{
	// Anthropic
	{Name: "claude-sonnet-4-20250514", Provider: "anthropic", ContextSize: 200_000, InputPrice: 3.0, OutputPrice: 15.0, Description: "Claude 4 Sonnet - balanced speed and quality", Recommended: true},
	{Name: "claude-opus-4-20250514", Provider: "anthropic", ContextSize: 200_000, InputPrice: 15.0, OutputPrice: 75.0, Description: "Claude 4 Opus - highest quality"},
	{Name: "claude-3-5-sonnet-20241022", Provider: "anthropic", ContextSize: 200_000, InputPrice: 3.0, OutputPrice: 15.0, Description: "Claude 3.5 Sonnet"},
	{Name: "claude-3-5-haiku-20241022", Provider: "anthropic", ContextSize: 200_000, InputPrice: 0.80, OutputPrice: 4.0, Description: "Claude 3.5 Haiku - fast and cheap"},
	{Name: "claude-3-opus-20240229", Provider: "anthropic", ContextSize: 200_000, InputPrice: 15.0, OutputPrice: 75.0, Description: "Claude 3 Opus"},
	{Name: "claude-3-haiku-20240307", Provider: "anthropic", ContextSize: 200_000, InputPrice: 0.25, OutputPrice: 1.25, Description: "Claude 3 Haiku"},

	// OpenAI
	{Name: "gpt-4o", Provider: "openai", ContextSize: 128_000, InputPrice: 2.50, OutputPrice: 10.0, Description: "GPT-4o - multimodal", Recommended: true},
	{Name: "gpt-4o-mini", Provider: "openai", ContextSize: 128_000, InputPrice: 0.15, OutputPrice: 0.60, Description: "GPT-4o Mini - fast and cheap"},
	{Name: "gpt-4-turbo-2024-04-09", Provider: "openai", ContextSize: 128_000, InputPrice: 10.0, OutputPrice: 30.0, Description: "GPT-4 Turbo"},
	{Name: "o1-preview", Provider: "openai", ContextSize: 128_000, InputPrice: 15.0, OutputPrice: 60.0, Description: "O1 Preview - reasoning"},
	{Name: "o1-mini", Provider: "openai", ContextSize: 128_000, InputPrice: 3.0, OutputPrice: 12.0, Description: "O1 Mini - reasoning"},

	// Gemini
	{Name: "gemini-2.5-flash", Provider: "gemini", ContextSize: 1_000_000, InputPrice: 0.15, OutputPrice: 0.60, Description: "Gemini 2.5 Flash - fast", Recommended: true},
	{Name: "gemini-2.5-pro", Provider: "gemini", ContextSize: 1_000_000, InputPrice: 1.25, OutputPrice: 10.0, Description: "Gemini 2.5 Pro - highest quality"},
	{Name: "gemini-2.0-flash", Provider: "gemini", ContextSize: 1_000_000, InputPrice: 0.10, OutputPrice: 0.40, Description: "Gemini 2.0 Flash"},
	{Name: "gemini-1.5-pro", Provider: "gemini", ContextSize: 2_000_000, InputPrice: 1.25, OutputPrice: 5.0, Description: "Gemini 1.5 Pro"},

	// OpenRouter
	{Name: "anthropic/claude-sonnet-4-20250514", Provider: "openrouter", ContextSize: 200_000, InputPrice: 3.0, OutputPrice: 15.0, Description: "Claude Sonnet via OpenRouter"},
	{Name: "openai/gpt-4o", Provider: "openrouter", ContextSize: 128_000, InputPrice: 2.50, OutputPrice: 10.0, Description: "GPT-4o via OpenRouter"},

	// Groq
	{Name: "llama-3.3-70b-versatile", Provider: "groq", ContextSize: 128_000, InputPrice: 0.20, OutputPrice: 0.20, Description: "Llama 3.3 70B via Groq"},
	{Name: "mixtral-8x7b-32768", Provider: "groq", ContextSize: 32_768, InputPrice: 0.20, OutputPrice: 0.20, Description: "Mixtral 8x7B via Groq"},

	// xAI
	{Name: "grok-3", Provider: "grok", ContextSize: 128_000, InputPrice: 3.0, OutputPrice: 15.0, Description: "Grok 3"},
	{Name: "grok-3-mini", Provider: "grok", ContextSize: 128_000, InputPrice: 0.50, OutputPrice: 2.0, Description: "Grok 3 Mini"},

	// Ollama (local)
	{Name: "llama3.2", Provider: "ollama", ContextSize: 128_000, InputPrice: 0, OutputPrice: 0, Description: "Llama 3.2 (local)"},
	{Name: "qwen2.5", Provider: "ollama", ContextSize: 128_000, InputPrice: 0, OutputPrice: 0, Description: "Qwen 2.5 (local)"},
	{Name: "codellama", Provider: "ollama", ContextSize: 16_000, InputPrice: 0, OutputPrice: 0, Description: "CodeLlama (local)"},
}

// Find looks up a model by name.
func Find(name string) (ModelInfo, bool) {
	for _, m := range Catalog {
		if m.Name == name {
			return m, true
		}
	}
	return ModelInfo{}, false
}

// ByProvider returns all models for a given provider.
func ByProvider(provider string) []ModelInfo {
	var out []ModelInfo
	for _, m := range Catalog {
		if m.Provider == provider {
			out = append(out, m)
		}
	}
	return out
}

// Recommended returns the recommended model for a provider.
func Recommended(provider string) (ModelInfo, bool) {
	for _, m := range Catalog {
		if m.Provider == provider && m.Recommended {
			return m, true
		}
	}
	return ModelInfo{}, false
}

// DefaultModel returns the default model for a provider.
func DefaultModel(provider string) string {
	if m, ok := Recommended(provider); ok {
		return m.Name
	}
	defaults := map[string]string{
		"anthropic":  "claude-sonnet-4-20250514",
		"openai":     "gpt-4o",
		"gemini":     "gemini-2.5-flash",
		"openrouter": "anthropic/claude-sonnet-4-20250514",
		"groq":       "llama-3.3-70b-versatile",
		"grok":       "grok-3",
		"ollama":     "llama3.2",
	}
	if m, ok := defaults[provider]; ok {
		return m
	}
	return "claude-sonnet-4-20250514"
}

// AllProviders returns all supported provider names.
func AllProviders() []string {
	seen := make(map[string]bool)
	var out []string
	for _, m := range Catalog {
		if !seen[m.Provider] {
			seen[m.Provider] = true
			out = append(out, m.Provider)
		}
	}
	return out
}
