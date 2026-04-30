package onboarding

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	hawkconfig "github.com/GrayCodeAI/hawk/config"
)

const (
	teal  = "\033[38;2;78;205;196m"
	dim   = "\033[2m"
	bold  = "\033[1m"
	red   = "\033[38;2;224;85;85m"
	reset = "\033[0m"
)

// Welcome prints the hawk welcome banner.
func Welcome(version string) {
	fmt.Println()
	fmt.Println(teal + bold + "  🦅 hawk" + reset + dim + " v" + version + reset)
	fmt.Println(dim + "  AI coding agent — reads, writes, and runs code in your terminal" + reset)
	fmt.Println(dim + "  Powered by eyrie • github.com/GrayCodeAI/hawk" + reset)
	fmt.Println()
}

// NeedsSetup returns true if first-run setup is needed.
func NeedsSetup() bool {
	settings := hawkconfig.LoadSettings()
	if settings.Provider != "" {
		return false
	}
	// Check if any API key is in env
	keys := []string{"ANTHROPIC_API_KEY", "OPENAI_API_KEY", "GEMINI_API_KEY",
		"OPENROUTER_API_KEY", "XAI_API_KEY", "GROQ_API_KEY"}
	for _, k := range keys {
		if os.Getenv(k) != "" {
			return false
		}
	}
	return true
}

// RunSetup runs the interactive first-run setup.
func RunSetup() error {
	reader := bufio.NewReader(os.Stdin)

	fmt.Println(teal + bold + "  First-time setup" + reset)
	fmt.Println()

	// Provider selection
	fmt.Println("  Choose your LLM provider:")
	fmt.Println()
	providers := []struct {
		name   string
		envKey string
		desc   string
	}{
		{"anthropic", "ANTHROPIC_API_KEY", "Claude (recommended)"},
		{"openai", "OPENAI_API_KEY", "GPT-4o, o1, o3"},
		{"gemini", "GEMINI_API_KEY", "Gemini 2.5"},
		{"openrouter", "OPENROUTER_API_KEY", "200+ models"},
		{"groq", "GROQ_API_KEY", "Fast inference"},
		{"ollama", "", "Local models (no API key needed)"},
	}

	for i, p := range providers {
		fmt.Printf("  %s%d%s) %s%-12s%s %s\n", teal, i+1, reset, bold, p.name, reset, dim+p.desc+reset)
	}
	fmt.Println()
	fmt.Print("  Enter number (1-6): ")

	input, _ := reader.ReadString('\n')
	input = strings.TrimSpace(input)

	idx := 0
	switch input {
	case "1":
		idx = 0
	case "2":
		idx = 1
	case "3":
		idx = 2
	case "4":
		idx = 3
	case "5":
		idx = 4
	case "6":
		idx = 5
	default:
		idx = 0 // default to anthropic
	}

	selected := providers[idx]
	fmt.Println()
	fmt.Printf("  Selected: %s%s%s\n", teal, selected.name, reset)

	// API key input
	if selected.envKey != "" && os.Getenv(selected.envKey) == "" {
		fmt.Println()
		fmt.Printf("  Enter your %s API key:\n", selected.name)
		fmt.Printf("  %s(Get one at the provider's website)%s\n", dim, reset)
		fmt.Print("  > ")

		apiKey, _ := reader.ReadString('\n')
		apiKey = strings.TrimSpace(apiKey)

		if apiKey == "" {
			fmt.Println(red + "  No API key entered. Set " + selected.envKey + " in your environment and try again." + reset)
			return fmt.Errorf("no API key")
		}

		// Set it for this session
		os.Setenv(selected.envKey, apiKey)

		// Save to settings
		settings := hawkconfig.LoadSettings()
		settings.Provider = selected.name
		settings.APIKey = apiKey
		if err := hawkconfig.SaveGlobal(settings); err != nil {
			fmt.Printf("  %sWarning: couldn't save settings: %s%s\n", dim, err, reset)
		}

		fmt.Println()
		fmt.Printf("  %s✓ API key saved to ~/.hawk/settings.json%s\n", teal, reset)
	} else if selected.name == "ollama" {
		settings := hawkconfig.LoadSettings()
		settings.Provider = "ollama"
		hawkconfig.SaveGlobal(settings)
		fmt.Printf("  %s✓ Ollama selected (make sure ollama is running)%s\n", teal, reset)
	} else {
		// Key already in env
		settings := hawkconfig.LoadSettings()
		settings.Provider = selected.name
		hawkconfig.SaveGlobal(settings)
		fmt.Printf("  %s✓ Using %s from environment%s\n", teal, selected.envKey, reset)
	}

	// Security notes
	fmt.Println()
	fmt.Println(dim + "  ─────────────────────────────────────────" + reset)
	fmt.Println()
	fmt.Println("  " + bold + "Security notes:" + reset)
	fmt.Println("  1. hawk can make mistakes — always review changes")
	fmt.Println("  2. hawk will ask before running commands or writing files")
	fmt.Println("  3. Use /permissions allow <tool> to auto-approve tools")
	fmt.Println()
	fmt.Println(dim + "  ─────────────────────────────────────────" + reset)
	fmt.Println()
	fmt.Print("  Press Enter to start... ")
	reader.ReadString('\n')

	return nil
}

// SaveAPIKeyToEnvFile appends the API key to ~/.hawk/env for future sessions.
func SaveAPIKeyToEnvFile(key, value string) {
	home, _ := os.UserHomeDir()
	path := home + "/.hawk/env"
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	defer f.Close()
	fmt.Fprintf(f, "export %s=%s\n", key, value)
}
