package onboarding

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	hawkconfig "github.com/GrayCodeAI/hawk/config"
	"github.com/mattn/go-runewidth"
	"golang.org/x/term"
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
	// Vivid Orange #FF5E0E
	hawkC := "\033[38;2;255;94;14m"

	totalW := 80
	if w, _, err := term.GetSize(int(os.Stdout.Fd())); err == nil && w > 40 {
		totalW = w
	}

	center := func(s string, visLen int) string {
		pad := (totalW - visLen) / 2
		if pad < 0 {
			pad = 0
		}
		return strings.Repeat(" ", pad) + s
	}

	art := []string{
		"█████████    █████████    ███       ███  ███   █████████",
		"███    ███   ███    ███   ███       ███  ███  ███       ",
		"███    ███   ███    ███   ███       ███  ███ ███        ",
		"███    ███   ███    ███   ███   █   ███  ██████         ",
		"█████████    █████████    ███  ███  ███  ██████         ",
		"███    ███   ███    ███   ████ ███ ████  ███ ███        ",
		"███    ███   ███    ███   ████████████   ███  ███       ",
		"███    ███   ███    ███   █████   █████  ███   ███      ",
		"███    ███   ███    ███   ████     ████  ███    ███     ",
	}

	fmt.Println()
	for _, line := range art {
		w := runewidth.StringWidth(line)
		fmt.Println(center(hawkC+line+reset, w))
	}

	fmt.Println()
	verLine := fmt.Sprintf("v%s", version)
	fmt.Println(center(dim+verLine+reset, len(verLine)))

	fmt.Println()
	fmt.Println(center(bold+"Welcome to Hawk!"+reset, 16))

	fmt.Println()
	fmt.Println(center(bold+"Quick start:"+reset, 12))
	fmt.Println(center(hawkC+"hawk"+reset+" -p \"explain this repo\"     one-shot mode", 49))
	fmt.Println(center(hawkC+"hawk"+reset+"                            interactive REPL", 49))
	fmt.Println(center(hawkC+"hawk"+reset+" -c                          continue last session", 54))

	fmt.Println()
	fmt.Println(center(hawkC+"? for shortcuts"+reset, 15))
	fmt.Println()
}

// NeedsSetup returns true if first-run setup is needed.
func NeedsSetup() bool {
	// Load persisted env vars first
	_ = hawkconfig.LoadEnvFile()

	settings := hawkconfig.LoadSettings()
	if settings.Provider != "" {
		return false
	}
	// Check if any API key is in env (either from shell or ~/.hawk/env)
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
	// Load any previously saved env vars first
	_ = hawkconfig.LoadEnvFile()

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

		// Herm-style: set env var for this session, persist to ~/.hawk/env
		os.Setenv(selected.envKey, apiKey)
		_ = hawkconfig.SaveEnvFile(selected.envKey, apiKey)

		// Save provider preference only (not the key)
		settings := hawkconfig.LoadSettings()
		settings.Provider = selected.name
		if err := hawkconfig.SaveGlobal(settings); err != nil {
			fmt.Printf("  %sWarning: couldn't save settings: %s%s\n", dim, err, reset)
		}

		fmt.Println()
		fmt.Printf("  %s✓ API key saved to ~/.hawk/env (secure, 600 perms)%s\n", teal, reset)
	} else if selected.name == "ollama" {
		settings := hawkconfig.LoadSettings()
		settings.Provider = "ollama"
		hawkconfig.SaveGlobal(settings)
		fmt.Printf("  %s✓ Ollama selected (make sure ollama is running)%s\n", teal, reset)
	} else {
		// Key already in env — just save provider preference
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
