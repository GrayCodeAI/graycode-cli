package cmd

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"runtime/debug"
	"strings"
	"sync"
	"syscall"
	"time"

	hawkconfig "github.com/GrayCodeAI/hawk/internal/config"
	"github.com/GrayCodeAI/hawk/internal/storage"
)

// friendlyError translates a raw error into a user-friendly message with an
// actionable suggestion. It delegates to the shared classifyError in
// error_classify.go so the human-readable message and the exit code never
// disagree about what kind of failure occurred.
func friendlyError(err error) string {
	return friendlyErrorMessage(err)
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
