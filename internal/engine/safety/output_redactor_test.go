package safety

import (
	"strings"
	"sync"
	"testing"
)

func TestNewOutputRedactor(t *testing.T) {
	r := NewOutputRedactor()
	if r == nil {
		t.Fatal("NewOutputRedactor returned nil")
	}
	if len(r.Patterns) < 25 {
		t.Errorf("expected at least 25 built-in patterns, got %d", len(r.Patterns))
	}
	if r.KnownSecrets == nil {
		t.Error("KnownSecrets map not initialized")
	}
	if r.Stats.ByCategory == nil {
		t.Error("Stats.ByCategory map not initialized")
	}
}

func TestRedactAWSAccessKey(t *testing.T) {
	r := NewOutputRedactor()
	input := "Found key: AKIAIOSFODNN7EXAMPLE in config"
	result := r.Redact(input)
	if strings.Contains(result, "AKIAIOSFODNN7EXAMPLE") {
		t.Error("AWS access key was not redacted")
	}
	if !strings.Contains(result, "[REDACTED:api_key]") {
		t.Errorf("expected [REDACTED:api_key] placeholder, got: %s", result)
	}
}

func TestRedactAWSSecretKey(t *testing.T) {
	r := NewOutputRedactor()
	input := "aws_secret_access_key = wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY"
	result := r.Redact(input)
	if strings.Contains(result, "wJalrXUtnFEMI") {
		t.Error("AWS secret key was not redacted")
	}
	if !strings.Contains(result, "[REDACTED:api_key]") {
		t.Errorf("expected [REDACTED:api_key], got: %s", result)
	}
}

func TestRedactGitHubTokens(t *testing.T) {
	r := NewOutputRedactor()
	tests := []struct {
		name  string
		input string
	}{
		{"PAT", "token: ghp_ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghij"},
		{"OAuth", "auth: gho_ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghij"},
		{"App", "install: ghs_ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghij"},
		{"Refresh", "refresh: ghr_ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghij"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := r.Redact(tt.input)
			if !strings.Contains(result, "[REDACTED:token]") {
				t.Errorf("GitHub token not redacted: %s", result)
			}
		})
	}
}

func TestRedactGenericAPIKeys(t *testing.T) {
	r := NewOutputRedactor()
	tests := []struct {
		name  string
		input string
	}{
		{"sk- prefix", "api_key: sk-abcdefghijklmnopqrstuvwxyz1234"},
		{"key- prefix", "my key-abcdefghijklmnopqrstuvwxyz1234"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := r.Redact(tt.input)
			if !strings.Contains(result, "[REDACTED:api_key]") {
				t.Errorf("API key not redacted: %s", result)
			}
		})
	}
}

func TestRedactPasswordInURL(t *testing.T) {
	r := NewOutputRedactor()
	input := "connecting to https://admin:sup3rS3cret@db.example.com:5432/mydb"
	result := r.Redact(input)
	if strings.Contains(result, "sup3rS3cret") {
		t.Error("password in URL was not redacted")
	}
}

func TestRedactBearerToken(t *testing.T) {
	r := NewOutputRedactor()
	input := "Authorization: Bearer eyAbcdefghijklmnopqrstuvwxyz.something"
	result := r.Redact(input)
	if !strings.Contains(result, "[REDACTED:token]") {
		t.Errorf("bearer token not redacted: %s", result)
	}
}

func TestRedactPrivateKeys(t *testing.T) {
	r := NewOutputRedactor()
	tests := []struct {
		name  string
		input string
	}{
		{
			"RSA",
			"-----BEGIN RSA PRIVATE KEY-----\nMIIBogIBAAJBALRm...\n-----END RSA PRIVATE KEY-----",
		},
		{
			"EC",
			"-----BEGIN EC PRIVATE KEY-----\nMHQCAQEEIODo...\n-----END EC PRIVATE KEY-----",
		},
		{
			"Generic",
			"-----BEGIN PRIVATE KEY-----\nMIIEvgIBADANBgk...\n-----END PRIVATE KEY-----",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := r.Redact(tt.input)
			if !strings.Contains(result, "[REDACTED:cert]") {
				t.Errorf("private key not redacted: %s", result)
			}
		})
	}
}

