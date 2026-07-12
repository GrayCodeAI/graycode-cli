package onboarding

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/GrayCodeAI/eyrie/credentials"
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
	hawkconfig.PrepareCredentialDiscovery(context.Background())

	reader := bufio.NewReader(os.Stdin)

	fmt.Println(teal + bold + "  Developer setup" + reset)
	fmt.Println()
	fmt.Println(dim + "  Keys are stored in " + credentials.PlatformSecretStoreName() + ", not .env or shell env." + reset)
	fmt.Println()

	fmt.Println("  Choose your LLM provider:")
	fmt.Println()
	providers := setupProviderOptions()
	if len(providers) == 0 {
		return fmt.Errorf("eyrie returned no setup providers")
	}
	for i, provider := range providers {
		fmt.Printf("  %s%d%s) %s%s%s\n", teal, i+1, reset, bold, hawkconfig.GatewayDisplayName(provider), reset)
	}
	fmt.Println()
	fmt.Printf("  Enter number (1-%d): ", len(providers))

	input, _ := reader.ReadString('\n')
	selected, err := selectProvider(providers, input)
	if err != nil {
		return err
	}

	fmt.Println()
	fmt.Printf("  Selected: %s%s%s\n", teal, hawkconfig.GatewayDisplayName(selected), reset)

	ctx := context.Background()
	envKey := hawkconfig.SetupGatewayCredentialEnv(selected)
	if envKey != "" && !hawkconfig.HasStoredCredentialForProvider(ctx, selected) {
		fmt.Println()
		fmt.Printf("  Enter your %s API key:\n", hawkconfig.GatewayDisplayName(selected))
		fmt.Printf("  %s(Get one at the provider's website)%s\n", dim, reset)
		fmt.Print("  > ")

		apiKey, _ := reader.ReadString('\n')
		apiKey = strings.TrimSpace(apiKey)

		if apiKey == "" {
			fmt.Println(red + "  No API key entered. Run hawk and use /config to save a key securely." + reset)
			return fmt.Errorf("no API key")
		}

		inference, err := hawkconfig.CredentialInferenceForProvider(selected)
		if err != nil {
			return err
		}
		if err := hawkconfig.SaveCredential(ctx, inference, apiKey); err != nil {
			fmt.Printf("  %s"+icons.Alert()+" %s%s\n", red, hawkconfig.FormatConfigProviderError(selected, err), reset)
			return err
		}
		fmt.Println()
		fmt.Printf("  %s"+icons.CheckBold()+" API key saved to %s%s\n", teal, credentials.PlatformSecretStoreName(), reset)
	} else if envKey == "" {
		fmt.Printf("  %s"+icons.CheckBold()+" %s selected; no API key required%s\n", teal, hawkconfig.GatewayDisplayName(selected), reset)
	} else {
		fmt.Printf("  %s"+icons.CheckBold()+" Using %s (credential already in %s)%s\n", teal, hawkconfig.GatewayDisplayName(selected), credentials.PlatformSecretStoreName(), reset)
	}
	if err := hawkconfig.SetActiveProvider(ctx, selected); err != nil {
		return fmt.Errorf("save active provider: %w", err)
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

func setupProviderOptions() []string {
	providers := hawkconfig.AllSetupGateways()
	return append([]string(nil), providers...)
}

func selectProvider(providers []string, input string) (string, error) {
	selection, err := strconv.Atoi(strings.TrimSpace(input))
	if err != nil || selection < 1 || selection > len(providers) {
		return "", fmt.Errorf("provider selection must be between 1 and %d", len(providers))
	}
	return providers[selection-1], nil
}
