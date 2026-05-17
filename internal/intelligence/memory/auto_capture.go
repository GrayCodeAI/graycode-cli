package memory

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"
)

// AutoCapture extracts memorable content from tool results without explicit agent calls.
// It analyzes tool outputs and automatically stores conventions, decisions, bugs, and
// file-level knowledge into yaad's graph.
type AutoCapture struct {
	bridge  *YaadBridge
	mu      sync.Mutex
	queue   chan captureJob
	done    chan struct{}
	metrics *CaptureMetrics
}

type captureJob struct {
	toolName string
	args     map[string]interface{}
	output   string
	isErr    bool
	ts       time.Time
}

// CaptureMetrics tracks auto-capture statistics.
type CaptureMetrics struct {
	mu             sync.Mutex
	Captured       int
	Skipped        int
	ConventionsOut int
	DecisionsOut   int
	BugsOut        int
	FilesOut       int
}

func (m *CaptureMetrics) inc(nodeType string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Captured++
	switch nodeType {
	case "convention":
		m.ConventionsOut++
	case "decision":
		m.DecisionsOut++
	case "bug":
		m.BugsOut++
	case "file":
		m.FilesOut++
	}
}

// NewAutoCapture creates an auto-capture processor that consumes tool results
// in the background and stores extracted memories.
func NewAutoCapture(bridge *YaadBridge) *AutoCapture {
	ac := &AutoCapture{
		bridge:  bridge,
		queue:   make(chan captureJob, 128),
		done:    make(chan struct{}),
		metrics: &CaptureMetrics{},
	}
	go ac.worker()
	return ac
}

// Ingest queues a tool result for background memory extraction.
func (ac *AutoCapture) Ingest(toolName string, args map[string]interface{}, output string, isErr bool) {
	if !ac.bridge.Ready() {
		return
	}
	select {
	case ac.queue <- captureJob{
		toolName: toolName,
		args:     args,
		output:   output,
		isErr:    isErr,
		ts:       time.Now(),
	}:
	default:
		ac.metrics.mu.Lock()
		ac.metrics.Skipped++
		ac.metrics.mu.Unlock()
	}
}

// Metrics returns capture statistics.
func (ac *AutoCapture) Metrics() CaptureMetrics {
	ac.metrics.mu.Lock()
	defer ac.metrics.mu.Unlock()
	return CaptureMetrics{
		Captured:       ac.metrics.Captured,
		Skipped:        ac.metrics.Skipped,
		ConventionsOut: ac.metrics.ConventionsOut,
		DecisionsOut:   ac.metrics.DecisionsOut,
		BugsOut:        ac.metrics.BugsOut,
		FilesOut:       ac.metrics.FilesOut,
	}
}

// Stop halts background processing.
func (ac *AutoCapture) Stop() {
	close(ac.queue)
	<-ac.done
}

func (ac *AutoCapture) worker() {
	defer close(ac.done)
	for job := range ac.queue {
		ac.process(job)
	}
}

func (ac *AutoCapture) process(job captureJob) {
	ac.mu.Lock()
	defer ac.mu.Unlock()

	switch canonicalName(job.toolName) {
	case "Write", "Edit":
		ac.processFileWrite(job)
	case "Bash":
		ac.processBash(job)
	case "Read", "Grep", "Glob":
		ac.processRead(job)
	}

	if job.isErr {
		ac.processError(job)
	}
}

func (ac *AutoCapture) processFileWrite(job captureJob) {
	path, ok := extractPath(job.args)
	if !ok || path == "" {
		return
	}
	_ = ac.bridge.Remember(
		fmt.Sprintf("File modified: %s", path),
		"file",
	)
	ac.metrics.inc("file")
}

func (ac *AutoCapture) processBash(job captureJob) {
	cmd, _ := job.args["command"].(string)
	if cmd == "" {
		return
	}

	// Detect test commands and their outcomes
	if isTestCommand(cmd) {
		if job.isErr || containsTestFailure(job.output) {
			snippet := truncate(job.output, 300)
			_ = ac.bridge.Remember(
				fmt.Sprintf("Test failure: `%s` → %s", truncate(cmd, 100), snippet),
				"bug",
			)
			ac.metrics.inc("bug")
		}
		return
	}

	// Detect git commits → extract decisions
	if isGitCommit(cmd) && !job.isErr {
		msg := extractCommitMessage(cmd)
		if msg != "" {
			_ = ac.bridge.Remember(
				fmt.Sprintf("Commit: %s", msg),
				"decision",
			)
			ac.metrics.inc("decision")
		}
		return
	}

	// Detect package install → extract dependency decisions
	if isPackageInstall(cmd) {
		pkg := extractPackageName(cmd)
		if pkg != "" {
			_ = ac.bridge.Remember(
				fmt.Sprintf("Dependency added: %s", pkg),
				"decision",
			)
			ac.metrics.inc("decision")
		}
		return
	}

	// Detect build/deploy commands as conventions
	if isBuildCommand(cmd) && !job.isErr {
		_ = ac.bridge.Remember(
			fmt.Sprintf("Build command: `%s`", truncate(cmd, 200)),
			"convention",
		)
		ac.metrics.inc("convention")
	}
}

