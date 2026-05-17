package health

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

// DiagnosticResult holds the outcome of a single diagnostic check.
type DiagnosticResult struct {
	Name     string
	Status   string // "pass", "warn", "fail"
	Message  string
	Fix      string
	Duration time.Duration
}

// DiagnosticSuite holds the collected results from running diagnostics.
type DiagnosticSuite struct {
	Results   []DiagnosticResult
	StartTime time.Time
	Duration  time.Duration
}

// DiagnosticCheck defines a single named check with a category and run function.
type DiagnosticCheck struct {
	Name     string
	Category string
	RunFn    func() DiagnosticResult
}

// Diagnostics manages a collection of diagnostic checks.
type Diagnostics struct {
	Checks []DiagnosticCheck
	mu     sync.Mutex
}

// NewDiagnostics creates a Diagnostics instance with built-in checks across
// environment, config, network, tools, and permissions categories.
func NewDiagnostics() *Diagnostics {
	d := &Diagnostics{}

	// Environment checks
	d.addCheck("go_version", "environment", checkGoVersion)
	d.addCheck("git_installed", "environment", checkGitInstalled)
	d.addCheck("shell_available", "environment", checkShellAvailable)
	d.addCheck("disk_space", "environment", checkDiskSpace)

	// Config checks
	d.addCheck("config_file_valid", "config", checkConfigFileValid)
	d.addCheck("api_key_set", "config", checkAPIKeySet)
	d.addCheck("model_configured", "config", checkModelConfigured)
	d.addCheck("session_dir_writable", "config", checkSessionDirWritable)

	// Network checks
	d.addCheck("anthropic_reachable", "network", checkAnthropicReachable)
	d.addCheck("openai_reachable", "network", checkOpenAIReachable)
	d.addCheck("dns_works", "network", checkDNSWorks)

	// Tools checks
	d.addCheck("git_binary", "tools", checkGitBinary)
	d.addCheck("go_binary", "tools", checkGoBinary)
	d.addCheck("node_binary", "tools", checkNodeBinary)

	// Permissions checks
	d.addCheck("config_dir_writable", "permissions", checkConfigDirWritable)
	d.addCheck("project_dir_writable", "permissions", checkProjectDirWritable)
	d.addCheck("temp_dir_writable", "permissions", checkTempDirWritable)

	return d
}

func (d *Diagnostics) addCheck(name, category string, fn func() DiagnosticResult) {
	d.Checks = append(d.Checks, DiagnosticCheck{
		Name:     name,
		Category: category,
		RunFn:    fn,
	})
}

// RunAll executes all registered diagnostic checks and returns a suite of results.
func (d *Diagnostics) RunAll() *DiagnosticSuite {
	d.mu.Lock()
	defer d.mu.Unlock()

	suite := &DiagnosticSuite{
		StartTime: time.Now(),
	}

	for _, check := range d.Checks {
		result := check.RunFn()
		suite.Results = append(suite.Results, result)
	}

	suite.Duration = time.Since(suite.StartTime)
	return suite
}

// RunCategory executes only the checks matching the given category.
func (d *Diagnostics) RunCategory(category string) *DiagnosticSuite {
	d.mu.Lock()
	defer d.mu.Unlock()

	suite := &DiagnosticSuite{
		StartTime: time.Now(),
	}

	for _, check := range d.Checks {
		if check.Category == category {
			result := check.RunFn()
			suite.Results = append(suite.Results, result)
		}
	}

	suite.Duration = time.Since(suite.StartTime)
	return suite
}

// FormatResults produces a human-readable terminal output of diagnostic results.
func FormatResults(suite *DiagnosticSuite) string {
	if suite == nil || len(suite.Results) == 0 {
		return "No diagnostic results available."
	}

	var b strings.Builder
	b.WriteString("=== Hawk Diagnostics ===\n\n")

	passCount := 0
	warnCount := 0
	failCount := 0

	for _, r := range suite.Results {
		var icon string
		switch r.Status {
		case "pass":
			icon = "✓" // checkmark
			passCount++
		case "warn":
			icon = "⚠" // warning
			warnCount++
		case "fail":
			icon = "✗" // x mark
			failCount++
		default:
			icon = "?"
		}

		b.WriteString(fmt.Sprintf("  %s %s (%s)\n", icon, r.Name, r.Duration.Round(time.Millisecond)))
		if r.Message != "" {
			b.WriteString(fmt.Sprintf("    %s\n", r.Message))
		}
		if r.Status == "fail" && r.Fix != "" {
			b.WriteString(fmt.Sprintf("    Fix: %s\n", r.Fix))
		}
	}

	b.WriteString(fmt.Sprintf("\nSummary: %d passed, %d warnings, %d failed (total %s)\n",
		passCount, warnCount, failCount, suite.Duration.Round(time.Millisecond)))

	return b.String()
}

