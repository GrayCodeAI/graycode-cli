package cmd

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	graycodeconfig "github.com/GrayCodeAI/graycode-cli/internal/config"
	"github.com/GrayCodeAI/graycode-cli/internal/storage"
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
// Graycode's user state crash log, and exits with a user-friendly message.

// panicSaveFn is set by runChat to a closure that persists the active session
// and stops the container sandbox. panicRecovery invokes it on an unexpected
// panic so a crash saves as much work as possible and never leaves a zombie
// Docker container running. It is nil outside the chat/TUI path (print mode
// persists via its own defer).
var panicSaveFn func()

// RunWithPanicRecovery executes fn with the process-level panic recovery
// installed. An unexpected panic in the main execution path is caught, the
// panicSaveFn (when set by runChat) is invoked to persist session state and
// stop the container, the stack is written to the crash log, and the process
// exits with a user-friendly message instead of a raw stack trace. Outside the
// TUI, sessions are persisted incrementally, so no saveFn loses at most the
// in-flight message.
func RunWithPanicRecovery(fn func() error) (err error) {
	defer panicRecovery(panicSaveFn)
	return fn()
}

func panicRecovery(saveFn func()) {
	if r := recover(); r != nil {
		stack := string(debug.Stack())

		// Generate a short, unique error ID for support reference.
		// Format: graycode-YYMMDD-<6 hex chars from stack hash>
		errorID := generateErrorID(stack)

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
				"─── CRASH %s [%s] ───\npanic: %v\n\n%s\n\n",
				time.Now().Format(time.RFC3339),
				errorID,
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
		_, _ = fmt.Fprintf(os.Stderr, "\ngraycode encountered an unexpected error and needs to exit.\n")
		if saveFn != nil {
			_, _ = fmt.Fprintf(os.Stderr, "Your session has been saved.\n")
		} else {
			_, _ = fmt.Fprintf(os.Stderr, "Session messages are persisted incrementally; the in-flight message may be lost.\n")
		}
		_, _ = fmt.Fprintf(os.Stderr, "Details logged to %s\n", filepath.Join(storage.StateDir(), "crash.log"))
		_, _ = fmt.Fprintf(os.Stderr, "Please report this at: https://github.com/GrayCodeAI/graycode-cli/issues\n")
		_, _ = fmt.Fprintf(os.Stderr, "Include this error ID: %s\n\n", errorID)
		_, _ = fmt.Fprintf(os.Stderr, "panic: %v\n", r)
		os.Exit(1) // os.Exit intentional: panic recovery, defer already unwound
	}
}

// generateErrorID creates a short, unique error ID from the stack trace.
// Format: graycode-YYMMDD-<6 hex chars> — enough to correlate with crash.log
// entries without requiring a random source.
func generateErrorID(stack string) string {
	// Simple hash of the stack trace for uniqueness.
	var hash uint32 = 5381
	for i := 0; i < len(stack) && i < 2000; i++ {
		hash = ((hash << 5) + hash) ^ uint32(stack[i])
	}
	return fmt.Sprintf("graycode-%s-%06x", time.Now().Format("060102"), hash&0xFFFFFF)
}

// ─── errorLogger ──────────────────────────────────────────────────────────────
// Writes errors to Graycode's user state error log with timestamps. Thread-safe.

type errorLoggerT struct {
	mu   sync.Mutex
	path string
}

// LogError writes a timestamped error entry to the Graycode error log.
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

// LogErrorf writes a formatted, timestamped error entry to the Graycode error log.
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

func validateStartup(settings graycodeconfig.Settings) []StartupWarning {
	var warnings []StartupWarning

	// 1. Check API key for configured provider.
	// Graycode reads credentials from the OS secret store (macOS Keychain /
	// Linux Keyring), not just env vars — so we must check there too.
	providerName := strings.TrimSpace(settings.Provider)
	if providerName == "" {
		providerName = strings.TrimSpace(graycodeconfig.ActiveProvider(context.Background()))
	}
	if providerName != "" && providerName != "ollama" {
		hasEnv := false
		if envKey := graycodeconfig.ProviderAPIKeyEnv(providerName); envKey != "" {
			hasEnv = os.Getenv(envKey) != ""
		}
		hasStored := graycodeconfig.HasStoredCredentialForProvider(context.Background(), providerName)
		if !hasEnv && !hasStored {
			envKey := graycodeconfig.ProviderAPIKeyEnv(providerName)
			warnings = append(warnings, StartupWarning{
				Check:   "api_key",
				Message: fmt.Sprintf("No API key found for %s. Set %s in your environment or run /config.", providerName, envKey),
			})
		}
	}

	// 2. Check sessions directory is writable
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

// checkNetworkReachability runs a quick DNS lookup for the active provider. It
// is intentionally separate from validateStartup so the blocking lookup runs
// post-first-paint (background), not on the TUI startup critical path where an
// offline machine would stall the UI for seconds. Returns a warning message, or
// "" when reachable/not applicable.
func checkNetworkReachability(settings graycodeconfig.Settings) string {
	providerName := strings.TrimSpace(settings.Provider)
	if providerName == "" {
		providerName = strings.TrimSpace(graycodeconfig.ActiveProvider(context.Background()))
	}
	if providerName == "" || providerName == "ollama" {
		return ""
	}
	host := providerDNSHost(providerName)
	if host == "" {
		return ""
	}
	if _, err := net.LookupHost(host); err != nil {
		return fmt.Sprintf("Cannot resolve %s. Check your internet connection.", host)
	}
	return ""
}

// providerDNSHost returns a hostname to check DNS resolution for a provider.
func providerDNSHost(provider string) string {
	return graycodeconfig.GatewayDNSHost(provider)
}