func (ac *AutoCapture) processRead(job captureJob) {
	// Extract structural discoveries from file reads (only on large files with clear structure)
	path, ok := extractPath(job.args)
	if !ok || path == "" {
		return
	}
	// Only track significant reads (file structure discovery)
	if len(job.output) > 500 && isStructuralFile(path) {
		_ = ac.bridge.Remember(
			fmt.Sprintf("Project file: %s", path),
			"file",
		)
		ac.metrics.inc("file")
	}
}

func (ac *AutoCapture) processError(job captureJob) {
	if job.output == "" || len(job.output) < 20 {
		return
	}
	// Extract error patterns that are likely bugs
	if containsErrorPattern(job.output) {
		snippet := truncate(job.output, 300)
		_ = ac.bridge.Remember(
			fmt.Sprintf("Error in %s: %s", job.toolName, snippet),
			"bug",
		)
		ac.metrics.inc("bug")
	}
}

// Pattern matchers

var (
	testCmdPattern    = regexp.MustCompile(`(?i)(go test|npm test|jest|pytest|vitest|cargo test|make test|pnpm test|yarn test)`)
	gitCommitPattern  = regexp.MustCompile(`git commit`)
	pkgInstallPattern = regexp.MustCompile(`(?i)(go get|npm install|pip install|cargo add|pnpm add|yarn add|brew install)`)
	buildCmdPattern   = regexp.MustCompile(`(?i)(go build|npm run build|make build|cargo build|docker build|make deploy|fly deploy)`)
	testFailPattern   = regexp.MustCompile(`(?i)(FAIL|FAILED|error|panic|assertion|expected .+ got)`)
	errorPattern      = regexp.MustCompile(`(?i)(error:|Error:|ERRO|panic:|fatal:|exception|traceback)`)
	structuralFiles   = regexp.MustCompile(`(?i)(package\.json|go\.mod|Cargo\.toml|pyproject\.toml|Makefile|Dockerfile|docker-compose|tsconfig|\.env\.example)`)
)

func isTestCommand(cmd string) bool        { return testCmdPattern.MatchString(cmd) }
func isGitCommit(cmd string) bool          { return gitCommitPattern.MatchString(cmd) }
func isPackageInstall(cmd string) bool     { return pkgInstallPattern.MatchString(cmd) }
func isBuildCommand(cmd string) bool       { return buildCmdPattern.MatchString(cmd) }
func containsTestFailure(out string) bool  { return testFailPattern.MatchString(out) }
func containsErrorPattern(out string) bool { return errorPattern.MatchString(out) }
func isStructuralFile(path string) bool    { return structuralFiles.MatchString(path) }

func extractCommitMessage(cmd string) string {
	// Match -m "message" or -m 'message'
	re := regexp.MustCompile(`-m\s+["']([^"']+)["']`)
	if m := re.FindStringSubmatch(cmd); len(m) > 1 {
		return m[1]
	}
	// Match heredoc style
	re2 := regexp.MustCompile(`-m\s+"([^"]+)"`)
	if m := re2.FindStringSubmatch(cmd); len(m) > 1 {
		return m[1]
	}
	return ""
}

func extractPackageName(cmd string) string {
	parts := strings.Fields(cmd)
	if len(parts) >= 3 {
		return parts[len(parts)-1]
	}
	return ""
}

func extractPath(args map[string]interface{}) (string, bool) {
	if p, ok := args["file_path"].(string); ok {
		return p, true
	}
	if p, ok := args["path"].(string); ok {
		return p, true
	}
	if p, ok := args["filePath"].(string); ok {
		return p, true
	}
	return "", false
}

func canonicalName(name string) string {
	switch strings.ToLower(name) {
	case "write", "file_write":
		return "Write"
	case "edit", "file_edit":
		return "Edit"
	case "bash", "shell", "execute":
		return "Bash"
	case "read", "file_read":
		return "Read"
	case "grep", "search":
		return "Grep"
	case "glob", "find":
		return "Glob"
	default:
		return name
	}
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}

// ExtractConventions scans text for convention-like statements.
func ExtractConventions(text string) []string {
	var conventions []string
	patterns := []string{
		`(?i)always\s+(.{10,80})`,
		`(?i)never\s+(.{10,80})`,
		`(?i)we\s+use\s+(.{5,80})`,
		`(?i)the\s+convention\s+is\s+(.{10,80})`,
		`(?i)prefer\s+(.{5,80})\s+over\s+(.{5,80})`,
		`(?i)make\s+sure\s+to\s+(.{10,80})`,
	}
	for _, p := range patterns {
		re := regexp.MustCompile(p)
		matches := re.FindAllStringSubmatch(text, 3)
		for _, m := range matches {
			if len(m) > 1 {
				conventions = append(conventions, strings.TrimSpace(m[0]))
			}
		}
	}
	return conventions
}

// ExtractFromAssistantResponse analyzes assistant text for memorable content
// and stores it via the bridge. Called from the agent loop when the LLM produces
// text that contains implicit conventions or decisions.
func (ac *AutoCapture) ExtractFromAssistantResponse(ctx context.Context, text string) {
	if !ac.bridge.Ready() || text == "" {
		return
	}
	conventions := ExtractConventions(text)
	for _, c := range conventions {
		_ = ac.bridge.Remember(c, "convention")
		ac.metrics.inc("convention")
	}
}
