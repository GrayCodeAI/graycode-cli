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

func TestIsDestructiveCommand_EmptyString(t *testing.T) {
	if IsDestructiveCommand("") {
		t.Error("empty string should not be destructive")
	}
}

func TestIsDestructiveCommand_CaseInsensitive(t *testing.T) {
	cases := []string{
		"RM -RF /tmp",
		"Rm -Rf /tmp",
		"git RESET --hard",
		"git PUSH --force",
		"drop TABLE x",
		"TRUNCATE logs",
	}
	for _, cmd := range cases {
		if !IsDestructiveCommand(cmd) {
			t.Errorf("expected case-insensitive match for %q", cmd)
		}
	}
}

func TestIsDestructiveCommand_SegmentedCommands(t *testing.T) {
	// Destructive pattern appears after a semicolon or pipe
	cases := []string{
		"echo hello; rm -rf /",
		"ls | rm -rf .",
		"echo a && git reset --hard",
		"cat f || drop table t",
	}
	for _, cmd := range cases {
		if !IsDestructiveCommand(cmd) {
			t.Errorf("expected destructive for segmented command %q", cmd)
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
		{"OpenAI key", "sk-abcde12345fghijklmnopqrstuvwxyz", true},
		{"OpenAI key", "sk-abc012345678901234567890123456789", true},
		{"Anthropic key", "sk-ant-api25-ABCDEFGHIJKLMNOPQRST", true},
		{"AWS key", "AKIAIOEXAMPLEMPLE012", true},
		{"GitHub PAT", "ghp_ABCDEfghijklmnopqrstuvwxyz0123456789AB", true},
		{"GitHub OAuth", "gho_ABCDEfghijklmnopqrstuvwxyz0123456789AB", true},
		{"EC private key", "-----BEGIN EC PRIVATE KEY-----", true},
		{"OpenSSH private key", "-----BEGIN OPENSSH PRIVATE KEY-----", true},
		{"Generic private key", "-----BEGIN PRIVATE KEY-----", true},
		{"Connection string", "postgres://admin:***@db.host:5432/mydb", true},
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

func TestDetectCredentials_EmptyString(t *testing.T) {
	if got := DetectCredentials(""); got != "" {
		t.Errorf("expected empty string for empty content, got %q", got)
	}
}

func TestDetectCredentials_GitLabPAT(t *testing.T) {
	content := "token = glpat-ABCDefghijklmnopqrst1234"
	got := DetectCredentials(content)
	if got == "" {
		t.Error("expected GitLab PAT to be detected")
	}
}

func TestDetectCredentials_SlackToken(t *testing.T) {
	cases := []string{
		"xoxb-1234567890-1234567890",
		"xoxp-1234567890-1234567890",
		"xoxs-1234567890-1234567890",
	}
	for _, token := range cases {
		if got := DetectCredentials(token); got == "" {
			t.Errorf("expected Slack token detected for %q", token)
		}
	}
}

func TestDetectCredentials_StripeKey(t *testing.T) {
	cases := []string{
		"sk_test_abc123def456ghi789jkl012mno",
		"rk_test_abc123def456ghi789jkl012mno",
	}
	for _, key := range cases {
		if got := DetectCredentials(key); got == "" {
			t.Errorf("expected Stripe key detected for %q", key)
		}
	}
}

func TestDetectCredentials_AnthropicKey(t *testing.T) {
	key := "sk-ant-api02-ABCDEFGHIJKLMNOPQRSTUVWXYZabcdef"
	got := DetectCredentials(key)
	if got == "" {
		t.Error("expected Anthropic API key to be detected")
	}
}

func TestDetectCredentials_OpenAIKey(t *testing.T) {
	// A key that matches sk- with 20+ alphanumeric chars but NOT sk-ant-
	key := "sk-ABCDEFGHIJKLMNOPQRSTUVWXYZ123456"
	got := DetectCredentials(key)
	if got == "" {
		t.Error("expected OpenAI key to be detected")
	}
}

func TestDetectCredentials_AWSAccessKey(t *testing.T) {
	key := "AKIAIOSFODNN7EXAMPLE"
	got := DetectCredentials(key)
	if got == "" {
		t.Error("expected AWS access key to be detected")
	}
}

func TestDetectCredentials_ConnectionStringVariants(t *testing.T) {
	cases := []string{
		"mysql://root:password123@localhost/mydb",
		"redis://default:secret@redis:6379",
		"mongodb://admin:mongo@mongo:27017/db",
	}
	for _, cs := range cases {
		if got := DetectCredentials(cs); got == "" {
			t.Errorf("expected connection string detected for %q", cs)
		}
	}
}

func TestDetectCredentials_SafeStrings(t *testing.T) {
	cases := []string{
		"hello world",
		"https://example.com/path",
		"SELECT * FROM users WHERE id = 1",
		"var x = 'not a secret'",
		"-----BEGIN CERTIFICATE-----",
	}
	for _, s := range cases {
		if got := DetectCredentials(s); got != "" {
			t.Errorf("unexpected credential in safe string %q: %s", s, got)
		}
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
		filepath.Join(home, ".hawk", "provider.json"),
		filepath.Join(home, ".hawk", "env"),
		filepath.Join(home, ".hawk", ".env"),
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

func TestIsSensitivePath_HawkConfigDir(t *testing.T) {
	cfgDir := t.TempDir()
	t.Setenv("HAWK_CONFIG_DIR", cfgDir)
	prov := filepath.Join(cfgDir, "provider.json")
	if reason := IsSensitivePath(prov); reason == "" {
		t.Fatalf("expected custom HAWK_CONFIG_DIR provider.json blocked, got empty")
	}
}

func TestIsSensitivePath_HawkConfigDirEnv(t *testing.T) {
	cfgDir := t.TempDir()
	t.Setenv("HAWK_CONFIG_DIR", cfgDir)

	envPath := filepath.Join(cfgDir, "env")
	if reason := IsSensitivePath(envPath); reason == "" {
		t.Error("expected HAWK_CONFIG_DIR/env to be blocked")
	}

	dotEnvPath := filepath.Join(cfgDir, ".env")
	if reason := IsSensitivePath(dotEnvPath); reason == "" {
		t.Error("expected HAWK_CONFIG_DIR/.env to be blocked")
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

func TestIsSensitivePath_BlockedBasenames(t *testing.T) {
	cases := []struct {
		path string
		name string
	}{
		{"/tmp/project/.env", ".env"},
		{"/tmp/app/credentials.json", "credentials.json"},
		{"/home/user/.npmrc", ".npmrc"},
		{"/home/user/.netrc", ".netrc"},
		{"/home/user/.pgpass", ".pgpass"},
		{"/etc/kubernetes/kubeconfig", "kubeconfig"},
		{"/app/token.json", "token.json"},
		{"/app/service-account.json", "service-account.json"},
		{"/app/credentials.yaml", "credentials.yaml"},
		{"/app/credentials.yml", "credentials.yml"},
		{"/app/credentials.xml", "credentials.xml"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if reason := IsSensitivePath(tc.path); reason == "" {
				t.Errorf("expected %q to be blocked (basename %s)", tc.path, tc.name)
			}
		})
	}
}

func TestIsSensitivePath_DotEnvVariants(t *testing.T) {
	// Files starting with ".env" (but not .envrc) should be blocked
	cases := []string{
		"/tmp/project/.env.local",
		"/tmp/project/.env.production",
		"/tmp/project/.env.backup",
		"/tmp/project/.env.development",
	}
	for _, p := range cases {
		if reason := IsSensitivePath(p); reason == "" {
			t.Errorf("expected %q to be blocked (.env variant)", p)
		}
	}
}

func TestIsSensitivePath_EnvrcNotBlocked(t *testing.T) {
	// .envrc should NOT be blocked (it's a direnv config, not an env file)
	reason := IsSensitivePath("/tmp/project/.envrc")
	if reason != "" {
		t.Errorf("expected .envrc to be allowed, got: %s", reason)
	}
}

func TestIsSensitivePath_SSHCatchAll(t *testing.T) {
	home, _ := os.UserHomeDir()
	// Any file inside ~/.ssh should be blocked
	paths := []string{
		filepath.Join(home, ".ssh", "my_custom_key"),
		filepath.Join(home, ".ssh", "random_file.txt"),
	}
	for _, p := range paths {
		if reason := IsSensitivePath(p); reason == "" {
			t.Errorf("expected %q to be blocked (inside .ssh)", p)
		}
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

func TestIsBinaryContent_HighControlCharRatio(t *testing.T) {
	// Data with >30% control characters (but no null bytes) should be binary
	data := make([]byte, 1000)
	// Fill with normal text chars first
	for i := range data {
		data[i] = 'A'
	}
	// Now put control chars in 31% of bytes (but not null, tab, newline, CR)
	for i := 0; i < 310; i++ {
		data[i] = 0x01 // SOH control char
	}
	if !IsBinaryContent(data) {
		t.Error("expected high control char ratio to be detected as binary")
	}
}

func TestIsBinaryContent_LowControlCharRatio(t *testing.T) {
	// Data with <30% control characters should NOT be binary
	data := make([]byte, 1000)
	for i := range data {
		data[i] = 'A'
	}
	// Put control chars in 29% of bytes
	for i := 0; i < 290; i++ {
		data[i] = 0x01
	}
	if IsBinaryContent(data) {
		t.Error("expected low control char ratio to not be binary")
	}
}

func TestIsBinaryContent_TabNewlineCR(t *testing.T) {
	// Tab, newline, and carriage return should not count as binary indicators
	data := []byte("hello\tworld\nline2\r\nline3\n")
	if IsBinaryContent(data) {
		t.Error("tab/newline/CR should not trigger binary detection")
	}
}

func TestIsBinaryContent_EmptyByteSlice(t *testing.T) {
	if IsBinaryContent([]byte{}) {
		t.Error("empty byte slice should not be binary")
	}
}

func TestIsBinaryContent_SingleNullByte(t *testing.T) {
	if !IsBinaryContent([]byte{0}) {
		t.Error("single null byte should be detected as binary")
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
	if !strings.HasSuffix(got, "[output truncated — showing first 500KB]") {
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

func TestTruncateOutput_Empty(t *testing.T) {
	if got := TruncateOutput(""); got != "" {
		t.Errorf("empty string should return empty, got %q", got)
	}
}

func TestTruncateOutput_UTF8Multibyte(t *testing.T) {
	// Build a string of multi-byte UTF-8 chars that exceeds the limit
	// 'é' is 2 bytes in UTF-8
	base := "é"
	long := strings.Repeat(base, maxOutputBytes) // much more than maxOutputBytes
	got := TruncateOutput(long)
	if !strings.HasSuffix(got, "[output truncated — showing first 500KB]") {
		t.Error("expected truncation indicator for UTF-8 content")
	}
	// The truncation should not split a multi-byte character
	// Verify by checking the truncated prefix is valid UTF-8
	truncated := got[:len(got)-len("\n[output truncated — showing first 500KB]")]
	if !isValidUTF8([]byte(truncated)) {
		t.Error("truncated output should be valid UTF-8")
	}
}

func TestTruncateOutput_JustUnderBoundary(t *testing.T) {
	s := strings.Repeat("x", maxOutputBytes-1)
	if got := TruncateOutput(s); got != s {
		t.Error("output just under boundary should not be truncated")
	}
}

func TestTruncateOutput_JustOverBoundary(t *testing.T) {
	s := strings.Repeat("x", maxOutputBytes+1)
	got := TruncateOutput(s)
	if !strings.Contains(got, "[output truncated") {
		t.Error("output just over boundary should be truncated")
	}
}

// isValidUTF8 checks if a byte slice is valid UTF-8
func isValidUTF8(b []byte) bool {
	for i := 0; i < len(b); {
		if b[i] < 0x80 {
			i++
			continue
		}
		// Multi-byte sequence
		var n int
		if b[i]&0xE0 == 0xC0 {
			n = 2
		} else if b[i]&0xF0 == 0xE0 {
			n = 3
		} else if b[i]&0xF8 == 0xF0 {
			n = 4
		} else {
			return false
		}
		if i+n > len(b) {
			return false
		}
		for j := 1; j < n; j++ {
			if b[i+j]&0xC0 != 0x80 {
				return false
			}
		}
		i += n
	}
	return true
}

// ---------------------------------------------------------------------------
// 6. Timeout configuration
// ---------------------------------------------------------------------------

func TestToolTimeout(t *testing.T) {
	cases := map[string]time.Duration{
		"Bash":      120 * time.Second,
		"bash":      120 * time.Second,
		"WebFetch":  30 * time.Second,
		"web_fetch": 30 * time.Second,
		"Grep":      60 * time.Second,
		"grep":      60 * time.Second,
		"Read":      60 * time.Second,
		"Write":     60 * time.Second,
		"Edit":      60 * time.Second,
		"unknown":   60 * time.Second,
	}
	for name, want := range cases {
		if got := ToolTimeout(name); got != want {
			t.Errorf("ToolTimeout(%q) = %v, want %v", name, got, want)
		}
	}
}

func TestToolTimeout_EmptyString(t *testing.T) {
	got := ToolTimeout("")
	if got != 60*time.Second {
		t.Errorf("ToolTimeout('') = %v, want 60s", got)
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

func TestResolvePath_RelativePath(t *testing.T) {
	got, err := ResolvePath("relative/path/file.txt")
	if err != nil {
		t.Fatal(err)
	}
	if !filepath.IsAbs(got) {
		t.Errorf("expected absolute path for relative input, got %q", got)
	}
}

func TestResolvePath_Symlink(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target.txt")
	if err := os.WriteFile(target, []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link.txt")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	got, err := ResolvePath(link)
	if err != nil {
		t.Fatal(err)
	}
	// Resolve target too (macOS /var -> /private/var)
	resolvedTarget, _ := filepath.EvalSymlinks(target)
	if resolvedTarget == "" {
		resolvedTarget = target
	}
	if got != resolvedTarget {
		t.Errorf("expected resolved path %q, got %q", target, got)
	}
}

// ---------------------------------------------------------------------------
// 8. SSRF protection
// ---------------------------------------------------------------------------

func TestValidateURLPublic_SkipContext(t *testing.T) {
	ctx := WithSSRFSkip(t.Context())
	got, _, err := validateURLPublic(ctx, "http://127.0.0.1/metadata")
	if err != nil {
		t.Fatalf("expected no error with SSRF skip, got: %v", err)
	}
	if got != "http://127.0.0.1/metadata" {
		t.Errorf("expected URL passthrough, got %q", got)
	}
}

func TestValidateURLPublic_InvalidURL(t *testing.T) {
	ctx := t.Context()
	_, _, err := validateURLPublic(ctx, "://invalid")
	if err == nil {
		t.Error("expected error for invalid URL")
	}
}

func TestValidateURLPublic_BlockedScheme(t *testing.T) {
	ctx := t.Context()
	cases := []string{
		"ftp://example.com/file",
		"file:///etc/passwd",
		"javascript:alert(1)",
	}
	for _, u := range cases {
		_, _, err := validateURLPublic(ctx, u)
		if err == nil {
			t.Errorf("expected error for URL scheme %q", u)
		}
	}
}

func TestValidateURLPublic_NoHost(t *testing.T) {
	ctx := t.Context()
	_, _, err := validateURLPublic(ctx, "http:///path")
	if err == nil {
		t.Error("expected error for URL with no host")
	}
}

func TestSSRFSafeClient_RedirectLimit(t *testing.T) {
	ctx := WithSSRFSkip(t.Context())
	client := ssrfSafeClient(ctx, 5*time.Second)
	if client == nil {
		t.Fatal("expected non-nil client")
	}
	if client.Timeout != 5*time.Second {
		t.Errorf("expected timeout 5s, got %v", client.Timeout)
	}
}

func TestCommandReferencesSensitivePath(t *testing.T) {
	blocked := []string{
		"cat ~/.ssh/id_rsa",
		"cat $HOME/.ssh/id_ed25519",
		"base64 /Users/someone/.ssh/id_ecdsa",
		"cat ~/.aws/credentials",
		"cat .env",
		"head -n5 .env.production",
		"cp config/.env /tmp/x",
		"cat .npmrc",
		"less ~/.netrc",
		"tar czf out.tgz ~/.ssh",
		"cat ~/.hawk/provider.json",
		"grep key credentials.json",
	}
	for _, cmd := range blocked {
		if reason := CommandReferencesSensitivePath(cmd); reason == "" {
			t.Errorf("CommandReferencesSensitivePath(%q) = empty, want blocked", cmd)
		}
	}

	allowed := []string{
		"ls -la",
		"cat README.md",
		"cat .envrc",
		"cat env.md",
		"go test ./...",
		"cat docs/environment.md",
		"echo hello > out.txt",
		"cat provider.json.md",
		"git status",
	}
	for _, cmd := range allowed {
		if reason := CommandReferencesSensitivePath(cmd); reason != "" {
			t.Errorf("CommandReferencesSensitivePath(%q) = %q, want allowed", cmd, reason)
		}
	}
}

func TestBashSensitivePathIntegration(t *testing.T) {
	if !IsSuspicious("cat ~/.ssh/id_rsa") {
		t.Error("IsSuspicious should flag reads of SSH private keys")
	}
	if !isHardDeny("cat ~/.aws/credentials") {
		t.Error("isHardDeny should block credential reads when prompts are bypassed")
	}
	if isHardDeny("go build ./...") {
		t.Error("isHardDeny should not block ordinary commands")
	}
}
