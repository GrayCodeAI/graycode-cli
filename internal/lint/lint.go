// Package lint provides a per-language linter/auto-fix cycle that can be
// invoked after hawk writes or edits a file. Linters are looked up by
// language (derived from the file extension) and run against a single file.
// A non-zero linter exit surfaces the captured output so an agent can
// auto-fix the reported issues.
//
// The package is intentionally self-contained: it shells out to well-known
// per-language tools (go vet/gofmt, eslint, ruff) and also supports custom
// linters supplied declaratively via Config.Custom.
package lint

import (
	"context"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// defaultTimeout bounds how long a single linter invocation may run.
const defaultTimeout = 60 * time.Second

// Result is the outcome of running a linter against a file.
type Result struct {
	// Language is the resolved language key (e.g. "go", "js", "python").
	Language string
	// Linter is a human-readable name of the linter that ran.
	Linter string
	// Output is the combined stdout/stderr of the linter, trimmed.
	Output string
	// OK reports whether the file passed (linter exited zero / produced no
	// actionable findings). When false, Output carries the findings.
	OK bool
	// Ran reports whether any linter was actually executed. It is false when
	// no linter is configured for the language or the underlying tool is not
	// installed.
	Ran bool
}

// Linter runs against a single file and returns a Result. Implementations
// must be safe to call concurrently.
type Linter interface {
	// Name returns a human-readable linter name.
	Name() string
	// Lint runs the linter against file and reports the result.
	Lint(ctx context.Context, file string) Result
}

// Config controls which linters run. The zero value disables linting, which
// keeps the post-write hook off by default so users are not surprised.
type Config struct {
	// Enabled gates the whole feature. When false, RunLint is a no-op.
	Enabled bool
	// Custom maps a language key to a shell command template. The token
	// "{file}" in the command is replaced with the absolute file path. If no
	// "{file}" token is present, the file path is appended as a final argument.
	// Custom entries take precedence over the built-in linter for a language.
	Custom map[string]string
	// Timeout overrides defaultTimeout when non-zero.
	Timeout time.Duration
}

// LanguageForExt maps a file extension (including the leading dot) to a
// canonical language key. Returns "" when the extension is unknown.
func LanguageForExt(ext string) string {
	switch strings.ToLower(ext) {
	case ".go":
		return "go"
	case ".js", ".jsx", ".mjs", ".cjs":
		return "js"
	case ".ts", ".tsx":
		return "ts"
	case ".py":
		return "python"
	default:
		return ""
	}
}

// LanguageForFile resolves the language key for a file path.
func LanguageForFile(file string) string {
	return LanguageForExt(filepath.Ext(file))
}

// LinterFor returns the Linter to use for the given language under cfg, or
// nil when no linter is configured. Custom commands win over built-ins.
func LinterFor(lang string, cfg Config) Linter {
	if lang == "" {
		return nil
	}
	if cfg.Custom != nil {
		if cmd, ok := cfg.Custom[lang]; ok && strings.TrimSpace(cmd) != "" {
			return &customLinter{lang: lang, command: cmd}
		}
	}
	switch lang {
	case "go":
		return goLinter{}
	case "js", "ts":
		return eslintLinter{}
	case "python":
		return ruffLinter{}
	default:
		return nil
	}
}

// RunLint resolves and runs the linter for file under cfg. When linting is
// disabled or no linter is configured/installed, it returns a Result with
// Ran=false and OK=true so callers can treat it as a no-op.
func RunLint(ctx context.Context, file string, cfg Config) Result {
	if !cfg.Enabled {
		return Result{OK: true}
	}
	lang := LanguageForFile(file)
	linter := LinterFor(lang, cfg)
	if linter == nil {
		return Result{Language: lang, OK: true}
	}
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	rctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	res := linter.Lint(rctx, file)
	res.Language = lang
	return res
}

// toolAvailable reports whether the named executable is on PATH.
func toolAvailable(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

// runCmd executes cmd, returning trimmed combined output and the exit error.
func runCmd(cmd *exec.Cmd) (string, error) {
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}
