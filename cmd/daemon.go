package cmd

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	hawkconfig "github.com/GrayCodeAI/hawk/internal/config"
	"github.com/GrayCodeAI/hawk/internal/daemon"
	"github.com/GrayCodeAI/hawk/internal/engine"
	"github.com/GrayCodeAI/hawk/internal/executiongraph"
	"github.com/GrayCodeAI/hawk/internal/fsutil"
	"github.com/GrayCodeAI/hawk/internal/multiagent/agents"
	"github.com/GrayCodeAI/hawk/internal/netutil"
	"github.com/GrayCodeAI/hawk/internal/observability/logger"
	"github.com/GrayCodeAI/hawk/internal/observability/oteltrace"
	"github.com/GrayCodeAI/hawk/internal/securitylog"
	"github.com/GrayCodeAI/hawk/internal/storage"
	"github.com/spf13/cobra"
)

var (
	daemonPort        int
	daemonHost        string
	daemonAPIKey      string
	daemonJSON        bool
	daemonLogLevel    string
	daemonCORSOrigins []string
	daemonTLSCertFile string
	daemonTLSKeyFile  string
	daemonAutonomy    string
)

var daemonCmd = &cobra.Command{
	Use:   "daemon",
	Short: "Manage the hawk background server",
	Long:  "Run hawk as a background HTTP server for programmatic/CI access.",
}

var daemonStartCmd = &cobra.Command{
	Use:   "start",
	Short: "Start the daemon server",
	RunE:  runDaemonStart,
}

var daemonStopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop the running daemon",
	RunE:  runDaemonStop,
}

var daemonStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show daemon status",
	RunE:  runDaemonStatus,
}

func init() {
	daemonStartCmd.Flags().IntVarP(&daemonPort, "port", "p", 4590, "Port to listen on")
	daemonStartCmd.Flags().StringVar(&daemonHost, "host", netutil.LoopbackHost, "Host to bind to (default: 127.0.0.1, use 0.0.0.0 for remote access)")
	daemonStartCmd.Flags().StringVar(&daemonAPIKey, "api-key", "", "API key for protected daemon endpoints (defaults to HAWK_DAEMON_API_KEY or a generated key)")
	daemonStartCmd.Flags().StringVar(&daemonLogLevel, "log-level", "INFO", "Log level for daemon output (DEBUG, INFO, WARN, ERROR)")
	daemonStartCmd.Flags().StringSliceVar(&daemonCORSOrigins, "cors", []string{}, "Comma-separated list of allowed CORS origins (empty disables CORS, '*' allows all)")
	daemonStartCmd.Flags().StringVar(&daemonTLSCertFile, "tls-cert", "", "Path to TLS certificate file (enables HTTPS when paired with --tls-key)")
	daemonStartCmd.Flags().StringVar(&daemonTLSKeyFile, "tls-key", "", "Path to TLS private key file (enables HTTPS when paired with --tls-cert)")
	daemonStartCmd.Flags().StringVar(&daemonAutonomy, "autonomy", "", "Maximum autonomy tier clients may request via the API (supervised, basic, semi, full, yolo; default: semi). The daemon is non-interactive, so full/yolo require an explicit opt-in.")
	daemonCmd.AddCommand(daemonStartCmd)
	daemonCmd.AddCommand(daemonStopCmd)
	daemonCmd.AddCommand(daemonStatusCmd)
	daemonStatusCmd.Flags().BoolVar(&daemonJSON, "json", false, "output status as JSON")
}

