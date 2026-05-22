package errs

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

type ErrorRecovery struct {
	Strategies  map[string]*RecoveryStrategy
	History     []RecoveryAttempt
	MaxAttempts int
	mu          sync.Mutex
}

type RecoveryStrategy struct {
	Name         string
	ErrorPattern *regexp.Regexp
	RecoverFn    func(err error, context *RecoveryContext) (*RecoveryResult, error)
	Priority     int
	SuccessCount int
	FailureCount int
}

type RecoveryContext struct {
	Error         error
	ErrorMsg      string
	LastToolCall  string
	LastArgs      map[string]interface{}
	Messages      []string
	FilesModified []string
	Attempt       int
}

type RecoveryResult struct {
	Recovered bool
	Action    string
	Message   string
	RetryWith string
}

type RecoveryAttempt struct {
	Error     string
	Strategy  string
	Recovered bool
	Duration  time.Duration
	Timestamp time.Time
}

func NewErrorRecovery() *ErrorRecovery {
	er := &ErrorRecovery{
		Strategies:  make(map[string]*RecoveryStrategy),
		History:     make([]RecoveryAttempt, 0, 64),
		MaxAttempts: 3,
	}
	er.registerBuiltins()
	return er
}

func (er *ErrorRecovery) registerBuiltins() {
	er.Strategies["file_not_found"] = &RecoveryStrategy{
		Name:         "file_not_found",
		ErrorPattern: regexp.MustCompile(`(?i)(no such file or directory|file not found|cannot find file|path.*does not exist|open .+: no such)`),
		Priority:     100,
		RecoverFn:    recoverFileNotFound,
	}

	er.Strategies["permission_denied"] = &RecoveryStrategy{
		Name:         "permission_denied",
		ErrorPattern: regexp.MustCompile(`(?i)(permission denied|access denied|operation not permitted)`),
		Priority:     90,
		RecoverFn:    recoverPermissionDenied,
	}

	er.Strategies["module_not_found"] = &RecoveryStrategy{
		Name:         "module_not_found",
		ErrorPattern: regexp.MustCompile(`(?i)(cannot find module|module not found|no required module|could not resolve|cannot resolve)`),
		Priority:     85,
		RecoverFn:    recoverModuleNotFound,
	}

	er.Strategies["port_in_use"] = &RecoveryStrategy{
		Name:         "port_in_use",
		ErrorPattern: regexp.MustCompile(`(?i)(address already in use|port.*already.*use|bind.*EADDRINUSE|listen tcp)`),
		Priority:     80,
		RecoverFn:    recoverPortInUse,
	}

	er.Strategies["out_of_memory"] = &RecoveryStrategy{
		Name:         "out_of_memory",
		ErrorPattern: regexp.MustCompile(`(?i)(out of memory|OOM|cannot allocate|memory limit|heap limit)`),
		Priority:     95,
		RecoverFn:    recoverOutOfMemory,
	}

	er.Strategies["timeout"] = &RecoveryStrategy{
		Name:         "timeout",
		ErrorPattern: regexp.MustCompile(`(?i)(timeout|timed out|context deadline exceeded|deadline exceeded)`),
		Priority:     75,
		RecoverFn:    recoverTimeout,
	}

	er.Strategies["rate_limited"] = &RecoveryStrategy{
		Name:         "rate_limited",
		ErrorPattern: regexp.MustCompile(`(?i)(rate limit|too many requests|429|throttled)`),
		Priority:     70,
		RecoverFn:    recoverRateLimited,
	}

	er.Strategies["syntax_error"] = &RecoveryStrategy{
		Name:         "syntax_error",
		ErrorPattern: regexp.MustCompile(`(?i)(syntax error|unexpected token|parse error|invalid syntax)`),
		Priority:     88,
		RecoverFn:    recoverSyntaxError,
	}

	er.Strategies["import_cycle"] = &RecoveryStrategy{
		Name:         "import_cycle",
		ErrorPattern: regexp.MustCompile(`(?i)(import cycle|circular dependency|cyclic import|circular import)`),
		Priority:     65,
		RecoverFn:    recoverImportCycle,
	}

	er.Strategies["merge_conflict"] = &RecoveryStrategy{
		Name:         "merge_conflict",
		ErrorPattern: regexp.MustCompile(`(?i)(merge conflict|CONFLICT|conflict marker|<<<<<<<)`),
		Priority:     60,
		RecoverFn:    recoverMergeConflict,
	}

	er.Strategies["git_dirty"] = &RecoveryStrategy{
		Name:         "git_dirty",
		ErrorPattern: regexp.MustCompile(`(?i)(uncommitted changes|working tree.*not clean|please commit.*stash|your local changes)`),
		Priority:     55,
		RecoverFn:    recoverGitDirty,
	}

	er.Strategies["build_failed"] = &RecoveryStrategy{
		Name:         "build_failed",
		ErrorPattern: regexp.MustCompile(`(?i)(build failed|compilation failed|compile error|FAILED.*build)`),
		Priority:     50,
		RecoverFn:    recoverBuildFailed,
	}

	er.Strategies["test_failed"] = &RecoveryStrategy{
		Name:         "test_failed",
		ErrorPattern: regexp.MustCompile(`(?i)(test failed|FAIL\s|tests? (failed|failing)|assertion failed)`),
		Priority:     45,
		RecoverFn:    recoverTestFailed,
	}
}

