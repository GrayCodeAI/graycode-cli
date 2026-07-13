package cmd

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"runtime/debug"
	"strings"
	"sync"
	"syscall"
	"time"

	hawkconfig "github.com/GrayCodeAI/hawk/internal/config"
	"github.com/GrayCodeAI/hawk/internal/storage"
)

// ─── friendlyError ────────────────────────────────────────────────────────────
// Translates raw API/system errors into user-friendly messages with actionable
// suggestions. Covers provider auth, network, rate limits, context overflow,
// model mismatches, file I/O, tool timeouts, config issues, MCP, and SSH.

var reRetryAfter = regexp.MustCompile(`(?i)retry[- ]?after[:\s]+(\d+)`)

func friendlyError(err error) string {
	if err == nil {
		return ""
	}
	msg := err.Error()
	low := strings.ToLower(msg)

	// ── Provider-specific API key errors ──────────────────────────────────
	providerKeys := []struct {
		patterns []string
		envVar   string
		provider string
	}{
		{[]string{"anthropic_api_key", "anthropic api key", "x-api-key"}, "ANTHROPIC_API_KEY", "Anthropic"},
		{[]string{"openai_api_key", "openai api key", "openai key"}, "OPENAI_API_KEY", "OpenAI"},
		{[]string{"gemini_api_key", "google_api_key", "gemini api key"}, "GEMINI_API_KEY", "Gemini"},
		{[]string{"openrouter_api_key", "openrouter api key"}, "OPENROUTER_API_KEY", "OpenRouter"},
		{[]string{"canopywave_api_key", "canopywave api key"}, "CANOPYWAVE_API_KEY", "CanopyWave"},
		{[]string{"zai_payg_api_key", "zai_api_key"}, "ZAI_API_KEY", "Z.AI"},
		{[]string{"zai_coding_api_key", "zai_coding_api_key"}, "ZAI_CODING_API_KEY", "Z.AI Coding Plan"},
		{[]string{"xai_api_key", "xai api key"}, "XAI_API_KEY", "xAI (Grok)"},
		{[]string{"opencodego_api_key", "opencodego api key"}, "OPENCODEGO_API_KEY", "OpenCodeGo"},
		{[]string{"moonshot_api_key", "moonshot api key"}, "MOONSHOT_API_KEY", "Kimi (Moonshot)"},
		{[]string{"xiaomi_mimo_payg_api_key", "xiaomi mimo payg"}, "XIAOMI_MIMO_PAYG_API_KEY", "Xiaomi (MiMo) Pay-as-you-go"},
		{[]string{"xiaomi_mimo_token_plan_api_key", "xiaomi mimo token plan"}, "XIAOMI_MIMO_TOKEN_PLAN_API_KEY", "Xiaomi (MiMo) Token Plan"},
		{[]string{"xiaomi_mimo_api_key", "xiaomi mimo api key"}, "XIAOMI_MIMO_PAYG_API_KEY", "Xiaomi (MiMo)"},
	}
	for _, pk := range providerKeys {
		for _, pat := range pk.patterns {
			if strings.Contains(low, pat) {
				return fmt.Sprintf("%s API key is missing or invalid. Run /config to save it in %s.", pk.provider, hawkconfig.CredentialStoreName())
			}
		}
	}

	// ── SSH connection failures (check early, before generic network/auth) ──
	if strings.Contains(low, "ssh") && (strings.Contains(low, "connection") || strings.Contains(low, "refused") ||
		strings.Contains(low, "timeout") || strings.Contains(low, "auth") || strings.Contains(low, "handshake") ||
		strings.Contains(low, "key exchange")) {
		return "SSH connection failed. Check your SSH configuration, keys, and that the remote host is reachable.\n  Try: ssh -vv <host> to diagnose."
	}

	// ── MCP server not responding (check early, before generic timeouts) ──
	if strings.Contains(low, "mcp") && (strings.Contains(low, "not responding") || strings.Contains(low, "connection") ||
		strings.Contains(low, "failed") || strings.Contains(low, "timeout") || strings.Contains(low, "refused")) {
		return "MCP server is not responding. Check that the server is running and accessible.\n  Use /mcp to see configured servers, or /doctor for diagnostics."
	}

	// ── Tool timeout (check early, before generic timeouts) ───────────────
	if strings.Contains(low, "tool timeout") || strings.Contains(low, "tool_timeout") ||
		(strings.Contains(low, "tool") && strings.Contains(low, "timed out")) {
		return "A tool execution timed out. The command may be taking too long.\n  Try breaking the task into smaller steps."
	}

	// ── Reasoning-only response (thinking tokens but no answer) ───────────
	if strings.Contains(low, "error_only_reasoning") ||
		strings.Contains(low, "reasoning tokens but no answer") ||
		strings.Contains(low, "reasoning but no answer") {
		return "The model produced internal reasoning but no reply.\n" +
			"  This often happens with reasoning models on OpenCode Go / MiniMax when the provider drops the answer after thinking.\n" +
			"  Try /model to switch model, or pick a non-reasoning model for simple chat."
	}

	// ── Rate limiting (429) ───────────────────────────────────────────────
	if strings.Contains(low, "429") || strings.Contains(low, "rate limit") || strings.Contains(low, "rate_limit") || strings.Contains(low, "too many requests") {
		base := "Rate limited by the API provider."
		if match := reRetryAfter.FindStringSubmatch(msg); len(match) > 1 {
			base += fmt.Sprintf(" Retry after %s seconds.", match[1])
		}
		base += " Wait a moment and try again, or switch providers with /config."
		return base
	}

	// ── Authentication / authorization ────────────────────────────────────
	if strings.Contains(low, "401") || strings.Contains(low, "unauthorized") || strings.Contains(low, "invalid api key") || strings.Contains(low, "invalid_api_key") || strings.Contains(low, "authentication") {
		return "Authentication failed. Your API key may be invalid or expired.\n  Check with /env, or update it with /config."
	}
	if strings.Contains(low, "403") || strings.Contains(low, "forbidden") || strings.Contains(low, "access denied") {
		return "Access denied by the API provider. Verify your API key has the required permissions."
	}

	// ── Provider billing / credits (OpenRouter free tier, etc.) ───────────
	if strings.Contains(low, "requires more credits") || strings.Contains(low, "can only afford") ||
		strings.Contains(low, "insufficient credits") || strings.Contains(low, "insufficient balance") ||
		strings.Contains(low, "payment required") || strings.Contains(low, "out of credits") {
		return "Insufficient provider credits for this request.\n  Add credits at your provider dashboard, switch to a cheaper model with /model, or try again with a shorter prompt."
	}

	// ── Context too long / token limit ────────────────────────────────────
	if strings.Contains(low, "context length") || strings.Contains(low, "context_length") ||
		strings.Contains(low, "token limit") || strings.Contains(low, "too many tokens") ||
		strings.Contains(low, "maximum context") ||
		strings.Contains(low, "max_tokens exceeded") || strings.Contains(low, "max tokens exceeded") ||
		strings.Contains(low, "context window") || strings.Contains(low, "prompt is too long") {
		return "The conversation exceeds the model's context window.\n  Use /compact to summarize and free up space, or start a new session."
	}

	// ── Invalid model name ────────────────────────────────────────────────
	if strings.Contains(low, "model not found") || strings.Contains(low, "model_not_found") ||
		strings.Contains(low, "unknown model") || strings.Contains(low, "invalid model") ||
		strings.Contains(low, "does not exist") || (strings.Contains(low, "404") && strings.Contains(low, "model")) {
		ex1, ex2 := hawkconfig.ExampleModelHints()
		return fmt.Sprintf(
			"Model not found. Check your model name with /model.\n  Examples from the eyrie catalog: %s, %s\n  Use /models to list all models, or /config to change provider.",
			ex1, ex2,
		)
	}

	// ── Network unreachable / connection refused / DNS ─────────────────────
	if strings.Contains(low, "network is unreachable") || strings.Contains(low, "network unreachable") {
		return "Network is unreachable. Check that you have an active internet connection."
	}
	if strings.Contains(low, "connection refused") {
		return "Connection refused. The API endpoint may be down, or a local proxy/firewall is blocking the connection.\n  If using Ollama, make sure it is running (ollama serve)."
	}
	if strings.Contains(low, "no such host") || strings.Contains(low, "dns") ||
		strings.Contains(low, "lookup") && strings.Contains(low, "no such host") {
		return "DNS resolution failed. Check your internet connection and DNS settings."
	}
	if strings.Contains(low, "connection reset") || strings.Contains(low, "broken pipe") ||
		strings.Contains(low, "eof") && (strings.Contains(low, "unexpected") || strings.Contains(low, "connection")) {
		return "Connection was reset by the server. This may be a transient issue -- try again."
	}

	// ── HTTP status codes (generic) ───────────────────────────────────────
	if strings.Contains(low, "404") || strings.Contains(low, "not found") {
		return "Endpoint or resource not found. Check your model with /model or provider with /config."
	}
	if strings.Contains(low, "500") || strings.Contains(low, "internal server error") {
		return "The API provider returned a server error (500). Try again shortly."
	}
	if strings.Contains(low, "502") || strings.Contains(low, "bad gateway") {
		return "The API provider is temporarily unavailable (502). Try again shortly."
	}
	if strings.Contains(low, "503") || strings.Contains(low, "service unavailable") {
		return "The API provider is temporarily unavailable (503). Try again shortly."
	}
	if strings.Contains(low, "504") || strings.Contains(low, "gateway timeout") {
		return "The API provider timed out (504). The request may have been too large -- try /compact."
	}

	// ── Timeouts ──────────────────────────────────────────────────────────
	if strings.Contains(low, "timeout") || strings.Contains(low, "deadline exceeded") ||
		strings.Contains(low, "context canceled") {
		return "Request timed out. Check your connection and try again, or use /compact to reduce context size."
	}

	// ── Permission denied on file operations ──────────────────────────────
	if strings.Contains(low, "permission denied") {
		return "Permission denied. Check file/directory permissions.\n  You may need to adjust permissions or run from a writable directory."
	}

	// ── Disk full ─────────────────────────────────────────────────────────
	if strings.Contains(low, "no space left") || strings.Contains(low, "disk full") ||
		strings.Contains(low, "not enough space") || strings.Contains(low, "disk quota") {
		return "Disk is full or quota exceeded. Free up space and try again.\n  Check Hawk's user state sessions directory for old sessions you can remove."
	}

	// ── Invalid JSON in config/settings ───────────────────────────────────
	if (strings.Contains(low, "json") || strings.Contains(low, "unmarshal") || strings.Contains(low, "syntax error")) &&
		(strings.Contains(low, "settings") || strings.Contains(low, "config") || strings.Contains(low, "parse") || strings.Contains(low, "invalid character")) {
		return "Invalid JSON in configuration. Check your Hawk settings.json files for syntax errors.\n  Tip: use a JSON linter to find the issue."
	}

	// ── TLS / certificate errors ──────────────────────────────────────────
	if strings.Contains(low, "certificate") || strings.Contains(low, "tls") || strings.Contains(low, "x509") {
		return "TLS/certificate error. This may be caused by a corporate proxy, expired certificate, or network issue.\n  If behind a proxy, you may need to configure custom CA certificates."
	}

	// ── Fallback ──────────────────────────────────────────────────────────
	return msg
}

