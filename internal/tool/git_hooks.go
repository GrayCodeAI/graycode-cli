package tool

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/GrayCodeAI/hawk/internal/ui/icons"
)

// GitHookInstaller manages installation and lifecycle of git hooks for
// hawk agent integration.
type GitHookInstaller struct {
	HooksDir  string
	Installed map[string]bool
	mu        sync.Mutex
}

// HookConfig describes a single git hook to install.
type HookConfig struct {
	Name     string // "pre-commit", "post-commit", "prepare-commit-msg", "pre-push"
	Script   string
	Enabled  bool
	Priority int
}

func validHookName(name string) bool {
	switch name {
	case "pre-commit", "prepare-commit-msg", "post-commit", "pre-push":
		return true
	default:
		return false
	}
}

// NewGitHookInstaller creates a new installer rooted at the given project directory.
// It resolves .git/hooks relative to projectDir and probes which hooks are already
// installed.
func NewGitHookInstaller(projectDir string) *GitHookInstaller {
	hooksDir := filepath.Join(projectDir, ".git", "hooks")
	installer := &GitHookInstaller{
		HooksDir:  hooksDir,
		Installed: make(map[string]bool),
	}

	// Probe existing hawk-managed hooks.
	for _, name := range []string{"pre-commit", "post-commit", "prepare-commit-msg", "pre-push"} {
		hookPath := filepath.Join(hooksDir, name)
		data, err := readPinnedFile(hookPath)
		if err == nil && strings.Contains(string(data), "# hawk-managed") {
			installer.Installed[name] = true
		}
	}

	return installer
}

// Install writes a hook script to .git/hooks/, makes it executable, and
// preserves any existing hook by chaining it.
func (g *GitHookInstaller) Install(hook HookConfig) error {
	g.mu.Lock()
	defer g.mu.Unlock()

	if !hook.Enabled {
		return nil
	}
	if !validHookName(hook.Name) {
		return fmt.Errorf("unsupported git hook name %q", hook.Name)
	}

	if err := os.MkdirAll(g.HooksDir, 0o750); err != nil {
		return fmt.Errorf("create hooks dir: %w", err)
	}

	hookPath := filepath.Join(g.HooksDir, hook.Name)

	// Preserve existing hook if present and not hawk-managed.
	existing, err := readPinnedFile(hookPath)
	if err == nil && !strings.Contains(string(existing), "# hawk-managed") {
		// Back up and chain existing hook.
		if backupErr := g.backupExisting(hook.Name); backupErr != nil {
			return fmt.Errorf("backup existing hook: %w", backupErr)
		}
		// Chain: run existing hook first, then hawk hook.
		script := hook.Script + "\n\n# --- chained previous hook ---\n" + string(existing)
		hook.Script = script
	}

	// #nosec G306 -- git hook must be executable by git
	if err := writePinnedFile(hookPath, []byte(hook.Script), 0o755); err != nil {
		return fmt.Errorf("write hook %s: %w", hook.Name, err)
	}

	g.Installed[hook.Name] = true
	return nil
}

// Uninstall removes a hawk-managed hook. If a backup exists it is restored.
func (g *GitHookInstaller) Uninstall(hookName string) error {
	g.mu.Lock()
	defer g.mu.Unlock()

	if !validHookName(hookName) {
		return fmt.Errorf("unsupported git hook name %q", hookName)
	}
	hookPath := filepath.Join(g.HooksDir, hookName)
	backupPath := hookPath + ".bak"

	if err := os.Remove(hookPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove hook %s: %w", hookName, err)
	}

	// Restore backup if one exists.
	if _, err := os.Stat(backupPath); err == nil {
		if restoreErr := os.Rename(backupPath, hookPath); restoreErr != nil {
			return fmt.Errorf("restore backup for %s: %w", hookName, restoreErr)
		}
	}

	delete(g.Installed, hookName)
	return nil
}

// InstallAll installs the default set of hawk hooks for agent integration.
func (g *GitHookInstaller) InstallAll() error {
	hooks := []HookConfig{
		{Name: "pre-commit", Script: g.GeneratePreCommit(), Enabled: true, Priority: 1},
		{Name: "prepare-commit-msg", Script: g.GeneratePrepareCommitMsg(), Enabled: true, Priority: 2},
		{Name: "post-commit", Script: g.GeneratePostCommit(), Enabled: true, Priority: 3},
		{Name: "pre-push", Script: g.GeneratePrePush(), Enabled: true, Priority: 4},
	}

	// Sort by priority.
	sort.Slice(hooks, func(i, j int) bool {
		return hooks[i].Priority < hooks[j].Priority
	})

	for _, hook := range hooks {
		if err := g.Install(hook); err != nil {
			return fmt.Errorf("install %s: %w", hook.Name, err)
		}
	}
	return nil
}

