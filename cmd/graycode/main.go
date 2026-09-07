package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"golang.org/x/term"

	"github.com/GrayCodeAI/graycode-cli/cmd"
	"github.com/GrayCodeAI/graycode-cli/internal/crash"
	"github.com/GrayCodeAI/graycode-cli/internal/graycodeerr"
	"github.com/GrayCodeAI/graycode-cli/internal/mcp"
	"github.com/GrayCodeAI/graycode-cli/internal/observability/otellog"
	"github.com/GrayCodeAI/graycode-cli/internal/observability/oteltrace"
)

// Version, Commit, and BuildDate are set at build time via ldflags.
//
// Source of truth: the VERSION file at the repo root, and the matching git
// tag created by release-please. The Makefile and goreleaser inject these
// values during release builds:
//
//	-X main.Version=$(cat VERSION)
//	-X main.Commit=$(git rev-parse --short HEAD)
//	-X main.BuildDate=$(date -u +%Y-%m-%dT%H:%M:%SZ)
//
// The "dev" / "none" / "unknown" defaults below apply only to local builds
// without ldflags so it's obvious when running an unreleased binary.
var (
	Version   = "dev"
	Commit    = "none"
	BuildDate = "unknown"
)

func main() {
	// Install the crash handler first so SIGQUIT/SIGTERM dumps and runtime
	// fault output are captured before any user code runs. Additive: it never
	// replaces existing signal handling (Bubble Tea, daemon shutdown).
	crash.Install()

	// Handle --version flag immediately
	if len(os.Args) > 1 && os.Args[1] == "--version" {
		fmt.Println("graycode " + Version)
		return
	}

	// Initialize OpenTelemetry telemetry (opt-in via GRAYCODE_ENABLE_TELEMETRY=1).
	// Telemetry failures are non-fatal: graycode continues with in-memory tracing only.
	telemetryProviders, telemetryErr := oteltrace.InitTelemetry(oteltrace.DefaultTelemetryConfig())
	if telemetryErr != nil {
		fmt.Fprintln(os.Stderr, "warning: telemetry initialization failed:", telemetryErr)
	}
	if telemetryProviders != nil && telemetryErr == nil && telemetryProviders.IsEnabled() {
		defer func() {
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = telemetryProviders.Shutdown(shutdownCtx)
		}()
	}

	// OTLP log-record export (DSH session-telemetry-otel port). Opt-in like
	// tracing: GRAYCODE_ENABLE_TELEMETRY=1 plus an OTLP logs endpoint. The
	// sharing policy gates emission; failures are non-fatal.
	logBackend, logBackendErr := otellog.NewBackend(otellog.DefaultConfig())
	if logBackendErr != nil {
		fmt.Fprintln(os.Stderr, "warning: telemetry log backend initialization failed:", logBackendErr)
	}
	if logBackend != nil && logBackend.Sharing() != otellog.SharingDisabled {
		defer func() {
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = logBackend.Shutdown(shutdownCtx)
		}()
	}

	// Propagate the canonical version to all sub-packages that surface it
	// (CLI version flag, HTTP API version field, and MCP clientInfo).
	// The sandbox image has an independent compatibility version.
	cmd.SetVersion(Version)
	cmd.SetBuildDate(BuildDate)
	mcp.SetClientVersion(Version)

	if err := cmd.RunWithPanicRecovery(cmd.Execute); err != nil {
		printError(err)
		// An explicit ExitCodeError (e.g. a wrapped Bash exit status) wins —
		// it already carries the intended code. Otherwise classify the failure
		// into the stable exit-code taxonomy so callers can branch on the
		// reason (auth vs rate-limit vs network) instead of seeing a bare 1.
		var exitErr *cmd.ExitCodeError
		if errors.As(err, &exitErr) {
			os.Exit(exitErr.Code)
		}
		os.Exit(graycodeerr.ClassifyExitCode(err))
	}
}

// errorCoralRGB is the brand error color (#FF6B6B) as an SGR truecolor
// sequence, applied only when stderr is a color-capable terminal so scripts
// piping diagnostics never see raw ANSI.
const errorCoralRGB = "\x1b[38;2;255;107;107m"

// printError writes a top-level failure to stderr, colorized (error coral)
// when the terminal supports it and NO_COLOR is unset.
func printError(err error) {
	msg := err.Error()
	if shouldColorErr() {
		msg = errorCoralRGB + msg + "\x1b[m"
	}
	fmt.Fprintln(os.Stderr, msg)
}

// shouldColorErr mirrors cmd.ShouldColor's detection for stderr: NO_COLOR
// wins, then FORCE_COLOR, else the terminal's color support.
func shouldColorErr() bool {
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	if os.Getenv("FORCE_COLOR") != "" {
		return true
	}
	return term.IsTerminal(int(os.Stderr.Fd()))
}