func (er *ErrorRecovery) Recover(err error, ctx *RecoveryContext) (*RecoveryResult, error) {
	er.mu.Lock()
	defer er.mu.Unlock()

	if err == nil {
		return nil, nil
	}

	if ctx == nil {
		ctx = &RecoveryContext{}
	}
	if ctx.ErrorMsg == "" && err != nil {
		ctx.ErrorMsg = err.Error()
	}
	if ctx.Error == nil {
		ctx.Error = err
	}

	errMsg := err.Error()

	var matched []*RecoveryStrategy
	for _, s := range er.Strategies {
		if s.ErrorPattern.MatchString(errMsg) {
			matched = append(matched, s)
		}
	}

	sort.Slice(matched, func(i, j int) bool {
		return matched[i].Priority > matched[j].Priority
	})

	for _, strategy := range matched {
		start := time.Now()
		result, recErr := strategy.RecoverFn(err, ctx)
		duration := time.Since(start)

		attempt := RecoveryAttempt{
			Error:     errMsg,
			Strategy:  strategy.Name,
			Recovered: result != nil && result.Recovered,
			Duration:  duration,
			Timestamp: time.Now(),
		}
		er.History = append(er.History, attempt)

		if recErr != nil {
			strategy.FailureCount++
			continue
		}

		if result != nil && result.Recovered {
			strategy.SuccessCount++
			return result, nil
		}

		strategy.FailureCount++
	}

	return nil, fmt.Errorf("no recovery strategy succeeded for: %s", errMsg)
}

func (er *ErrorRecovery) ShouldRetry(err error) bool {
	if err == nil {
		return false
	}
	er.mu.Lock()
	defer er.mu.Unlock()

	errMsg := err.Error()
	for _, s := range er.Strategies {
		if s.ErrorPattern.MatchString(errMsg) {
			return true
		}
	}
	return false
}

