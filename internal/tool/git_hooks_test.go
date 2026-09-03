package tool

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/GrayCodeAI/graycode-cli/internal/ui/icons"
)

func setupHooksTestDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	hooksDir := filepath.Join(dir, ".git", "hooks")
	if err := os.MkdirAll(hooksDir, 0o755); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestNewGitHookInstaller(t *testing.T) {
	dir := setupHooksTestDir(t)
	installer := NewGitHookInstaller(dir)

	if installer.HooksDir != filepath.Join(dir, ".git", "hooks") {
		t.Errorf("HooksDir = %q, want %q", installer.HooksDir, filepath.Join(dir, ".git", "hooks"))
	}
	if len(installer.Installed) != 0 {
		t.Errorf("expected no installed hooks, got %d", len(installer.Installed))
	}
}

func TestNewGitHookInstaller_DetectsExisting(t *testing.T) {
	dir := setupHooksTestDir(t)
	hookPath := filepath.Join(dir, ".git", "hooks", "pre-commit")
	content := "#!/bin/sh\n# graycode-managed: pre-commit hook\necho hello\n"
	if err := os.WriteFile(hookPath, []byte(content), 0o755); err != nil {
		t.Fatal(err)
	}

	installer := NewGitHookInstaller(dir)
	if !installer.Installed["pre-commit"] {
		t.Error("expected pre-commit to be detected as installed")
	}
}