// QuickCheck returns true if all critical checks (those that would "fail") pass.
// It runs all checks and returns false if any check has status "fail".
func (d *Diagnostics) QuickCheck() bool {
	suite := d.RunAll()
	for _, r := range suite.Results {
		if r.Status == "fail" {
			return false
		}
	}
	return true
}

// SuggestFixes returns prioritized fix suggestions from the suite results.
// Failures are listed first, then warnings.
func SuggestFixes(suite *DiagnosticSuite) []string {
	if suite == nil {
		return nil
	}

	var fixes []string

	// Failures first (higher priority)
	for _, r := range suite.Results {
		if r.Status == "fail" && r.Fix != "" {
			fixes = append(fixes, fmt.Sprintf("[FAIL] %s: %s", r.Name, r.Fix))
		}
	}

	// Warnings second
	for _, r := range suite.Results {
		if r.Status == "warn" && r.Fix != "" {
			fixes = append(fixes, fmt.Sprintf("[WARN] %s: %s", r.Name, r.Fix))
		}
	}

	return fixes
}

// --- Built-in check implementations ---

func checkGoVersion() DiagnosticResult {
	start := time.Now()
	version := runtime.Version()
	return DiagnosticResult{
		Name:     "go_version",
		Status:   "pass",
		Message:  fmt.Sprintf("Go version: %s", version),
		Duration: time.Since(start),
	}
}

func checkGitInstalled() DiagnosticResult {
	start := time.Now()
	path, err := exec.LookPath("git")
	if err != nil {
		return DiagnosticResult{
			Name:     "git_installed",
			Status:   "fail",
			Message:  "git is not installed or not in PATH",
			Fix:      "Install git: https://git-scm.com/downloads",
			Duration: time.Since(start),
		}
	}
	return DiagnosticResult{
		Name:     "git_installed",
		Status:   "pass",
		Message:  fmt.Sprintf("git found at %s", path),
		Duration: time.Since(start),
	}
}

func checkShellAvailable() DiagnosticResult {
	start := time.Now()
	shell := os.Getenv("SHELL")
	if shell == "" {
		// Try common shells
		for _, s := range []string{"/bin/sh", "/bin/bash", "/bin/zsh"} {
			if _, err := os.Stat(s); err == nil {
				shell = s
				break
			}
		}
	}
	if shell == "" {
		return DiagnosticResult{
			Name:     "shell_available",
			Status:   "fail",
			Message:  "No shell found",
			Fix:      "Set SHELL environment variable or ensure /bin/sh exists",
			Duration: time.Since(start),
		}
	}
	return DiagnosticResult{
		Name:     "shell_available",
		Status:   "pass",
		Message:  fmt.Sprintf("Shell: %s", shell),
		Duration: time.Since(start),
	}
}

func checkDiskSpace() DiagnosticResult {
	start := time.Now()
	// Use a simple heuristic: try to create a temp file to verify we can write
	tmpFile, err := os.CreateTemp("", "hawk-disk-check-*")
	if err != nil {
		return DiagnosticResult{
			Name:     "disk_space",
			Status:   "warn",
			Message:  "Could not verify disk space (temp file creation failed)",
			Fix:      "Ensure sufficient disk space and temp directory permissions",
			Duration: time.Since(start),
		}
	}
	_ = tmpFile.Close()
	_ = os.Remove(tmpFile.Name())

	return DiagnosticResult{
		Name:     "disk_space",
		Status:   "pass",
		Message:  "Disk is writable (temp file test passed)",
		Duration: time.Since(start),
	}
}