func (er *ErrorRecovery) BuildRecoveryPrompt(result *RecoveryResult) string {
	if result == nil {
		return ""
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Error encountered: %s\n", result.Message))
	sb.WriteString(fmt.Sprintf("Suggested recovery: %s\n", result.Action))
	if result.RetryWith != "" {
		sb.WriteString(fmt.Sprintf("Action: Retry with: %s\n", result.RetryWith))
	} else {
		sb.WriteString("Action: Follow the suggested recovery steps.\n")
	}
	return sb.String()
}

func (er *ErrorRecovery) FormatHistory(limit int) string {
	er.mu.Lock()
	defer er.mu.Unlock()

	if len(er.History) == 0 {
		return "No recovery attempts recorded."
	}

	start := 0
	if limit > 0 && limit < len(er.History) {
		start = len(er.History) - limit
	}

	var sb strings.Builder
	sb.WriteString("Recovery History:\n")
	for i := start; i < len(er.History); i++ {
		a := er.History[i]
		status := "FAILED"
		if a.Recovered {
			status = "OK"
		}
		sb.WriteString(fmt.Sprintf(
			"  [%s] %s | strategy=%s | duration=%s | error=%s\n",
			a.Timestamp.Format("15:04:05"),
			status,
			a.Strategy,
			a.Duration.Round(time.Millisecond),
			truncate(a.Error, 80),
		))
	}
	return sb.String()
}

func (er *ErrorRecovery) SuccessRate() float64 {
	er.mu.Lock()
	defer er.mu.Unlock()

	if len(er.History) == 0 {
		return 0.0
	}

	successes := 0
	for _, a := range er.History {
		if a.Recovered {
			successes++
		}
	}
	return float64(successes) / float64(len(er.History))
}

func (er *ErrorRecovery) RegisterStrategy(strategy *RecoveryStrategy) {
	if strategy == nil {
		return
	}
	er.mu.Lock()
	defer er.mu.Unlock()
	er.Strategies[strategy.Name] = strategy
}

func recoverFileNotFound(err error, ctx *RecoveryContext) (*RecoveryResult, error) {
	errMsg := ctx.ErrorMsg

	path := extractPath(errMsg)
	if path == "" {
		return &RecoveryResult{
			Recovered: true,
			Action:    "Check if the file path is correct. List the directory to find available files.",
			Message:   errMsg,
		}, nil
	}

	dir := filepath.Dir(path)
	base := filepath.Base(path)

	entries, dirErr := os.ReadDir(dir)
	if dirErr != nil {
		return &RecoveryResult{
			Recovered: true,
			Action:    fmt.Sprintf("Directory %q may not exist. Verify the full path.", dir),
			Message:   errMsg,
		}, nil
	}

	bestMatch := ""
	bestDist := len(base) + 1

	for _, entry := range entries {
		name := entry.Name()
		dist := levenshtein(base, name)
		if dist < bestDist {
			bestDist = dist
			bestMatch = name
		}
	}

	if bestMatch != "" && bestDist <= 3 {
		suggested := filepath.Join(dir, bestMatch)
		return &RecoveryResult{
			Recovered: true,
			Action:    fmt.Sprintf("Did you mean %q? (Levenshtein distance: %d)", suggested, bestDist),
			Message:   errMsg,
			RetryWith: suggested,
		}, nil
	}

	return &RecoveryResult{
		Recovered: true,
		Action:    fmt.Sprintf("File not found. List %q to see available files.", dir),
		Message:   errMsg,
	}, nil
}

func recoverPermissionDenied(_ error, ctx *RecoveryContext) (*RecoveryResult, error) {
	path := extractPath(ctx.ErrorMsg)
	action := "Check file permissions. Consider running: chmod +rw <file> or use elevated permissions."
	if path != "" {
		action = fmt.Sprintf("Permission denied on %q. Try: chmod +rw %s or run with elevated permissions.", path, path)
	}
	return &RecoveryResult{
		Recovered: true,
		Action:    action,
		Message:   ctx.ErrorMsg,
	}, nil
}

func recoverModuleNotFound(_ error, ctx *RecoveryContext) (*RecoveryResult, error) {
	errMsg := ctx.ErrorMsg
	action := "Run `go mod tidy` (Go) or `npm install` (Node.js) to install missing dependencies."

	if strings.Contains(errMsg, "go") || strings.Contains(errMsg, "module") {
		action = "Run `go mod tidy` to resolve missing Go module dependencies."
	} else if strings.Contains(errMsg, "npm") || strings.Contains(errMsg, "node_modules") || strings.Contains(errMsg, "require") {
		action = "Run `npm install` to install missing Node.js dependencies."
	}

	return &RecoveryResult{
		Recovered: true,
		Action:    action,
		Message:   errMsg,
		RetryWith: "go mod tidy",
	}, nil
}

func recoverPortInUse(_ error, ctx *RecoveryContext) (*RecoveryResult, error) {
	port := extractPort(ctx.ErrorMsg)
	action := "The port is already in use. Kill the process occupying it or use a different port."
	if port != "" {
		action = fmt.Sprintf("Port %s is in use. Run `lsof -i :%s` to find the process, then kill it, or use a different port.", port, port)
	}
	return &RecoveryResult{
		Recovered: true,
		Action:    action,
		Message:   ctx.ErrorMsg,
	}, nil
}

func recoverOutOfMemory(_ error, ctx *RecoveryContext) (*RecoveryResult, error) {
	return &RecoveryResult{
		Recovered: true,
		Action:    "Out of memory. Reduce batch size, compact the context, or increase available memory.",
		Message:   ctx.ErrorMsg,
	}, nil
}

func recoverTimeout(_ error, ctx *RecoveryContext) (*RecoveryResult, error) {
	return &RecoveryResult{
		Recovered: true,
		Action:    "Operation timed out. Increase the timeout duration or split the work into smaller chunks.",
		Message:   ctx.ErrorMsg,
	}, nil
}

func recoverRateLimited(_ error, ctx *RecoveryContext) (*RecoveryResult, error) {
	return &RecoveryResult{
		Recovered: true,
		Action:    "Rate limited. Wait before retrying or switch to an alternative provider.",
		Message:   ctx.ErrorMsg,
	}, nil
}

func recoverSyntaxError(_ error, ctx *RecoveryContext) (*RecoveryResult, error) {
	errMsg := ctx.ErrorMsg
	line := extractLineNumber(errMsg)
	file := extractPath(errMsg)

	action := "Syntax error detected. Re-read the file and fix the syntax."
	if file != "" && line != "" {
		action = fmt.Sprintf("Syntax error in %s at line %s. Re-read the file around that line and correct the syntax.", file, line)
	} else if line != "" {
		action = fmt.Sprintf("Syntax error at line %s. Re-read the file around that line and correct the syntax.", line)
	}

	return &RecoveryResult{
		Recovered: true,
		Action:    action,
		Message:   errMsg,
	}, nil
}

func recoverImportCycle(_ error, ctx *RecoveryContext) (*RecoveryResult, error) {
	return &RecoveryResult{
		Recovered: true,
		Action:    "Import cycle detected. Extract a shared interface into a separate package to break the dependency cycle.",
		Message:   ctx.ErrorMsg,
	}, nil
}

func recoverMergeConflict(_ error, ctx *RecoveryContext) (*RecoveryResult, error) {
	return &RecoveryResult{
		Recovered: true,
		Action:    "Merge conflict detected. Open the conflicting files, resolve the conflict markers (<<<<<<< / ======= / >>>>>>>), then stage and commit.",
		Message:   ctx.ErrorMsg,
	}, nil
}

func recoverGitDirty(_ error, ctx *RecoveryContext) (*RecoveryResult, error) {
	return &RecoveryResult{
		Recovered: true,
		Action:    "Working tree has uncommitted changes. Run `git stash` to save them temporarily, or `git commit` to persist them before proceeding.",
		Message:   ctx.ErrorMsg,
		RetryWith: "git stash",
	}, nil
}

func recoverBuildFailed(_ error, ctx *RecoveryContext) (*RecoveryResult, error) {
	return &RecoveryResult{
		Recovered: true,
		Action:    "Build failed. Read the error output carefully, identify the failing file and line, then fix the issue.",
		Message:   ctx.ErrorMsg,
	}, nil
}

func recoverTestFailed(_ error, ctx *RecoveryContext) (*RecoveryResult, error) {
	return &RecoveryResult{
		Recovered: true,
		Action:    "Tests failed. Read the test output to identify which test(s) failed and why, then fix the code or update the test expectations.",
		Message:   ctx.ErrorMsg,
	}, nil
}

func levenshtein(a, b string) int {
	if len(a) == 0 {
		return len(b)
	}
	if len(b) == 0 {
		return len(a)
	}

	prev := make([]int, len(b)+1)
	curr := make([]int, len(b)+1)

	for j := 0; j <= len(b); j++ {
		prev[j] = j
	}

	for i := 1; i <= len(a); i++ {
		curr[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			curr[j] = min3(
				curr[j-1]+1,
				prev[j]+1,
				prev[j-1]+cost,
			)
		}
		prev, curr = curr, prev
	}

	return prev[len(b)]
}

func min3(a, b, c int) int {
	if a < b {
		if a < c {
			return a
		}
		return c
	}
	if b < c {
		return b
	}
	return c
}

func extractPath(msg string) string {
	pathRe := regexp.MustCompile(`(?:["'\s]|^)([./]?(?:[a-zA-Z0-9_\-]+/)*[a-zA-Z0-9_\-]+\.[a-zA-Z0-9]+)`)
	matches := pathRe.FindStringSubmatch(msg)
	if len(matches) > 1 {
		return strings.TrimSpace(matches[1])
	}

	quotedRe := regexp.MustCompile(`["']([^"']+)["']`)
	matches = quotedRe.FindStringSubmatch(msg)
	if len(matches) > 1 {
		candidate := matches[1]
		if strings.Contains(candidate, "/") || strings.Contains(candidate, ".") {
			return candidate
		}
	}

	return ""
}

func extractPort(msg string) string {
	portRe := regexp.MustCompile(`:(\d{2,5})\b`)
	matches := portRe.FindStringSubmatch(msg)
	if len(matches) > 1 {
		return matches[1]
	}
	return ""
}

func extractLineNumber(msg string) string {
	lineRe := regexp.MustCompile(`(?:line\s*|:)(\d+)`)
	matches := lineRe.FindStringSubmatch(msg)
	if len(matches) > 1 {
		return matches[1]
	}
	return ""
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return s[:maxLen]
	}
	return s[:maxLen-3] + "..."
}