func TestRedactConnectionStrings(t *testing.T) {
	r := NewOutputRedactor()
	tests := []struct {
		name  string
		input string
	}{
		{"Postgres", "DATABASE_URL=postgres://user:password123@localhost:5432/db"},
		{"MySQL", "mysql://root:s3cr3t@127.0.0.1:3306/app"},
		{"MongoDB", "mongodb+srv://admin:hunter2@cluster0.example.net/test"},
		{"Redis", "redis://:mysecretpassword@redis.example.com:6379"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := r.Redact(tt.input)
			if !strings.Contains(result, "[REDACTED:") {
				t.Errorf("connection string not redacted: %s", result)
			}
		})
	}
}

func TestRedactJWT(t *testing.T) {
	r := NewOutputRedactor()
	// A minimal JWT-shaped token.
	jwt := "eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.dozjgNryP4J3jVmNHl0w5N_XgL0n3I9PlFUP0THsR8U"
	input := "token: " + jwt
	result := r.Redact(input)
	if strings.Contains(result, jwt) {
		t.Error("JWT was not redacted")
	}
	if !strings.Contains(result, "[REDACTED:token]") {
		t.Errorf("expected [REDACTED:token], got: %s", result)
	}
}

func TestRedactSlackToken(t *testing.T) {
	r := NewOutputRedactor()
	input := "SLACK_TOKEN=xoxb-FAKE00000000-0000000000000-FaKeToKeNvAlUeTeStOnLy0"
	result := r.Redact(input)
	if !strings.Contains(result, "[REDACTED:token]") {
		t.Errorf("Slack token not redacted: %s", result)
	}
}

func TestRedactSendGridKey(t *testing.T) {
	r := NewOutputRedactor()
	// SG.{22 chars}.{43 chars} = valid SendGrid key format
	input := "SENDGRID_API_KEY=SG.abcdefghijklmnopqrstuv.abcdefghijklmnopqrstuvwxyz01234567890123456"
	result := r.Redact(input)
	if !strings.Contains(result, "[REDACTED:api_key]") {
		t.Errorf("SendGrid key not redacted: %s", result)
	}
}

func TestAddKnownSecret(t *testing.T) {
	r := NewOutputRedactor()
	r.AddKnownSecret("my_api_key", "super-secret-value-12345")
	input := "The API returned: super-secret-value-12345 for your request"
	result := r.Redact(input)
	if strings.Contains(result, "super-secret-value-12345") {
		t.Error("known secret was not redacted")
	}
	if !strings.Contains(result, "[REDACTED:my_api_key]") {
		t.Errorf("expected [REDACTED:my_api_key], got: %s", result)
	}
}

func TestRedactEnvVars(t *testing.T) {
	r := NewOutputRedactor()
	r.AddKnownSecret("env:GITHUB_TOKEN", "ghp_realtoken123456789012345678901234567")

	// RedactEnvVars should catch it via the env var path.
	input := "Using token ghp_realtoken123456789012345678901234567 for auth"
	result := r.RedactEnvVars(input)
	if strings.Contains(result, "ghp_realtoken123456789012345678901234567") {
		t.Error("env var value was not redacted")
	}
	if !strings.Contains(result, "[REDACTED:env:GITHUB_TOKEN]") {
		t.Errorf("expected [REDACTED:env:GITHUB_TOKEN], got: %s", result)
	}
}