func runDaemonStart(_ *cobra.Command, _ []string) error {
	settings := hawkconfig.LoadSettings()

	// Initialize OpenTelemetry telemetry (opt-in via HAWK_CODE_ENABLE_TELEMETRY=1).
	telemetryProviders, telemetryErr := oteltrace.InitTelemetry(oteltrace.DefaultTelemetryConfig())
	if telemetryErr != nil {
		fmt.Fprintln(os.Stderr, "warning: telemetry initialization failed:", telemetryErr)
	}

	// Set up file-backed logging for the daemon. Logs go to
	// ~/.hawk/state/daemon.log with slog structured output.
	logFile, logErr := openDaemonLogFile()
	var daemonLogger *logger.Logger
	if logErr != nil {
		// Fall back to stderr if file logging fails.
		daemonLogger = logger.New(os.Stderr, logLevelFromString(daemonLogLevel))
		fmt.Fprintln(os.Stderr, "warning: daemon file logging failed, falling back to stderr:", logErr)
	} else {
		daemonLogger = logger.New(logFile, logLevelFromString(daemonLogLevel))
	}
	if logFile != nil {
		slog.SetDefault(slog.New(slog.NewTextHandler(logFile, &slog.HandlerOptions{
			Level: slogLevelFromString(daemonLogLevel),
		})))
	}

	// Replace the discarded logger with the real file-backed logger.
	newSession := newConfiguredHawkSessionFactory(settings, daemonLogger)

	// Log startup banner.
	daemonLogger.Info("hawk daemon starting", map[string]interface{}{
		"host":              daemonHost,
		"port":              daemonPort,
		"telemetry_enabled": telemetryProviders != nil && telemetryProviders.IsEnabled(),
	})

	apiKey := daemonAPIKey
	if apiKey == "" {
		apiKey = os.Getenv("HAWK_DAEMON_API_KEY")
	}
	if apiKey == "" {
		var err error
		apiKey, err = generateDaemonAPIKey()
		if err != nil {
			return err
		}
	}

	// Initialize the security audit log early so it can be shared between
	// the daemon server and the session factory (for tool execution auditing).
	var secLog *securitylog.Log
	if l, err := securitylog.New(securitylog.DefaultDir()); err != nil {
		daemonLogger.Warn("failed to initialize security audit log", map[string]interface{}{"error": err})
	} else {
		secLog = l
	}

	factory := func(req daemon.ChatRequest) (*engine.Session, error) {
		systemPrompt, err := buildSystemPrompt()
		if err != nil {
			return nil, err
		}
		var agentModel string
		systemPrompt, agentModel, err = daemonAgentConfig(req.Agent, systemPrompt)
		if err != nil {
			return nil, err
		}
		modelOverride := ""
		if req.Model != "" {
			modelOverride = req.Model
		} else if agentModel != "" {
			modelOverride = agentModel
		}
		session, err := newSession(systemPrompt, modelOverride)
		if err != nil {
			return nil, err
		}
		// Wire the audit log into the session's tool service so tool
		// executions are recorded in the tamper-evident log.
		if secLog != nil {
			session.Tools().WithAuditLog(secLog)
		}
		return session, nil
	}

	daemon.SetVersion(version)
	srv := daemon.New(daemon.Config{
		Port:        daemonPort,
		Host:        daemonHost,
		APIKey:      apiKey,
		CORSOrigins: daemonCORSOrigins,
		TLSCertFile: daemonTLSCertFile,
		TLSKeyFile:  daemonTLSKeyFile,
		SecurityLog: secLog,
		MaxAutonomy: daemonAutonomyFromFlag(daemonAutonomy),
	}, factory)
	srv.SetGraphFactory(func(ctx context.Context, req daemon.GraphRequest) (executiongraph.Export, error) {
		if err := ctx.Err(); err != nil {
			return executiongraph.Export{}, err
		}
		return buildExecutionGraphExport(
			[]string{req.SessionID},
			req.RepositoryID,
			req.TraceCheckpointIDs,
			req.GeneratedAt,
		)
	})

	// Wire Eyrie's authoritative local preflight into GET /v1/ready. A session
	// factory only proves Hawk can attempt construction; readiness additionally
	// requires Eyrie's provider state, catalog, credentials, and model selection.
	srv.SetReadyFn(daemonReadyProbe(factory))
	addr, err := srv.Start()
	if err != nil {
		return err
	}

	// Start background preheater to keep LLM connections warm
	preheater := daemon.NewPreheater(30 * time.Second)
	preheater.Start([]string{
		"https://api.anthropic.com/v1/messages",
		"https://api.openai.com/v1/chat/completions",
		fmt.Sprintf("http://%s:%d/v1/health", daemonHost, daemonPort),
	})
	defer preheater.Stop()

	fmt.Printf("hawk daemon running on http://%s\n", addr)
	fmt.Println("Endpoints: GET /v1/health, GET /v1/ready, POST /v1/chat, GET /v1/sessions, GET /v1/metrics")
	fmt.Println("Protected endpoints require Authorization: Bearer <api-key> or X-API-Key.")
	if len(apiKey) > 8 {
		fmt.Printf("API key: %s...%s\n", apiKey[:4], apiKey[len(apiKey)-4:])
	} else {
		fmt.Println("API key: (set via --api-key or HAWK_DAEMON_API_KEY)")
	}
	fmt.Printf("Logs: %s\n", filepath.Join(storage.DaemonRunDir(), "daemon.log"))
	if telemetryProviders != nil && telemetryProviders.IsEnabled() {
		fmt.Println("Telemetry: enabled (OTLP export configured)")
	} else {
		fmt.Println("Telemetry: disabled (set HAWK_CODE_ENABLE_TELEMETRY=1 to enable)")
	}
	keyFile := filepath.Join(storage.DaemonRunDir(), "daemon.key")
	_ = os.MkdirAll(filepath.Dir(keyFile), 0o700)
	if err := fsutil.WritePinnedFile(keyFile, []byte(apiKey), 0o600); err == nil {
		fmt.Printf("Full API key written to %s\n", keyFile)
	}

	// SSH tunnel hint for remote access
	if daemonHost == netutil.LoopbackHost {
		fmt.Println("\nFor remote access via SSH tunnel:")
		fmt.Printf("  ssh -L %d:127.0.0.1:%d <remote-host>\n", daemonPort, daemonPort)
		fmt.Printf("  curl http://localhost:%d/v1/health\n", daemonPort)
	} else {
		fmt.Println("\nWARNING: Bound to non-localhost. Ensure TLS is configured for production use.")
	}

	fmt.Println("Press Ctrl+C to stop.")

	// Wait for interrupt
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig

	fmt.Println("\nShutting down...")
	// Clean up the API key file to avoid leaving secrets on disk.
	_ = os.Remove(keyFile)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Flush telemetry before shutdown.
	if telemetryProviders != nil {
		if err := telemetryProviders.Flush(ctx); err != nil {
			slog.Warn("telemetry flush failed", "error", err)
		}
		if err := telemetryProviders.Shutdown(ctx); err != nil {
			slog.Warn("telemetry shutdown failed", "error", err)
		}
	}

	return srv.Stop(ctx)
}

