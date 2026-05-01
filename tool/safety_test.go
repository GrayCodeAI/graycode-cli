package tool

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// 1. Destructive command detection
// ---------------------------------------------------------------------------

func TestIsDestructiveCommand_TruePositives(t *testing.T) {
	cases := []string{
		"rm -rf /",
		"rm -rf .",
		"rm -rf ~",
		"git reset --hard HEAD~3",
		"git push --force origin main",
		"DROP TABLE users;",
		"TRUNCATE TABLE sessions;",
		"> /dev/sda",
		"dd if=/dev/zero of=/dev/sda",
		"mkfs.ext4 /dev/sda1",
		":(){ :|:& };:",
		// Mixed case
		"DROP TABLE Users",
		"Git Push --Force",
	}
	for _, cmd := range cases {
		if !IsDestructiveCommand(cmd) {
			t.Errorf("expected IsDestructiveCommand=true for %q", cmd)
		}
	}
}

func TestIsDestructiveCommand_FalseNegatives(t *testing.T) {
	safe := []string{
		"echo hello",
		"go test ./...",
		"git status",
		"git push origin main",
		"ls -la",
		"cat file.txt",
		"grep -r pattern .",
	}
	for _, cmd := range safe {
		if IsDestructiveCommand(cmd) {
			t.Errorf("expected IsDestructiveCommand=false for %q", cmd)
		}
	}
}

// ---------------------------------------------------------------------------
// 2. Credential pattern matching
// ---------------------------------------------------------------------------