func TestRegisterEnvSecrets(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "ghp_registered123456789012345678901234567")
	t.Setenv("OPENAI_API_KEY", "sk-registered-token-value-12345")
	t.Setenv("SHORT_VAL", "x")            // not a secret-named var; ignored by design
	t.Setenv("AWS_SECRET_ACCESS_KEY", "") // empty values are skipped

	r := NewOutputRedactor()
	r.RegisterEnvSecrets()

	input := "auth via ghp_registered123456789012345678901234567 and sk-registered-token-value-12345"
	result := r.RedactEnvVars(input)
	if strings.Contains(result, "ghp_registered123456789012345678901234567") {
		t.Error("GITHUB_TOKEN value was not registered and redacted")
	}
	if strings.Contains(result, "sk-registered-token-value-12345") {
		t.Error("OPENAI_API_KEY value was not registered and redacted")
	}
	if !strings.Contains(result, "[REDACTED:env:GITHUB_TOKEN]") || !strings.Contains(result, "[REDACTED:env:OPENAI_API_KEY]") {
		t.Errorf("expected env redaction placeholders, got: %s", result)
	}
}

func TestRedactPaths(t *testing.T) {
	r := NewOutputRedactor()
	input := "Reading /home/user/.config/secrets.json"
	result := r.RedactPaths(input, "/home/user")
	if strings.Contains(result, "/home/user") {
		t.Error("home directory path was not redacted")
	}
	expected := "Reading ~/.config/secrets.json"
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestRedactPathsTrailingSlash(t *testing.T) {
	r := NewOutputRedactor()
	input := "File at /Users/dev/project/main.go"
	result := r.RedactPaths(input, "/Users/dev/")
	if strings.Contains(result, "/Users/dev") {
		t.Error("home directory path was not redacted even with trailing slash")
	}
}

func TestRedactPathsEmpty(t *testing.T) {
	r := NewOutputRedactor()
	input := "some output"
	result := r.RedactPaths(input, "")
	if result != input {
		t.Error("empty homedir should not modify output")
	}
}

func TestIsClean(t *testing.T) {
	r := NewOutputRedactor()

	clean := "This is a normal output with no secrets"
	if !r.IsClean(clean) {
		t.Error("IsClean returned false for clean output")
	}

	dirty := "Found AKIAIOSFODNN7EXAMPLE in the logs"
	if r.IsClean(dirty) {
		t.Error("IsClean returned true for output containing AWS key")
	}
}

func TestIsCleanWithKnownSecrets(t *testing.T) {
	r := NewOutputRedactor()
	r.AddKnownSecret("test_secret", "mysecretvalue42")

	if r.IsClean("output contains mysecretvalue42 here") {
		t.Error("IsClean should return false when known secret is present")
	}

	if !r.IsClean("output is safe") {
		t.Error("IsClean should return true when no secrets present")
	}
}

func TestOutputRedactorFormatStats(t *testing.T) {
	r := NewOutputRedactor()

	// Empty stats.
	stats := r.FormatStats()
	if !strings.Contains(stats, "Total: 0 secrets redacted") {
		t.Errorf("unexpected empty stats: %s", stats)
	}

	// Redact something to generate stats.
	r.Redact("key: AKIAIOSFODNN7EXAMPLE and ghp_ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghij")

	stats = r.FormatStats()
	if !strings.Contains(stats, "Redaction Stats:") {
		t.Errorf("missing header in stats: %s", stats)
	}
	if !strings.Contains(stats, "Total:") {
		t.Errorf("missing total in stats: %s", stats)
	}
	if !strings.Contains(stats, "By type:") {
		t.Errorf("missing by-type in stats: %s", stats)
	}
	if !strings.Contains(stats, "Bytes saved:") {
		t.Errorf("missing bytes saved in stats: %s", stats)
	}
}

func TestOutputRedactorAddPattern(t *testing.T) {
	r := NewOutputRedactor()
	initialCount := len(r.Patterns)

	err := r.AddPattern("custom_secret", `CUSTOM_[A-Z]{10,}`, "api_key")
	if err != nil {
		t.Fatalf("AddPattern failed: %v", err)
	}
	if len(r.Patterns) != initialCount+1 {
		t.Error("pattern was not added")
	}

	input := "secret: CUSTOM_ABCDEFGHIJKLMN"
	result := r.Redact(input)
	if !strings.Contains(result, "[REDACTED:api_key]") {
		t.Errorf("custom pattern not applied: %s", result)
	}
}

func TestAddPatternInvalid(t *testing.T) {
	r := NewOutputRedactor()
	err := r.AddPattern("bad_pattern", `[invalid`, "api_key")
	if err == nil {
		t.Error("expected error for invalid regex pattern")
	}
}

func TestRedactMultipleMatches(t *testing.T) {
	r := NewOutputRedactor()
	input := "key1: AKIAIOSFODNN7EXAMPLE1 and key2: AKIAIOSFODNN7EXAMPLE2"
	result := r.Redact(input)

	count := strings.Count(result, "[REDACTED:api_key]")
	if count != 2 {
		t.Errorf("expected 2 redactions, got %d in: %s", count, result)
	}
}

func TestRedactStatsAccumulate(t *testing.T) {
	r := NewOutputRedactor()

	r.Redact("AKIAIOSFODNN7EXAMPLE1")
	r.Redact("ghp_ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghij")

	if r.Stats.TotalRedacted < 2 {
		t.Errorf("expected at least 2 total redactions, got %d", r.Stats.TotalRedacted)
	}
	if r.Stats.ByCategory["api_key"] < 1 {
		t.Errorf("expected at least 1 api_key redaction, got %d", r.Stats.ByCategory["api_key"])
	}
	if r.Stats.ByCategory["token"] < 1 {
		t.Errorf("expected at least 1 token redaction, got %d", r.Stats.ByCategory["token"])
	}
}

func TestConcurrentRedaction(t *testing.T) {
	r := NewOutputRedactor()
	var wg sync.WaitGroup

	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r.Redact("key: AKIAIOSFODNN7EXAMPLE1")
		}()
	}

	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r.IsClean("test output")
		}()
	}

	wg.Wait()

	if r.Stats.TotalRedacted < 50 {
		t.Errorf("expected at least 50 redactions from concurrent calls, got %d", r.Stats.TotalRedacted)
	}
}

