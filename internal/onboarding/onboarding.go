package onboarding

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	hawkconfig "github.com/GrayCodeAI/hawk/internal/config"
	"github.com/mattn/go-runewidth"
	"golang.org/x/term"

	"github.com/GrayCodeAI/hawk/internal/ui/icons"
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
	fmt.Println(center(dim+"Built for developers — one machine, keychain credentials"+reset, 48))

	fmt.Println()
	fmt.Println(center(bold+"Quick start:"+reset, 12))
	fmt.Println(center(hawkC+"hawk"+reset+"                            interactive REPL (/config on first run)", 58))
	fmt.Println(center(hawkC+"hawk path"+reset+"                       check readiness", 49))
	fmt.Println(center(hawkC+"hawk"+reset+" -p \"explain this repo\"     one-shot mode", 49))
	fmt.Println(center(hawkC+"hawk"+reset+" -c                          continue last session", 54))
	fmt.Println(center(hawkC+"/config"+reset+"                         API key (keychain) + model", 54))

	fmt.Println()
	fmt.Println(center(hawkC+"? for shortcuts"+reset, 15))
	fmt.Println()
}

// NeedsSetup returns true only when hawk setup is explicitly requested.
// Normal hawk startup uses /config inside the TUI instead of blocking setup.
func NeedsSetup() bool {
	return false
}

// RunSetup runs the interactive first-run setup.
func RunSetup() error {
	reader := bufio.NewReader(os.Stdin)

	fmt.Println(teal + bold + "  Developer setup" + reset)
	fmt.Println()
	fmt.Println(dim + "  Keys are stored in " + hawkconfig.CredentialStoreName() + ", not .env or shell env." + reset)
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
		{"poolside", "POOLSIDE_API_KEY", "Poolside models"},
		{"ollama", "", "Local models (no API key needed)"},
	}

	for i, p := range providers {
		fmt.Printf("  %s%d%s) %s%-12s%s %s\n", teal, i+1, reset, bold, p.name, reset, dim+p.desc+reset)
	}
	fmt.Println()
	fmt.Print("  Enter number (1-7): ")

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
	case "7":
		idx = 6
	default:
		idx = 0 // default to anthropic
	}

	selected := providers[idx]
	fmt.Println()
	fmt.Printf("  Selected: %s%s%s\n", teal, selected.name, reset)

	// API key input
	if selected.envKey != "" && !hawkconfig.HasStoredCredentialForProvider(context.Background(), selected.name) {
		fmt.Println()
		fmt.Printf("  Enter your %s API key:\n", selected.name)
		fmt.Printf("  %s(Get one at the provider's website)%s\n", dim, reset)
		fmt.Print("  > ")

		apiKey, _ := reader.ReadString('\n')
		apiKey = strings.TrimSpace(apiKey)

		if apiKey == "" {
			fmt.Println(red + "  No API key entered. Run hawk and use /config to save a key securely." + reset)
			return fmt.Errorf("no API key")
		}

		// Validate key format before saving
		if warning, valid := validateAPIKey(selected.name, apiKey); !valid {
			fmt.Printf("  %s"+icons.Alert()+" %s%s\n", red, warning, reset)
			fmt.Println(red + "  API key not saved. Please check your key and try again." + reset)
			return fmt.Errorf("invalid API key")
		} else if warning != "" {
			fmt.Printf("  %s"+icons.Alert()+" %s (saving anyway)%s\n", dim, warning, reset)
		}

		ctx := context.Background()
		if err := hawkconfig.PersistAPIKey(ctx, selected.envKey, apiKey); err != nil {
			fmt.Printf("  %sWarning: couldn't save API key: %s%s\n", dim, err, reset)
			return err
		}

		if err := hawkconfig.SetActiveProvider(context.Background(), selected.name); err != nil {
			fmt.Printf("  %sWarning: couldn't save provider: %s%s\n", dim, err, reset)
		}

		fmt.Println()
		fmt.Printf("  %s"+icons.CheckBold()+" API key saved to %s%s\n", teal, hawkconfig.CredentialStoreName(), reset)
	} else if selected.name == "ollama" {
		_ = hawkconfig.SetActiveProvider(context.Background(), "ollama")
		fmt.Printf("  %s"+icons.CheckBold()+" Ollama selected (make sure ollama is running)%s\n", teal, reset)
	} else {
		_ = hawkconfig.SetActiveProvider(context.Background(), selected.name)
		fmt.Printf("  %s"+icons.CheckBold()+" Using %s (credential already in %s)%s\n", teal, selected.name, hawkconfig.CredentialStoreName(), reset)
	}

	// Security notes
	fmt.Println()
	fmt.Println(dim + "  ─────────────────────────────────────────" + reset)
	fmt.Println()
	fmt.Println("  " + bold + "Security notes:" + reset)
	fmt.Println("  1. hawk can make mistakes — always review changes")
	fmt.Println("  2. hawk will ask before running commands or writing files")
	fmt.Println("  3. Use /autonomy allow <tool> to auto-approve tools")
	fmt.Println()
	fmt.Println(dim + "  ─────────────────────────────────────────" + reset)
	fmt.Println()
	fmt.Print("  Press Enter to start... ")
	_, _ = reader.ReadString('\n')

	hawkconfig.DiscoverCatalogAfterSetup(context.Background(), os.Stdout)

	return nil
}

// validateAPIKey checks the key format for known providers.
// Returns (warning, isValid). A warning with isValid=true means the key is
// acceptable but may have an unusual format.
func validateAPIKey(provider, key string) (string, bool) {
	if len(key) <= 10 {
		return "API key seems too short", false
	}
	switch strings.ToLower(provider) {
	case "anthropic":
		if !strings.HasPrefix(key, "sk-ant-") {
			return "Anthropic keys typically start with 'sk-ant-'", false
		}
	case "openai":
		if !strings.HasPrefix(key, "sk-") {
			return "OpenAI keys typically start with 'sk-'", false
		}
	}
	return "", true
}
