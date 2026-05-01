package config

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

// LoadDotEnv loads environment variables from .env files.
// Checks in order: .env, .env.local (project), then ~/.hawk/.env (global).
// Does NOT override existing environment variables.
func LoadDotEnv() {
	// Project-level .env files
	loadEnvFile(".env")
	loadEnvFile(".env.local")

	// Global hawk .env
	home, err := os.UserHomeDir()
	if err == nil {
		loadEnvFile(filepath.Join(home, ".hawk", ".env"))
	}
}

// loadEnvFile reads a .env file and sets environment variables.
func loadEnvFile(path string) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Skip comments and empty lines
		if line == "" || line[0] == '#' {
			continue
		}

		// Parse KEY=VALUE
		eqIdx := strings.IndexByte(line, '=')
		if eqIdx < 0 {
			continue
		}

		key := strings.TrimSpace(line[:eqIdx])
		value := strings.TrimSpace(line[eqIdx+1:])

		// Remove surrounding quotes
		if len(value) >= 2 {
			if (value[0] == '"' && value[len(value)-1] == '"') ||
				(value[0] == '\'' && value[len(value)-1] == '\'') {
				value = value[1 : len(value)-1]
			}
		}

		// Don't override existing env vars
		if os.Getenv(key) == "" {
			os.Setenv(key, value)
		}
	}
}

// GetAPIKey returns the API key for a provider, checking multiple sources.
func GetAPIKey(provider string) string {
	envVars := providerKeyEnvVars(provider)
	for _, key := range envVars {
		if v := os.Getenv(key); v != "" {
			return v
		}
	}
	return ""
}

// providerKeyEnvVars returns the environment variable names for a provider's API key.
func providerKeyEnvVars(provider string) []string {
	switch strings.ToLower(provider) {
	case "anthropic":
		return []string{"ANTHROPIC_API_KEY", "CLAUDE_API_KEY"}
	case "openai":
		return []string{"OPENAI_API_KEY"}
	case "gemini", "google":
		return []string{"GEMINI_API_KEY", "GOOGLE_API_KEY"}
	case "openrouter":
		return []string{"OPENROUTER_API_KEY"}
	case "groq":
		return []string{"GROQ_API_KEY"}
	case "grok", "xai":
		return []string{"XAI_API_KEY", "GROK_API_KEY"}
	case "deepseek":
		return []string{"DEEPSEEK_API_KEY"}
	case "mistral":
		return []string{"MISTRAL_API_KEY"}
	case "bedrock":
		return []string{"AWS_ACCESS_KEY_ID"}
	case "vertex":
		return []string{"GOOGLE_APPLICATION_CREDENTIALS"}
	case "ollama":
		return []string{} // no key needed
	default:
		return []string{strings.ToUpper(provider) + "_API_KEY"}
	}
}

// ValidateAPIKey checks if an API key is set for the provider.
func ValidateAPIKey(provider string) (string, bool) {
	key := GetAPIKey(provider)
	return key, key != ""
}

// MaskAPIKey returns a masked version of an API key for display.
func MaskAPIKey(key string) string {
	if len(key) <= 8 {
		return "****"
	}
	return key[:4] + "..." + key[len(key)-4:]
}