func TestRedactNoFalsePositives(t *testing.T) {
	r := NewOutputRedactor()
	inputs := []string{
		"normal log line with no secrets",
		"file: /usr/local/bin/app",
		"status code: 200 OK",
		"the variable is set to true",
		"short key: sk-short",
		"AKI is a name, not a key",
	}

	for _, input := range inputs {
		result := r.Redact(input)
		if result != input {
			t.Errorf("false positive redaction on: %q -> %q", input, result)
		}
	}
}

func TestRedactEnvAssignment(t *testing.T) {
	r := NewOutputRedactor()
	input := `export PASSWORD="my-very-secret-password-here"`
	result := r.Redact(input)
	if !strings.Contains(result, "[REDACTED:password]") {
		t.Errorf("password assignment not redacted: %s", result)
	}
}

func TestRedactStripeKey(t *testing.T) {
	r := NewOutputRedactor()
	input := "stripe key: " + "sk_" + "live_FAKETESTONLY00000000000000"
	result := r.Redact(input)
	if !strings.Contains(result, "[REDACTED:api_key]") {
		t.Errorf("Stripe key not redacted: %s", result)
	}
}

func TestFormatBytesHelper(t *testing.T) {
	tests := []struct {
		input    int
		expected string
	}{
		{0, "0"},
		{100, "100"},
		{1000, "1,000"},
		{1240, "1,240"},
		{12345, "12,345"},
		{1234567, "1,234,567"},
	}

	for _, tt := range tests {
		result := formatBytes(tt.input)
		if result != tt.expected {
			t.Errorf("formatBytes(%d) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

func TestRedactNPMToken(t *testing.T) {
	r := NewOutputRedactor()
	input := "NPM_TOKEN=npm_abcdefghijklmnopqrstuvwxyz1234567890"
	result := r.Redact(input)
	if !strings.Contains(result, "[REDACTED:token]") {
		t.Errorf("npm token not redacted: %s", result)
	}
}

func TestRedactBasicAuth(t *testing.T) {
	r := NewOutputRedactor()
	input := "Authorization: Basic dXNlcjpwYXNzd29yZA=="
	result := r.Redact(input)
	if !strings.Contains(result, "[REDACTED:token]") {
		t.Errorf("basic auth not redacted: %s", result)
	}
}