func TestDetectCredentials(t *testing.T) {
	cases := []struct {
		name    string
		content string
		want    bool
	}{
		{"OpenAI key", "sk-abc123def456ghi789jkl012mno", true},
		{"AWS key", "AKIAIOSFODNN7EXAMPLE", true},
		{"GitHub PAT", "ghp_ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghij", true},
		{"GitHub OAuth", "gho_ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghij", true},
		{"RSA private key", "-----BEGIN RSA PRIVATE KEY-----", true},
		{"EC private key", "-----BEGIN EC PRIVATE KEY-----", true},
		{"OpenSSH private key", "-----BEGIN OPENSSH PRIVATE KEY-----", true},
		{"Generic private key", "-----BEGIN PRIVATE KEY-----", true},
		{"Connection string", "postgres://admin:s3cret@db.host:5432/mydb", true},
		{"Safe content", "this is normal code with no secrets", false},
		{"Short sk prefix", "sk-short", false}, // too short to match 20+
		{"Public key", "-----BEGIN PUBLIC KEY-----", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := DetectCredentials(tc.content)
			if tc.want && got == "" {
				t.Errorf("expected credential detected in %q", tc.content)
			}
			if !tc.want && got != "" {
				t.Errorf("unexpected credential detected (%s) in %q", got, tc.content)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// 3. Path blocking
// ---------------------------------------------------------------------------

func TestIsSensitivePath(t *testing.T) {
	home, _ := os.UserHomeDir()

	blocked := []string{
		filepath.Join(home, ".ssh", "id_rsa"),
		filepath.Join(home, ".ssh", "id_ed25519"),
		filepath.Join(home, ".ssh", "config"),
		filepath.Join(home, ".ssh", "authorized_keys"),
		filepath.Join(home, ".aws", "credentials"),
		filepath.Join(home, ".env"),
		"/some/project/.env",
		"/tmp/app/credentials.json",
	}
	for _, p := range blocked {
		if reason := IsSensitivePath(p); reason == "" {
			t.Errorf("expected path %q to be blocked", p)
		}
	}

	allowed := []string{
		filepath.Join(home, "project", "main.go"),
		"/tmp/test.txt",
		filepath.Join(home, ".bashrc"),
		filepath.Join(home, "code", "config.json"),
	}
	for _, p := range allowed {
		if reason := IsSensitivePath(p); reason != "" {
			t.Errorf("expected path %q to be allowed, got: %s", p, reason)
		}
	}
}

func TestIsSensitivePath_Symlink(t *testing.T) {
	home, _ := os.UserHomeDir()
	sshDir := filepath.Join(home, ".ssh")

	// Only run if ~/.ssh exists (common on dev machines).
	if _, err := os.Stat(sshDir); os.IsNotExist(err) {
		t.Skip("~/.ssh does not exist, skipping symlink test")
	}

	tmpDir := t.TempDir()
	link := filepath.Join(tmpDir, "sneaky_link")
	target := filepath.Join(sshDir, "id_rsa")

	// Only create symlink if the target exists.
	if _, err := os.Stat(target); os.IsNotExist(err) {
		t.Skip("~/.ssh/id_rsa does not exist")
	}

	if err := os.Symlink(target, link); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	if reason := IsSensitivePath(link); reason == "" {
		t.Errorf("symlink to %s should be blocked", target)
	}
}

// ---------------------------------------------------------------------------
// 4. Binary detection
// ---------------------------------------------------------------------------

func TestIsBinaryContent(t *testing.T) {
	// Text content — no null bytes.
	text := []byte("Hello, world!\nThis is plain text.\n")
	if IsBinaryContent(text) {
		t.Error("expected text content to not be detected as binary")
	}

	// Binary content — null byte early.
	bin := make([]byte, 100)
	bin[50] = 0
	if !IsBinaryContent(bin) {
		t.Error("expected binary content (null at byte 50) to be detected")
	}

	// Null byte beyond probe window — should NOT be flagged.
	large := make([]byte, binaryProbeSize+100)
	for i := range large {
		large[i] = 'A'
	}
	large[binaryProbeSize+50] = 0
	if IsBinaryContent(large) {
		t.Error("null byte beyond probe window should not trigger binary detection")
	}

	// Empty content.
	if IsBinaryContent(nil) {
		t.Error("empty/nil content should not be binary")
	}
}

// ---------------------------------------------------------------------------
// 5. Output truncation
// ---------------------------------------------------------------------------

func TestTruncateOutput(t *testing.T) {
	short := "hello world"
	if got := TruncateOutput(short); got != short {
		t.Errorf("short output should not be truncated, got len %d", len(got))
	}

	long := strings.Repeat("A", maxOutputBytes+1000)
	got := TruncateOutput(long)
	if !strings.HasSuffix(got, "[output truncated — showing first 50KB]") {
		t.Error("expected truncation indicator")
	}
	// The prefix should be exactly maxOutputBytes of the original.
	prefix := got[:maxOutputBytes]
	if prefix != long[:maxOutputBytes] {
		t.Error("truncated prefix does not match original")
	}
}

func TestTruncateOutput_ExactBoundary(t *testing.T) {
	exact := strings.Repeat("B", maxOutputBytes)
	if got := TruncateOutput(exact); got != exact {
		t.Error("output at exact boundary should not be truncated")
	}
}

// ---------------------------------------------------------------------------
// 6. Timeout configuration
// ---------------------------------------------------------------------------

func TestToolTimeout(t *testing.T) {
	cases := map[string]time.Duration{
		"Bash":     120 * time.Second,
		"bash":     120 * time.Second,
		"WebFetch": 30 * time.Second,
		"web_fetch": 30 * time.Second,
		"Grep":     60 * time.Second,
		"grep":     60 * time.Second,
		"Read":     60 * time.Second,
		"Write":    60 * time.Second,
		"Edit":     60 * time.Second,
		"unknown":  60 * time.Second,
	}
	for name, want := range cases {
		if got := ToolTimeout(name); got != want {
			t.Errorf("ToolTimeout(%q) = %v, want %v", name, got, want)
		}
	}
}

// ---------------------------------------------------------------------------
// 7. ResolvePath
// ---------------------------------------------------------------------------

func TestResolvePath(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "hello.txt")
	if err := os.WriteFile(file, []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := ResolvePath(file)
	if err != nil {
		t.Fatal(err)
	}
	if !filepath.IsAbs(got) {
		t.Errorf("expected absolute path, got %q", got)
	}

	// Non-existent file should still return an absolute path (parent resolved).
	got2, err := ResolvePath(filepath.Join(dir, "nonexistent.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if !filepath.IsAbs(got2) {
		t.Errorf("expected absolute path for nonexistent file, got %q", got2)
	}
}