func checkConfigFileValid() DiagnosticResult {
	start := time.Now()
	home, err := os.UserHomeDir()
	if err != nil {
		return DiagnosticResult{
			Name:     "config_file_valid",
			Status:   "warn",
			Message:  "Could not determine home directory",
			Fix:      "Set HOME environment variable",
			Duration: time.Since(start),
		}
	}

	configPath := filepath.Join(home, ".hawk", "config.json")
	info, err := os.Stat(configPath)
	if os.IsNotExist(err) {
		return DiagnosticResult{
			Name:     "config_file_valid",
			Status:   "warn",
			Message:  fmt.Sprintf("Config file not found at %s", configPath),
			Fix:      "Run 'hawk init' to create a default configuration",
			Duration: time.Since(start),
		}
	}
	if err != nil {
		return DiagnosticResult{
			Name:     "config_file_valid",
			Status:   "fail",
			Message:  fmt.Sprintf("Error accessing config: %v", err),
			Fix:      "Check file permissions on ~/.hawk/config.json",
			Duration: time.Since(start),
		}
	}
	if info.Size() == 0 {
		return DiagnosticResult{
			Name:     "config_file_valid",
			Status:   "warn",
			Message:  "Config file is empty",
			Fix:      "Run 'hawk init' to regenerate configuration",
			Duration: time.Since(start),
		}
	}

	return DiagnosticResult{
		Name:     "config_file_valid",
		Status:   "pass",
		Message:  fmt.Sprintf("Config file exists at %s (%d bytes)", configPath, info.Size()),
		Duration: time.Since(start),
	}
}

func checkAPIKeySet() DiagnosticResult {
	start := time.Now()
	// Check common API key environment variables
	keys := []string{"ANTHROPIC_API_KEY", "OPENAI_API_KEY", "HAWK_API_KEY"}
	found := []string{}
	for _, k := range keys {
		if os.Getenv(k) != "" {
			found = append(found, k)
		}
	}

	if len(found) == 0 {
		return DiagnosticResult{
			Name:     "api_key_set",
			Status:   "fail",
			Message:  "No API keys found in environment",
			Fix:      "Set ANTHROPIC_API_KEY or OPENAI_API_KEY environment variable",
			Duration: time.Since(start),
		}
	}

	return DiagnosticResult{
		Name:     "api_key_set",
		Status:   "pass",
		Message:  fmt.Sprintf("API keys found: %s", strings.Join(found, ", ")),
		Duration: time.Since(start),
	}
}

func checkModelConfigured() DiagnosticResult {
	start := time.Now()
	model := os.Getenv("HAWK_MODEL")
	if model == "" {
		return DiagnosticResult{
			Name:     "model_configured",
			Status:   "warn",
			Message:  "HAWK_MODEL not set, will use default",
			Fix:      "Set HAWK_MODEL environment variable to specify preferred model",
			Duration: time.Since(start),
		}
	}
	return DiagnosticResult{
		Name:     "model_configured",
		Status:   "pass",
		Message:  fmt.Sprintf("Model configured: %s", model),
		Duration: time.Since(start),
	}
}

func checkSessionDirWritable() DiagnosticResult {
	start := time.Now()
	home, err := os.UserHomeDir()
	if err != nil {
		return DiagnosticResult{
			Name:     "session_dir_writable",
			Status:   "fail",
			Message:  "Could not determine home directory",
			Fix:      "Set HOME environment variable",
			Duration: time.Since(start),
		}
	}

	sessionDir := filepath.Join(home, ".hawk", "sessions")
	return checkDirWritable("session_dir_writable", sessionDir, start)
}

func checkAnthropicReachable() DiagnosticResult {
	start := time.Now()
	return checkTCPReachable("anthropic_reachable", "api.anthropic.com", "443", start)
}

func checkOpenAIReachable() DiagnosticResult {
	start := time.Now()
	return checkTCPReachable("openai_reachable", "api.openai.com", "443", start)
}

func checkDNSWorks() DiagnosticResult {
	start := time.Now()
	_, err := net.LookupHost("dns.google")
	if err != nil {
		return DiagnosticResult{
			Name:     "dns_works",
			Status:   "fail",
			Message:  fmt.Sprintf("DNS resolution failed: %v", err),
			Fix:      "Check network connection and DNS configuration",
			Duration: time.Since(start),
		}
	}
	return DiagnosticResult{
		Name:     "dns_works",
		Status:   "pass",
		Message:  "DNS resolution working",
		Duration: time.Since(start),
	}
}

func checkGitBinary() DiagnosticResult {
	return checkBinaryAvailable("git_binary", "git")
}