// GeneratePreCommit returns a shell script that runs format check, lint, and
// secret scan before allowing a commit.
func (g *GitHookInstaller) GeneratePreCommit() string {
	return `#!/bin/sh
# hawk-managed: pre-commit hook
# Runs format check, lint, and secret scan.

set -e

echo "[hawk] Running pre-commit checks..."

# Format check
if command -v gofmt >/dev/null 2>&1; then
    UNFORMATTED=$(gofmt -l . 2>/dev/null || true)
    if [ -n "$UNFORMATTED" ]; then
        echo "[hawk] ERROR: Unformatted files detected:"
        echo "$UNFORMATTED"
        exit 1
    fi
fi

# Lint
if command -v golangci-lint >/dev/null 2>&1; then
    golangci-lint run --fast ./... || exit 1
fi

# Secret scan
if command -v hawk >/dev/null 2>&1; then
    hawk scan --secrets --staged || exit 1
fi

echo "[hawk] Pre-commit checks passed."
`
}

// GeneratePrepareCommitMsg returns a shell script that invokes hawk to generate
// an AI-powered commit message.
func (g *GitHookInstaller) GeneratePrepareCommitMsg() string {
	return `#!/bin/sh
# hawk-managed: prepare-commit-msg hook
# Calls hawk to generate an AI-assisted commit message.

COMMIT_MSG_FILE="$1"
COMMIT_SOURCE="$2"

# Only generate for normal commits (not merge, squash, etc.)
if [ -z "$COMMIT_SOURCE" ]; then
    if command -v hawk >/dev/null 2>&1; then
        GENERATED=$(hawk commit-msg --staged 2>/dev/null || true)
        if [ -n "$GENERATED" ]; then
            printf '%s\n\n%s' "$GENERATED" "$(cat "$COMMIT_MSG_FILE")" > "$COMMIT_MSG_FILE"
        fi
    fi
fi
`
}

// GeneratePostCommit returns a shell script that notifies the hawk swift system
// for session capture after a commit.
func (g *GitHookInstaller) GeneratePostCommit() string {
	return `#!/bin/sh
# hawk-managed: post-commit hook
# Notifies hawk swift for session capture.

if command -v hawk >/dev/null 2>&1; then
    COMMIT_HASH=$(git rev-parse HEAD 2>/dev/null)
    COMMIT_MSG=$(git log -1 --format='%s' 2>/dev/null)
    hawk swift --event post-commit --hash "$COMMIT_HASH" --message "$COMMIT_MSG" 2>/dev/null &
fi
`
}

// GeneratePrePush returns a shell script that runs the test suite before
// allowing a push.
func (g *GitHookInstaller) GeneratePrePush() string {
	return `#!/bin/sh
# hawk-managed: pre-push hook
# Runs the test suite before push.

set -e

echo "[hawk] Running pre-push tests..."

if [ -f "go.mod" ]; then
    go test -race -short ./... || exit 1
elif [ -f "package.json" ]; then
    npm test || exit 1
elif [ -f "Makefile" ]; then
    make test || exit 1
fi

echo "[hawk] Pre-push tests passed."
`
}

// ListInstalled returns the names of all currently installed hawk-managed hooks.
func (g *GitHookInstaller) ListInstalled() []string {
	g.mu.Lock()
	defer g.mu.Unlock()

	var names []string
	for name := range g.Installed {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// IsInstalled reports whether a hawk-managed hook is installed for the given name.
func (g *GitHookInstaller) IsInstalled(hookName string) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.Installed[hookName]
}

// BackupExisting saves the existing hook as .bak before overwriting.
func (g *GitHookInstaller) BackupExisting(hookName string) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.backupExisting(hookName)
}

// backupExisting is the internal (unlocked) implementation.
func (g *GitHookInstaller) backupExisting(hookName string) error {
	if !validHookName(hookName) {
		return fmt.Errorf("unsupported git hook name %q", hookName)
	}
	hookPath := filepath.Join(g.HooksDir, hookName)
	backupPath := hookPath + ".bak"

	data, err := readPinnedFile(hookPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // nothing to back up
		}
		return fmt.Errorf("read hook %s: %w", hookName, err)
	}

	// #nosec G306 -- backup preserves executable hook script
	if err := writePinnedFile(backupPath, data, 0o755); err != nil {
		return fmt.Errorf("write backup %s: %w", hookName, err)
	}
	return nil
}

// FormatStatus returns a human-readable summary of installed/missing hooks.
func (g *GitHookInstaller) FormatStatus() string {
	g.mu.Lock()
	defer g.mu.Unlock()

	type hookInfo struct {
		name string
		desc string
	}

	hooks := []hookInfo{
		{"pre-commit", "lint + format + secrets"},
		{"prepare-commit-msg", "AI message"},
		{"post-commit", "session trace"},
		{"pre-push", "test suite"},
	}

	var b strings.Builder
	b.WriteString("Git Hooks:\n")
	b.WriteString("──────────────\n")

	for _, h := range hooks {
		if g.Installed[h.name] {
			_, _ = fmt.Fprintf(&b, "  "+icons.CheckBold()+" %s (%s)\n", h.name, h.desc)
		} else {
			_, _ = fmt.Fprintf(&b, "  "+icons.CloseThick()+" %s (not installed)\n", h.name)
		}
	}

	return b.String()
}
