package tool

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ──────────────────────────────────────────────────────────────────────────────
// 1. Bash -> Read chain: bash creates file, read reads it
// ──────────────────────────────────────────────────────────────────────────────

func TestIntegration_BashThenRead(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "created_by_bash.txt")

	// Bash creates a file.
	bashInput, _ := json.Marshal(map[string]string{
		"command": "echo -n 'hello from bash' > " + filePath,
	})
	bashOut, err := BashTool{}.Execute(context.Background(), bashInput)
	if err != nil {
		t.Fatalf("Bash execute error: %v (output: %s)", err, bashOut)
	}

	// Verify the file exists on disk.
	if _, err := os.Stat(filePath); err != nil {
		t.Fatalf("expected file to exist after bash: %v", err)
	}

	// Read reads it back.
	readInput, _ := json.Marshal(map[string]string{"path": filePath})
	readOut, err := FileReadTool{}.Execute(context.Background(), readInput)
	if err != nil {
		t.Fatalf("Read execute error: %v", err)
	}
	if readOut != "hello from bash" {
		t.Fatalf("expected 'hello from bash', got %q", readOut)
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// 2. Edit -> Read verification: edit file, read confirms change
// ──────────────────────────────────────────────────────────────────────────────

func TestIntegration_EditThenRead(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "editable.txt")

	// Create initial file.
	if err := os.WriteFile(filePath, []byte("The quick brown fox jumps over the lazy dog"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Edit replaces "brown fox" with "red hawk".
	editInput, _ := json.Marshal(map[string]string{
		"path":    filePath,
		"old_str": "brown fox",
		"new_str": "red hawk",
	})
	editOut, err := FileEditTool{}.Execute(context.Background(), editInput)
	if err != nil {
		t.Fatalf("Edit error: %v", err)
	}
	if !strings.Contains(editOut, "Edited") {
		t.Fatalf("expected edit confirmation, got %q", editOut)
	}

	// Read confirms the change.
	readInput, _ := json.Marshal(map[string]string{"path": filePath})
	readOut, err := FileReadTool{}.Execute(context.Background(), readInput)
	if err != nil {
		t.Fatalf("Read error: %v", err)
	}
	if readOut != "The quick red hawk jumps over the lazy dog" {
		t.Fatalf("expected edited content, got %q", readOut)
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// 3. Glob -> Read chain: glob finds files, read them
// ──────────────────────────────────────────────────────────────────────────────

func TestIntegration_GlobThenRead(t *testing.T) {
	dir := t.TempDir()

	// Create several files.
	files := map[string]string{
		"alpha.go":    "package alpha",
		"beta.go":     "package beta",
		"gamma.txt":   "not a go file",
		"delta.go":    "package delta",
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	// Glob finds *.go files.
	globInput, _ := json.Marshal(map[string]interface{}{
		"pattern": "*.go",
		"path":    dir,
	})
	globOut, err := GlobTool{}.Execute(context.Background(), globInput)
	if err != nil {
		t.Fatalf("Glob error: %v", err)
	}
	if !strings.Contains(globOut, "alpha.go") {
		t.Fatalf("expected alpha.go in glob output: %s", globOut)
	}
	if !strings.Contains(globOut, "beta.go") {
		t.Fatalf("expected beta.go in glob output: %s", globOut)
	}
	if !strings.Contains(globOut, "delta.go") {
		t.Fatalf("expected delta.go in glob output: %s", globOut)
	}
	if strings.Contains(globOut, "gamma.txt") {
		t.Fatalf("gamma.txt should not be in glob output: %s", globOut)
	}

	// Read each found .go file to verify content.
	for _, name := range []string{"alpha.go", "beta.go", "delta.go"} {
		readInput, _ := json.Marshal(map[string]string{"path": filepath.Join(dir, name)})
		readOut, err := FileReadTool{}.Execute(context.Background(), readInput)
		if err != nil {
			t.Fatalf("Read %s error: %v", name, err)
		}
		expected := files[name]
		if readOut != expected {
			t.Fatalf("Read %s: expected %q, got %q", name, expected, readOut)
		}
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// 4. Safety blocks: destructive command blocked, credential write blocked
// ──────────────────────────────────────────────────────────────────────────────

func TestIntegration_SafetyBlocks_DestructiveCommands(t *testing.T) {
	destructive := []struct {
		name string
		cmd  string
	}{
		{"rm -rf root", "rm -rf /"},
		{"rm -rf home", "rm -rf ~"},
		{"rm -rf cwd", "rm -rf ."},
		{"fork bomb", ":(){ :|:& };:"},
		{"dd wipe disk", "dd if=/dev/zero of=/dev/sda"},
		{"git reset hard", "git reset --hard HEAD~10"},
		{"git push force", "git push --force origin main"},
	}

	for _, tc := range destructive {
		t.Run(tc.name, func(t *testing.T) {
			// IsDestructiveCommand should flag it.
			if !IsDestructiveCommand(tc.cmd) {
				t.Errorf("expected IsDestructiveCommand=true for %q", tc.cmd)
			}

			// BashTool.Execute should block it.
			input, _ := json.Marshal(map[string]string{"command": tc.cmd})
			_, err := BashTool{}.Execute(context.Background(), input)
			if err == nil {
				t.Errorf("expected BashTool to block destructive command: %s", tc.cmd)
			}
		})
	}
}

func TestIntegration_SafetyBlocks_CredentialDetection(t *testing.T) {
	cases := []struct {
		name    string
		content string
		blocked bool
	}{
		{"OpenAI key", "sk-abcdefghijklmnopqrstuvwxyz1234567890", true},
		{"AWS key", "AKIAIOSFODNN7EXAMPLE1", true},
		{"GitHub PAT", "ghp_ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijkl", true},
		{"PEM key", "-----BEGIN RSA PRIVATE KEY-----", true},
		{"connection string", "postgres://admin:s3cret@db.host:5432/mydb", true},
		{"safe content", "package main\n\nfunc main() {}", false},
		{"public key", "-----BEGIN PUBLIC KEY-----", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			detected := DetectCredentials(tc.content)
			if tc.blocked && detected == "" {
				t.Errorf("expected credential detection for %q", tc.name)
			}
			if !tc.blocked && detected != "" {
				t.Errorf("unexpected credential detection (%s) for %q", detected, tc.name)
			}
		})
	}
}

func TestIntegration_SafetyBlocks_SensitivePaths(t *testing.T) {
	home, _ := os.UserHomeDir()

	blocked := []string{
		filepath.Join(home, ".ssh", "id_rsa"),
		filepath.Join(home, ".ssh", "id_ed25519"),
		filepath.Join(home, ".ssh", "config"),
		filepath.Join(home, ".aws", "credentials"),
		filepath.Join(home, ".env"),
		"/tmp/project/.env",
		"/tmp/project/credentials.json",
	}
	for _, p := range blocked {
		if reason := IsSensitivePath(p); reason == "" {
			t.Errorf("expected path %q to be blocked", p)
		}
	}

	allowed := []string{
		filepath.Join(home, "project", "main.go"),
		filepath.Join(home, ".bashrc"),
		"/tmp/test.go",
	}
	for _, p := range allowed {
		if reason := IsSensitivePath(p); reason != "" {
			t.Errorf("expected path %q to be allowed, got: %s", p, reason)
		}
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// Additional: Write -> Read chain
// ──────────────────────────────────────────────────────────────────────────────

func TestIntegration_WriteThenRead(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "written.txt")

	// Write creates a file.
	writeInput, _ := json.Marshal(map[string]string{
		"path":    filePath,
		"content": "Content from Write tool",
	})
	wt := FileWriteTool{}
	writeOut, err := wt.Execute(context.Background(), writeInput)
	if err != nil {
		t.Fatalf("Write error: %v", err)
	}
	if !strings.Contains(writeOut, "Wrote") {
		t.Fatalf("expected write confirmation, got %q", writeOut)
	}

	// Read confirms the content.
	readInput, _ := json.Marshal(map[string]string{"path": filePath})
	rt := FileReadTool{}
	readOut, err := rt.Execute(context.Background(), readInput)
	if err != nil {
		t.Fatalf("Read error: %v", err)
	}
	if readOut != "Content from Write tool" {
		t.Fatalf("expected written content, got %q", readOut)
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// Additional: Write -> Edit -> Read chain
// ──────────────────────────────────────────────────────────────────────────────

func TestIntegration_WriteEditRead(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "chain.txt")

	// Write initial content.
	writeInput, _ := json.Marshal(map[string]string{
		"path":    filePath,
		"content": "Hello World",
	})
	wt := FileWriteTool{}
	if _, err := wt.Execute(context.Background(), writeInput); err != nil {
		t.Fatal(err)
	}

	// Edit it.
	editInput, _ := json.Marshal(map[string]string{
		"path":    filePath,
		"old_str": "World",
		"new_str": "Hawk",
	})
	et := FileEditTool{}
	if _, err := et.Execute(context.Background(), editInput); err != nil {
		t.Fatal(err)
	}

	// Read confirms the chain.
	readInput, _ := json.Marshal(map[string]string{"path": filePath})
	rt := FileReadTool{}
	readOut, err := rt.Execute(context.Background(), readInput)
	if err != nil {
		t.Fatal(err)
	}
	if readOut != "Hello Hawk" {
		t.Fatalf("expected 'Hello Hawk', got %q", readOut)
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// Additional: Bash -> Glob chain
// ──────────────────────────────────────────────────────────────────────────────

func TestIntegration_BashCreatesFilesGlobFinds(t *testing.T) {
	dir := t.TempDir()

	// Bash creates multiple files.
	bashCmd := "for f in a.go b.go c.go; do echo 'pkg' > " + dir + "/$f; done"
	bashInput, _ := json.Marshal(map[string]string{"command": bashCmd})
	bt := BashTool{}
	if _, err := bt.Execute(context.Background(), bashInput); err != nil {
		t.Fatal(err)
	}

	// Glob finds them.
	globInput, _ := json.Marshal(map[string]interface{}{
		"pattern": "*.go",
		"path":    dir,
	})
	gt := GlobTool{}
	globOut, err := gt.Execute(context.Background(), globInput)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(globOut, "3 files") {
		t.Fatalf("expected 3 files found, got: %s", globOut)
	}
	for _, name := range []string{"a.go", "b.go", "c.go"} {
		if !strings.Contains(globOut, name) {
			t.Fatalf("expected %s in glob output: %s", name, globOut)
		}
	}
}