func checkGoBinary() DiagnosticResult {
	return checkBinaryAvailable("go_binary", "go")
}

func checkNodeBinary() DiagnosticResult {
	start := time.Now()
	path, err := exec.LookPath("node")
	if err != nil {
		return DiagnosticResult{
			Name:     "node_binary",
			Status:   "warn",
			Message:  "node is not installed (optional for non-JS projects)",
			Fix:      "Install Node.js if working with JavaScript/TypeScript projects",
			Duration: time.Since(start),
		}
	}
	return DiagnosticResult{
		Name:     "node_binary",
		Status:   "pass",
		Message:  fmt.Sprintf("node found at %s", path),
		Duration: time.Since(start),
	}
}

func checkConfigDirWritable() DiagnosticResult {
	start := time.Now()
	home, err := os.UserHomeDir()
	if err != nil {
		return DiagnosticResult{
			Name:     "config_dir_writable",
			Status:   "fail",
			Message:  "Could not determine home directory",
			Fix:      "Set HOME environment variable",
			Duration: time.Since(start),
		}
	}
	configDir := filepath.Join(home, ".hawk")
	return checkDirWritable("config_dir_writable", configDir, start)
}

func checkProjectDirWritable() DiagnosticResult {
	start := time.Now()
	cwd, err := os.Getwd()
	if err != nil {
		return DiagnosticResult{
			Name:     "project_dir_writable",
			Status:   "fail",
			Message:  fmt.Sprintf("Could not get working directory: %v", err),
			Fix:      "Ensure you are running hawk from a valid directory",
			Duration: time.Since(start),
		}
	}
	return checkDirWritable("project_dir_writable", cwd, start)
}

func checkTempDirWritable() DiagnosticResult {
	start := time.Now()
	tmpDir := os.TempDir()
	return checkDirWritable("temp_dir_writable", tmpDir, start)
}

// --- Helpers ---

func checkTCPReachable(name, host, port string, start time.Time) DiagnosticResult {
	conn, err := net.DialTimeout("tcp", net.JoinHostPort(host, port), 5*time.Second)
	if err != nil {
		return DiagnosticResult{
			Name:     name,
			Status:   "warn",
			Message:  fmt.Sprintf("Cannot reach %s:%s: %v", host, port, err),
			Fix:      fmt.Sprintf("Check network connectivity to %s", host),
			Duration: time.Since(start),
		}
	}
	_ = conn.Close()
	return DiagnosticResult{
		Name:     name,
		Status:   "pass",
		Message:  fmt.Sprintf("%s:%s is reachable", host, port),
		Duration: time.Since(start),
	}
}

func checkBinaryAvailable(name, binary string) DiagnosticResult {
	start := time.Now()
	path, err := exec.LookPath(binary)
	if err != nil {
		return DiagnosticResult{
			Name:     name,
			Status:   "fail",
			Message:  fmt.Sprintf("%s is not installed or not in PATH", binary),
			Fix:      fmt.Sprintf("Install %s and ensure it is in your PATH", binary),
			Duration: time.Since(start),
		}
	}
	return DiagnosticResult{
		Name:     name,
		Status:   "pass",
		Message:  fmt.Sprintf("%s found at %s", binary, path),
		Duration: time.Since(start),
	}
}

func checkDirWritable(name, dir string, start time.Time) DiagnosticResult {
	// Ensure directory exists
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return DiagnosticResult{
			Name:     name,
			Status:   "fail",
			Message:  fmt.Sprintf("Cannot create directory %s: %v", dir, err),
			Fix:      fmt.Sprintf("Create directory manually: mkdir -p %s", dir),
			Duration: time.Since(start),
		}
	}

	// Try writing a temp file in the directory
	testFile := filepath.Join(dir, ".hawk-write-test")
	if err := os.WriteFile(testFile, []byte("test"), 0o644); err != nil {
		return DiagnosticResult{
			Name:     name,
			Status:   "fail",
			Message:  fmt.Sprintf("Directory %s is not writable: %v", dir, err),
			Fix:      fmt.Sprintf("Fix permissions: chmod u+w %s", dir),
			Duration: time.Since(start),
		}
	}
	_ = os.Remove(testFile)

	return DiagnosticResult{
		Name:     name,
		Status:   "pass",
		Message:  fmt.Sprintf("Directory %s is writable", dir),
		Duration: time.Since(start),
	}
}