func TestInstall_Basic(t *testing.T) {
	dir := setupHooksTestDir(t)
	installer := NewGitHookInstaller(dir)

	hook := HookConfig{
		Name:     "pre-commit",
		Script:   "#!/bin/sh\n# graycode-managed: pre-commit hook\necho test\n",
		Enabled:  true,
		Priority: 1,
	}

	if err := installer.Install(hook); err != nil {
		t.Fatalf("Install: %v", err)
	}

	// Verify file was written.
	hookPath := filepath.Join(dir, ".git", "hooks", "pre-commit")
	data, err := os.ReadFile(hookPath)
	if err != nil {
		t.Fatalf("read hook: %v", err)
	}
	if !strings.Contains(string(data), "graycode-managed") {
		t.Error("hook file does not contain graycode-managed marker")
	}

	// Verify permissions.
	info, err := os.Stat(hookPath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm()&0o111 == 0 {
		t.Error("hook file is not executable")
	}

	// Verify tracked.
	if !installer.Installed["pre-commit"] {
		t.Error("hook not tracked as installed")
	}
}

func TestInstall_DisabledHook(t *testing.T) {
	dir := setupHooksTestDir(t)
	installer := NewGitHookInstaller(dir)

	hook := HookConfig{
		Name:    "pre-commit",
		Script:  "#!/bin/sh\necho test\n",
		Enabled: false,
	}

	if err := installer.Install(hook); err != nil {
		t.Fatalf("Install: %v", err)
	}

	hookPath := filepath.Join(dir, ".git", "hooks", "pre-commit")
	if _, err := os.Stat(hookPath); !os.IsNotExist(err) {
		t.Error("disabled hook should not be written")
	}
}

func TestInstall_PreservesExisting(t *testing.T) {
	dir := setupHooksTestDir(t)
	hookPath := filepath.Join(dir, ".git", "hooks", "pre-commit")

	// Write existing non-graycode hook.
	existing := "#!/bin/sh\necho existing\n"
	if err := os.WriteFile(hookPath, []byte(existing), 0o755); err != nil {
		t.Fatal(err)
	}

	installer := NewGitHookInstaller(dir)
	hook := HookConfig{
		Name:     "pre-commit",
		Script:   "#!/bin/sh\n# graycode-managed: pre-commit hook\necho graycode\n",
		Enabled:  true,
		Priority: 1,
	}

	if err := installer.Install(hook); err != nil {
		t.Fatalf("Install: %v", err)
	}

	// Verify backup was created.
	backupPath := hookPath + ".bak"
	backupData, err := os.ReadFile(backupPath)
	if err != nil {
		t.Fatal("backup file not created")
	}
	if string(backupData) != existing {
		t.Errorf("backup content = %q, want %q", string(backupData), existing)
	}

	// Verify chained content.
	data, err := os.ReadFile(hookPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "graycode-managed") {
		t.Error("hook does not contain graycode-managed marker")
	}
	if !strings.Contains(string(data), "echo existing") {
		t.Error("hook does not chain existing script")
	}
}

func TestUninstall(t *testing.T) {
	dir := setupHooksTestDir(t)
	installer := NewGitHookInstaller(dir)

	hook := HookConfig{
		Name:     "pre-commit",
		Script:   "#!/bin/sh\n# graycode-managed: pre-commit hook\necho test\n",
		Enabled:  true,
		Priority: 1,
	}
	if err := installer.Install(hook); err != nil {
		t.Fatal(err)
	}

	if err := installer.Uninstall("pre-commit"); err != nil {
		t.Fatalf("Uninstall: %v", err)
	}

	hookPath := filepath.Join(dir, ".git", "hooks", "pre-commit")
	if _, err := os.Stat(hookPath); !os.IsNotExist(err) {
		t.Error("hook file should be removed after uninstall")
	}
	if installer.IsInstalled("pre-commit") {
		t.Error("hook still tracked as installed after uninstall")
	}
}

func TestUninstall_RestoresBackup(t *testing.T) {
	dir := setupHooksTestDir(t)
	hookPath := filepath.Join(dir, ".git", "hooks", "pre-commit")

	// Write existing non-graycode hook.
	existing := "#!/bin/sh\necho original\n"
	if err := os.WriteFile(hookPath, []byte(existing), 0o755); err != nil {
		t.Fatal(err)
	}

	installer := NewGitHookInstaller(dir)
	hook := HookConfig{
		Name:     "pre-commit",
		Script:   "#!/bin/sh\n# graycode-managed: pre-commit hook\necho graycode\n",
		Enabled:  true,
		Priority: 1,
	}
	if err := installer.Install(hook); err != nil {
		t.Fatal(err)
	}

	if err := installer.Uninstall("pre-commit"); err != nil {
		t.Fatal(err)
	}

	// Original hook should be restored.
	data, err := os.ReadFile(hookPath)
	if err != nil {
		t.Fatal("original hook not restored")
	}
	if string(data) != existing {
		t.Errorf("restored content = %q, want %q", string(data), existing)
	}
}

func TestInstallAll(t *testing.T) {
	dir := setupHooksTestDir(t)
	installer := NewGitHookInstaller(dir)

	if err := installer.InstallAll(); err != nil {
		t.Fatalf("InstallAll: %v", err)
	}

	expected := []string{"pre-commit", "prepare-commit-msg", "post-commit", "pre-push"}
	for _, name := range expected {
		if !installer.IsInstalled(name) {
			t.Errorf("hook %s not installed", name)
		}
		hookPath := filepath.Join(dir, ".git", "hooks", name)
		data, err := os.ReadFile(hookPath)
		if err != nil {
			t.Errorf("cannot read %s: %v", name, err)
			continue
		}
		if !strings.Contains(string(data), "# graycode-managed") {
			t.Errorf("hook %s missing graycode-managed marker", name)
		}
	}
}

func TestGeneratePreCommit(t *testing.T) {
	installer := &GitHookInstaller{}
	script := installer.GeneratePreCommit()

	if !strings.HasPrefix(script, "#!/bin/sh") {
		t.Error("script must start with shebang")
	}
	if !strings.Contains(script, "graycode-managed") {
		t.Error("missing graycode-managed marker")
	}
	if !strings.Contains(script, "gofmt") {
		t.Error("missing format check")
	}
	if !strings.Contains(script, "golangci-lint") {
		t.Error("missing lint step")
	}
	if !strings.Contains(script, "scan --secrets") {
		t.Error("missing secret scan")
	}
}

func TestGeneratePrepareCommitMsg(t *testing.T) {
	installer := &GitHookInstaller{}
	script := installer.GeneratePrepareCommitMsg()

	if !strings.HasPrefix(script, "#!/bin/sh") {
		t.Error("script must start with shebang")
	}
	if !strings.Contains(script, "graycode-managed") {
		t.Error("missing graycode-managed marker")
	}
	if !strings.Contains(script, "graycode commit-msg") {
		t.Error("missing graycode commit-msg invocation")
	}
	if !strings.Contains(script, "COMMIT_MSG_FILE") {
		t.Error("missing COMMIT_MSG_FILE handling")
	}
}

func TestGeneratePostCommit(t *testing.T) {
	installer := &GitHookInstaller{}
	script := installer.GeneratePostCommit()

	if !strings.HasPrefix(script, "#!/bin/sh") {
		t.Error("script must start with shebang")
	}
	if !strings.Contains(script, "graycode-managed") {
		t.Error("missing graycode-managed marker")
	}
	if !strings.Contains(script, "graycode swift") {
		t.Error("missing trace notification")
	}
	if !strings.Contains(script, "post-commit") {
		t.Error("missing post-commit event type")
	}
}

func TestGeneratePrePush(t *testing.T) {
	installer := &GitHookInstaller{}
	script := installer.GeneratePrePush()

	if !strings.HasPrefix(script, "#!/bin/sh") {
		t.Error("script must start with shebang")
	}
	if !strings.Contains(script, "graycode-managed") {
		t.Error("missing graycode-managed marker")
	}
	if !strings.Contains(script, "go test") {
		t.Error("missing go test")
	}
	if !strings.Contains(script, "npm test") {
		t.Error("missing npm test fallback")
	}
}

func TestListInstalled(t *testing.T) {
	dir := setupHooksTestDir(t)
	installer := NewGitHookInstaller(dir)

	if got := installer.ListInstalled(); len(got) != 0 {
		t.Errorf("expected empty list, got %v", got)
	}

	installer.Install(HookConfig{Name: "pre-commit", Script: "#!/bin/sh\n# graycode-managed\n", Enabled: true})
	installer.Install(HookConfig{Name: "pre-push", Script: "#!/bin/sh\n# graycode-managed\n", Enabled: true})

	got := installer.ListInstalled()
	if len(got) != 2 {
		t.Fatalf("expected 2 installed, got %d", len(got))
	}
	// ListInstalled returns sorted.
	if got[0] != "pre-commit" || got[1] != "pre-push" {
		t.Errorf("unexpected order: %v", got)
	}
}

func TestIsInstalled(t *testing.T) {
	dir := setupHooksTestDir(t)
	installer := NewGitHookInstaller(dir)

	if installer.IsInstalled("pre-commit") {
		t.Error("should not be installed initially")
	}

	installer.Install(HookConfig{Name: "pre-commit", Script: "#!/bin/sh\n# graycode-managed\n", Enabled: true})

	if !installer.IsInstalled("pre-commit") {
		t.Error("should be installed after Install")
	}
}

func TestBackupExisting(t *testing.T) {
	dir := setupHooksTestDir(t)
	hookPath := filepath.Join(dir, ".git", "hooks", "pre-commit")
	content := "#!/bin/sh\necho original\n"
	if err := os.WriteFile(hookPath, []byte(content), 0o755); err != nil {
		t.Fatal(err)
	}

	installer := NewGitHookInstaller(dir)
	if err := installer.BackupExisting("pre-commit"); err != nil {
		t.Fatalf("BackupExisting: %v", err)
	}

	backupPath := hookPath + ".bak"
	data, err := os.ReadFile(backupPath)
	if err != nil {
		t.Fatal("backup not created")
	}
	if string(data) != content {
		t.Errorf("backup content = %q, want %q", string(data), content)
	}
}

func TestBackupExisting_NoFile(t *testing.T) {
	dir := setupHooksTestDir(t)
	installer := NewGitHookInstaller(dir)

	// Should not error when there's nothing to back up.
	if err := installer.BackupExisting("pre-commit"); err != nil {
		t.Fatalf("BackupExisting with no file: %v", err)
	}
}

func TestFormatStatus(t *testing.T) {
	dir := setupHooksTestDir(t)
	installer := NewGitHookInstaller(dir)

	// Install only two hooks.
	installer.Install(HookConfig{Name: "pre-commit", Script: "#!/bin/sh\n# graycode-managed\n", Enabled: true})
	installer.Install(HookConfig{Name: "prepare-commit-msg", Script: "#!/bin/sh\n# graycode-managed\n", Enabled: true})

	status := installer.FormatStatus()

	if !strings.Contains(status, "Git Hooks:") {
		t.Error("missing header")
	}
	if !strings.Contains(status, icons.CheckBold()+" pre-commit") {
		t.Error("pre-commit should show as installed")
	}
	if !strings.Contains(status, icons.CheckBold()+" prepare-commit-msg") {
		t.Error("prepare-commit-msg should show as installed")
	}
	if !strings.Contains(status, icons.CloseThick()+" post-commit (not installed)") {
		t.Error("post-commit should show as not installed")
	}
	if !strings.Contains(status, icons.CloseThick()+" pre-push (not installed)") {
		t.Error("pre-push should show as not installed")
	}
}

func TestConcurrentInstall(t *testing.T) {
	dir := setupHooksTestDir(t)
	installer := NewGitHookInstaller(dir)

	var wg sync.WaitGroup
	hooks := []string{"pre-commit", "post-commit", "prepare-commit-msg", "pre-push"}

	for _, name := range hooks {
		wg.Add(1)
		go func(n string) {
			defer wg.Done()
			hook := HookConfig{
				Name:    n,
				Script:  "#!/bin/sh\n# graycode-managed: " + n + "\necho " + n + "\n",
				Enabled: true,
			}
			if err := installer.Install(hook); err != nil {
				t.Errorf("concurrent Install(%s): %v", n, err)
			}
		}(name)
	}
	wg.Wait()

	for _, name := range hooks {
		if !installer.IsInstalled(name) {
			t.Errorf("hook %s not installed after concurrent install", name)
		}
	}
}