// ─── panicRecovery ────────────────────────────────────────────────────────────
// Catches panics, saves the current session state, logs the stack trace to
// Hawk's user state crash log, and exits with a user-friendly message.

//lint:ignore U1000 Infrastructure wired in main.go for production builds
func panicRecovery(saveFn func()) {
	if r := recover(); r != nil {
		stack := string(debug.Stack())

		// Attempt to save session
		if saveFn != nil {
			func() {
				defer func() { _ = recover() }() // don't let save panic again
				saveFn()
			}()
		}

		// Log to crash file
		crashDir := storage.StateDir()
		if crashDir != "" {
			_ = os.MkdirAll(crashDir, 0o750)
			crashLog := filepath.Join(crashDir, "crash.log")

			entry := fmt.Sprintf(
				"─── CRASH %s ───\npanic: %v\n\n%s\n\n",
				time.Now().Format(time.RFC3339),
				r,
				stack,
			)

			f, err := os.OpenFile(crashLog, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600) // #nosec G304 -- crashLog is an internal, statically-derived log path
			if err == nil {
				_, _ = f.WriteString(entry)
				_ = f.Close()
			}
		}

		// Print user-friendly message
		_, _ = fmt.Fprintf(os.Stderr, "\nhawk encountered an unexpected error and needs to exit.\n")
		_, _ = fmt.Fprintf(os.Stderr, "Your session has been saved.\n")
		_, _ = fmt.Fprintf(os.Stderr, "Details logged to %s\n", filepath.Join(storage.StateDir(), "crash.log"))
		_, _ = fmt.Fprintf(os.Stderr, "Please report this at: https://github.com/GrayCodeAI/hawk/issues\n\n")
		_, _ = fmt.Fprintf(os.Stderr, "panic: %v\n", r)
		os.Exit(1) // os.Exit intentional: panic recovery, defer already unwound
	}
}