// openDaemonLogFile opens (or creates) the daemon log file at
// ~/.hawk/state/daemon.log and returns it. The directory is created if needed.
func openDaemonLogFile() (*os.File, error) {
	dir := storage.DaemonRunDir()
	if err := os.MkdirAll(dir, 0o750); err != nil { // #nosec G301 -- daemon run dir needs group traversal
		return nil, err
	}
	return os.OpenFile(filepath.Join(dir, "daemon.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
}

// logLevelFromLevelString maps a string log level to the logger.Level type.
func logLevelFromString(s string) logger.Level {
	switch strings.ToUpper(strings.TrimSpace(s)) {
	case "DEBUG":
		return logger.Debug
	case "INFO":
		return logger.Info
	case "WARN":
		return logger.Warn
	case "ERROR":
		return logger.Error
	case "FATAL":
		return logger.Fatal
	default:
		return logger.Info
	}
}

// slogLevelFromString maps a string log level to slog.Level.
func slogLevelFromString(s string) slog.Level {
	switch strings.ToUpper(strings.TrimSpace(s)) {
	case "DEBUG":
		return slog.LevelDebug
	case "INFO":
		return slog.LevelInfo
	case "WARN":
		return slog.LevelWarn
	case "ERROR":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// daemonReadyProbe builds the readiness function installed via SetReadyFn. It
// performs Eyrie's local preflight (provider state, catalog, credentials, and
// model selection) under a short timeout:
//
//   - No session factory wired      -> not ready ("engine not configured").
//   - Preflight reports ready        -> ready.
//   - Preflight reports incomplete   -> not ready with the failed Eyrie check.
//
// This never performs a paid/live model call — Eyrie preflight only inspects
// local catalog/credential/model state — so it is safe to call on every probe.
func daemonReadyProbe(factory daemon.SessionFactory) func() (bool, string) {
	return daemonReadyProbeWithPreflight(factory, hawkconfig.EnginePreflightReport)
}

func daemonReadyProbeWithPreflight(factory daemon.SessionFactory, preflight func(context.Context) hawkconfig.EnginePreflight) func() (bool, string) {
	return func() (bool, string) {
		if factory == nil {
			return false, "engine not configured"
		}
		if preflight == nil {
			return false, "Eyrie readiness probe not configured"
		}
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		report := preflight(ctx)
		if report.Ready {
			return true, ""
		}
		for _, check := range report.Checks {
			if string(check.Status) == "fail" {
				detail := strings.TrimSpace(check.Detail)
				if detail == "" {
					detail = "failed"
				}
				return false, fmt.Sprintf("Eyrie %s: %s", check.Name, detail)
			}
		}
		if ctx.Err() != nil {
			return false, "Eyrie preflight timed out"
		}
		return false, "Eyrie preflight is not ready"
	}
}

func daemonAgentConfig(name, basePrompt string) (string, string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return basePrompt, "", nil
	}
	agentDef, err := agents.Get(name)
	if err != nil {
		return "", "", &daemon.InvalidChatRequestError{
			Message: fmt.Sprintf("agent %q is not available", name),
			Err:     err,
		}
	}
	prompt := strings.TrimSpace(agentDef.Prompt)
	if prompt != "" {
		basePrompt = prompt + "\n\n" + basePrompt
	}
	return basePrompt, strings.TrimSpace(agentDef.Model), nil
}

func generateDaemonAPIKey() (string, error) {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("generate daemon API key: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b[:]), nil
}

// daemonAutonomyFromFlag resolves the --autonomy flag / HAWK_DAEMON_AUTONOMY
// env var into the server-side autonomy cap. An empty value leaves the
// default cap (AutonomySemi) in place; invalid values fail closed at
// "supervised" rather than silently allowing full autonomy.
func daemonAutonomyFromFlag(s string) engine.AutonomyLevel {
	s = strings.TrimSpace(s)
	if s == "" {
		s = os.Getenv("HAWK_DAEMON_AUTONOMY")
	}
	if s == "" {
		return 0 // zero => DefaultMaxAutonomy in the daemon
	}
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "0", "supervised":
		return engine.AutonomySupervised
	case "1", "basic":
		return engine.AutonomyBasic
	case "2", "semi", "accept_edits", "acceptedits":
		return engine.AutonomySemi
	case "3", "full":
		return engine.AutonomyFull
	case "4", "yolo", "dont_ask", "dontask":
		return engine.AutonomyYOLO
	default:
		return engine.AutonomySupervised
	}
}

func runDaemonStop(_ *cobra.Command, _ []string) error {
	pidFile := filepath.Join(storage.DaemonRunDir(), "daemon.json")

	data, err := os.ReadFile(pidFile) // #nosec G304 -- pidFile built from internal daemon run directory
	if err != nil {
		return fmt.Errorf("no daemon running (PID file not found)")
	}

	var info struct {
		PID  int    `json:"pid"`
		Addr string `json:"addr"`
	}
	if unmarshalErr := json.Unmarshal(data, &info); unmarshalErr != nil {
		return fmt.Errorf("invalid PID file: %w", unmarshalErr)
	}

	proc, err := os.FindProcess(info.PID)
	if err != nil {
		return fmt.Errorf("process %d not found: %w", info.PID, err)
	}

	if err := proc.Signal(syscall.SIGTERM); err != nil {
		return fmt.Errorf("failed to stop daemon (PID %d): %w", info.PID, err)
	}

	_ = os.Remove(pidFile)
	fmt.Printf("Stopped daemon (PID %d)\n", info.PID)
	return nil
}

func runDaemonStatus(_ *cobra.Command, _ []string) error {
	pidFile := filepath.Join(storage.DaemonRunDir(), "daemon.json")

	data, err := os.ReadFile(pidFile) // #nosec G304 -- pidFile built from internal daemon run directory
	if err != nil {
		if daemonJSON {
			fmt.Println(`{"status":"not running"}`)
		} else {
			fmt.Println("Status: not running")
		}
		return nil
	}

	var info struct {
		PID       int    `json:"pid"`
		Addr      string `json:"addr"`
		StartedAt string `json:"started_at"`
	}
	if unmarshalErr := json.Unmarshal(data, &info); unmarshalErr != nil {
		if daemonJSON {
			fmt.Println(`{"status":"unknown","error":"invalid PID file"}`)
		} else {
			fmt.Println("Status: unknown (invalid PID file)")
		}
		return nil
	}

	// Check if process is alive
	proc, err := os.FindProcess(info.PID)
	if err != nil {
		if daemonJSON {
			fmt.Println(`{"status":"not running","error":"stale PID file"}`)
		} else {
			fmt.Println("Status: not running (stale PID file)")
		}
		_ = os.Remove(pidFile)
		return nil
	}
	if err := proc.Signal(syscall.Signal(0)); err != nil {
		if daemonJSON {
			fmt.Println(`{"status":"not running","error":"stale PID file"}`)
		} else {
			fmt.Println("Status: not running (stale PID file)")
		}
		_ = os.Remove(pidFile)
		return nil
	}

	if daemonJSON {
		out, _ := json.MarshalIndent(map[string]any{
			"status":     "running",
			"pid":        info.PID,
			"address":    "http://" + info.Addr,
			"started_at": info.StartedAt,
		}, "", "  ")
		fmt.Println(string(out))
		return nil
	}

	fmt.Printf("Status: running\n")
	fmt.Printf("  PID:     %d\n", info.PID)
	fmt.Printf("  Address: http://%s\n", info.Addr)
	fmt.Printf("  Started: %s\n", info.StartedAt)
	return nil
}
