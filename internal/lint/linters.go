package lint

import (
	"context"
	"os/exec"
	"path/filepath"
	"strings"
)

// goLinter runs gofmt -l (format check) and go vet against a single file's
// package. A non-empty gofmt list or a non-zero vet exit is a failure.
type goLinter struct{}

func (goLinter) Name() string { return "go vet/gofmt" }

func (goLinter) Lint(ctx context.Context, file string) Result {
	if !toolAvailable("go") && !toolAvailable("gofmt") {
		return Result{Linter: "go vet/gofmt", OK: true}
	}
	var findings []string

	// gofmt -l prints the filename when it is NOT properly formatted.
	if toolAvailable("gofmt") {
		fmtOut, _ := runCmd(exec.CommandContext(ctx, "gofmt", "-l", file)) // #nosec G204 -- fixed formatter executable
		if fmtOut != "" {
			findings = append(findings, "gofmt: needs formatting:\n"+fmtOut)
		}
	}

	// go vet runs against the package directory containing the file.
	if toolAvailable("go") {
		cmd := exec.CommandContext(ctx, "go", "vet", file) // #nosec G204 -- fixed Go executable
		cmd.Dir = filepath.Dir(file)
		vetOut, err := runCmd(cmd)
		if err != nil || vetOut != "" {
			if vetOut != "" {
				findings = append(findings, vetOut)
			} else if err != nil {
				findings = append(findings, "go vet: "+err.Error())
			}
		}
	}

	if len(findings) == 0 {
		return Result{Linter: "go vet/gofmt", Output: "", OK: true, Ran: true}
	}
	return Result{Linter: "go vet/gofmt", Output: strings.Join(findings, "\n"), OK: false, Ran: true}
}

// eslintLinter runs eslint (via npx) against a single JS/TS file.
type eslintLinter struct{}

func (eslintLinter) Name() string { return "eslint" }

func (eslintLinter) Lint(ctx context.Context, file string) Result {
	if !toolAvailable("npx") && !toolAvailable("eslint") {
		return Result{Linter: "eslint", OK: true}
	}
	var cmd *exec.Cmd
	if toolAvailable("eslint") {
		cmd = exec.CommandContext(ctx, "eslint", "--format", "compact", file) // #nosec G204 -- fixed linter executable
	} else {
		cmd = exec.CommandContext(ctx, "npx", "--no-install", "eslint", "--format", "compact", file) // #nosec G204 -- fixed linter executable
	}
	cmd.Dir = filepath.Dir(file)
	out, err := runCmd(cmd)
	if err == nil && out == "" {
		return Result{Linter: "eslint", OK: true, Ran: true}
	}
	if out == "" && err != nil {
		// eslint not actually available (npx --no-install miss) — treat as no-op.
		return Result{Linter: "eslint", OK: true}
	}
	return Result{Linter: "eslint", Output: out, OK: false, Ran: true}
}

// ruffLinter runs ruff against a single Python file.
type ruffLinter struct{}

func (ruffLinter) Name() string { return "ruff" }

func (ruffLinter) Lint(ctx context.Context, file string) Result {
	if !toolAvailable("ruff") {
		return Result{Linter: "ruff", OK: true}
	}
	cmd := exec.CommandContext(ctx, "ruff", "check", file) // #nosec G204 -- fixed linter executable
	cmd.Dir = filepath.Dir(file)
	out, err := runCmd(cmd)
	if err == nil && out == "" {
		return Result{Linter: "ruff", OK: true, Ran: true}
	}
	return Result{Linter: "ruff", Output: out, OK: false, Ran: true}
}

// customLinter runs a user-supplied shell command. The "{file}" token in the
// command is replaced with the file path; if absent, the file is appended.
type customLinter struct {
	lang    string
	command string
}

func (c *customLinter) Name() string { return "custom:" + c.lang }

func (c *customLinter) Lint(ctx context.Context, file string) Result {
	command := c.command
	if strings.Contains(command, "{file}") {
		command = strings.ReplaceAll(command, "{file}", file)
	} else {
		command = command + " " + file
	}
	cmd := exec.CommandContext(ctx, "sh", "-c", command) // #nosec G204 -- command from user-supplied --lint config, not external/untrusted input
	cmd.Dir = filepath.Dir(file)
	out, err := runCmd(cmd)
	if err == nil {
		return Result{Linter: c.Name(), Output: out, OK: true, Ran: true}
	}
	if out == "" {
		out = err.Error()
	}
	return Result{Linter: c.Name(), Output: out, OK: false, Ran: true}
}

// ParseCustomFlag parses a "--lint 'lang: cmd'" style value into a (lang, cmd)
// pair. Returns ok=false when the value is malformed (missing colon or empty
// lang/cmd). Exposed so the CLI flag layer can populate Config.Custom.
func ParseCustomFlag(value string) (lang, cmd string, ok bool) {
	idx := strings.Index(value, ":")
	if idx < 0 {
		return "", "", false
	}
	lang = strings.TrimSpace(value[:idx])
	cmd = strings.TrimSpace(value[idx+1:])
	if lang == "" || cmd == "" {
		return "", "", false
	}
	return lang, cmd, true
}