// ─── signalHandler ────────────────────────────────────────────────────────────
// Handles SIGTERM, SIGINT, and SIGHUP gracefully. Calls the provided save
// function before exiting to ensure the current session is persisted.

//lint:ignore U1000 Infrastructure wired in main.go for production builds
func signalHandler(saveFn func()) {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)

	go func() {
		sig := <-sigCh
		_, _ = fmt.Fprintf(os.Stderr, "\nReceived %v, saving session...\n", sig)

		if saveFn != nil {
			// Give save a bounded amount of time
			done := make(chan struct{})
			go func() {
				defer func() {
					_ = recover() // don't let save panic crash the handler
					close(done)
				}()
				saveFn()
			}()

			select {
			case <-done:
				// saved successfully
			case <-time.After(5 * time.Second):
				_, _ = fmt.Fprintf(os.Stderr, "Save timed out, exiting.\n")
			}
		}

		_, _ = fmt.Fprintf(os.Stderr, "Goodbye.\n")
		os.Exit(0) // os.Exit intentional: signal handler, must terminate process
	}()
}

// ─── errorLogger ──────────────────────────────────────────────────────────────
// Writes errors to Hawk's user state error log with timestamps. Thread-safe.

type errorLoggerT struct {
	mu   sync.Mutex
	path string
}

