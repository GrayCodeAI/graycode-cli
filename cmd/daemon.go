package cmd

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	hawkconfig "github.com/GrayCodeAI/hawk/internal/config"
	"github.com/GrayCodeAI/hawk/internal/daemon"
	"github.com/GrayCodeAI/hawk/internal/engine"
	"github.com/GrayCodeAI/hawk/internal/home"
	"github.com/GrayCodeAI/hawk/internal/netutil"
	"github.com/GrayCodeAI/hawk/internal/observability/logger"
	"github.com/spf13/cobra"
)

var (
	daemonPort   int
	daemonHost   string
	daemonAPIKey string
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
	daemonCmd.AddCommand(daemonStartCmd)
	daemonCmd.AddCommand(daemonStopCmd)
	daemonCmd.AddCommand(daemonStatusCmd)
}

func runDaemonStart(_ *cobra.Command, _ []string) error {
	settings := hawkconfig.LoadSettings()
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

	factory := func(req daemon.ChatRequest) (*engine.Session, error) {
		systemPrompt, err := buildSystemPrompt()
		if err != nil {
			return nil, err
		}
		registry, err := defaultRegistry(settings)
		if err != nil {
			return nil, err
		}
		effectiveModel, effectiveProvider := effectiveModelAndProvider(settings)
		if req.Model != "" {
			effectiveModel = req.Model
		}
		sess := newHawkSession(settings, effectiveProvider, effectiveModel, systemPrompt, registry)
		sess.SetLogger(logger.New(io.Discard, logger.Error))
		if err := configureSession(sess, settings); err != nil {
			return nil, err
		}
		return sess, nil
	}

	srv := daemon.New(daemon.Config{Port: daemonPort, Host: daemonHost, APIKey: apiKey}, factory)
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
	fmt.Println("Endpoints: GET /v1/health, POST /v1/chat, GET /v1/sessions")
	fmt.Println("Protected endpoints require Authorization: Bearer <api-key> or X-API-Key.")
	if len(apiKey) > 8 {
		fmt.Printf("API key: %s...%s\n", apiKey[:4], apiKey[len(apiKey)-4:])
	} else {
		fmt.Println("API key: (set via --api-key or HAWK_DAEMON_API_KEY)")
	}
	keyFile := filepath.Join(home.Dir(), ".hawk", "run", "daemon.key")
	_ = os.MkdirAll(filepath.Dir(keyFile), 0o700)
	if err := os.WriteFile(keyFile, []byte(apiKey), 0o600); err == nil {
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
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return srv.Stop(ctx)
}

func generateDaemonAPIKey() (string, error) {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("generate daemon API key: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b[:]), nil
}

func runDaemonStop(_ *cobra.Command, _ []string) error {
	home := home.Dir()
	pidFile := filepath.Join(home, ".hawk", "run", "daemon.json")

	data, err := os.ReadFile(pidFile)
	if err != nil {
		return fmt.Errorf("no daemon running (PID file not found)")
	}

	var info struct {
		PID  int    `json:"pid"`
		Addr string `json:"addr"`
	}
	if err := json.Unmarshal(data, &info); err != nil {
		return fmt.Errorf("invalid PID file: %w", err)
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
	home := home.Dir()
	pidFile := filepath.Join(home, ".hawk", "run", "daemon.json")

	data, err := os.ReadFile(pidFile)
	if err != nil {
		fmt.Println("Status: not running")
		return nil
	}

	var info struct {
		PID       int    `json:"pid"`
		Addr      string `json:"addr"`
		StartedAt string `json:"started_at"`
	}
	if err := json.Unmarshal(data, &info); err != nil {
		fmt.Println("Status: unknown (invalid PID file)")
		return nil
	}

	// Check if process is alive
	proc, err := os.FindProcess(info.PID)
	if err != nil {
		fmt.Println("Status: not running (stale PID file)")
		_ = os.Remove(pidFile)
		return nil
	}
	if err := proc.Signal(syscall.Signal(0)); err != nil {
		fmt.Println("Status: not running (stale PID file)")
		_ = os.Remove(pidFile)
		return nil
	}

	fmt.Printf("Status: running\n")
	fmt.Printf("  PID:     %d\n", info.PID)
	fmt.Printf("  Address: http://%s\n", info.Addr)
	fmt.Printf("  Started: %s\n", info.StartedAt)
	return nil
}