// LogError writes a timestamped error entry to the Hawk error log.
func (l *errorLoggerT) LogError(context string, err error) {
	if l == nil || err == nil {
		return
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	entry := fmt.Sprintf(
		"[%s] %s: %s\n",
		time.Now().Format(time.RFC3339),
		context,
		err.Error(),
	)

	f, ferr := os.OpenFile(l.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if ferr != nil {
		return
	}
	defer func() { _ = f.Close() }()
	_, _ = f.WriteString(entry)
}

// LogErrorf writes a formatted, timestamped error entry to the Hawk error log.
func (l *errorLoggerT) LogErrorf(format string, args ...interface{}) {
	if l == nil {
		return
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	entry := fmt.Sprintf(
		"[%s] %s\n",
		time.Now().Format(time.RFC3339),
		fmt.Sprintf(format, args...),
	)

	f, ferr := os.OpenFile(l.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if ferr != nil {
		return
	}
	defer func() { _ = f.Close() }()
	_, _ = f.WriteString(entry)
}

// ─── validateStartup ─────────────────────────────────────────────────────────
// Checks essential prerequisites before starting a session:
//   - API key is set for the configured provider
//   - Network is reachable (quick DNS check)
//   - Sessions directory is writable

// StartupWarning represents a non-fatal startup issue.
type StartupWarning struct {
	Check   string
	Message string
}

func (w StartupWarning) String() string {
	return fmt.Sprintf("[%s] %s", w.Check, w.Message)
}

func validateStartup(settings hawkconfig.Settings) []StartupWarning {
	var warnings []StartupWarning

	// 1. Check API key for configured provider
	providerName := strings.TrimSpace(settings.Provider)
	if providerName == "" {
		providerName = strings.TrimSpace(hawkconfig.ActiveProvider(context.Background()))
	}
	if providerName != "" && providerName != "ollama" {
		envKey := hawkconfig.ProviderAPIKeyEnv(providerName)
		if envKey != "" && os.Getenv(envKey) == "" {
			warnings = append(warnings, StartupWarning{
				Check:   "api_key",
				Message: fmt.Sprintf("No API key found for %s. Set %s in your environment or run /config.", providerName, envKey),
			})
		}
	}

	// 2. Quick network reachability check (DNS lookup, no full HTTP request)
	if providerName != "" && providerName != "ollama" {
		host := providerDNSHost(providerName)
		if host != "" {
			if _, err := net.LookupHost(host); err != nil {
				warnings = append(warnings, StartupWarning{
					Check:   "network",
					Message: fmt.Sprintf("Cannot resolve %s. Check your internet connection.", host),
				})
			}
		}
	}

	// 3. Check sessions directory is writable
	sessDir := storage.SessionsDir()
	if err := os.MkdirAll(sessDir, 0o750); err != nil {
		warnings = append(warnings, StartupWarning{
			Check:   "sessions_dir",
			Message: fmt.Sprintf("Cannot create sessions directory %s: %v", sessDir, err),
		})
	} else {
		// Try writing a temp file to verify writability
		tmpPath := filepath.Join(sessDir, ".write_test")
		if err := os.WriteFile(tmpPath, []byte("ok"), 0o600); err != nil {
			warnings = append(warnings, StartupWarning{
				Check:   "sessions_dir",
				Message: fmt.Sprintf("Sessions directory %s is not writable: %v", sessDir, err),
			})
		} else {
			_ = os.Remove(tmpPath)
		}
	}

	return warnings
}

// providerDNSHost returns a hostname to check DNS resolution for a provider.
func providerDNSHost(provider string) string {
	switch strings.ToLower(provider) {
	case "anthropic":
		return "api.anthropic.com"
	case "openai":
		return "api.openai.com"
	case "gemini", "google":
		return "generativelanguage.googleapis.com"
	case "openrouter":
		return "openrouter.ai"
	case "grok", "xai":
		return "api.x.ai"
	case "canopywave":
		return "inference.canopywave.io"
	case "zai_payg", "zai_coding":
		return "api.z.ai"
	case "kimi", "moonshotai":
		return "api.moonshot.ai"
	case "xiaomi_mimo", "xiaomi_mimo_payg":
		return "api.xiaomimimo.com"
	case "xiaomi_mimo_token_plan":
		return "token-plan-*.xiaomimimo.com"
	default:
		return ""
	}
}
