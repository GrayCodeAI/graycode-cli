package repomap

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestNewOwnershipMap(t *testing.T) {
	om := NewOwnershipMap()
	if om == nil {
		t.Fatal("NewOwnershipMap returned nil")
	}
	if om.Owners == nil {
		t.Fatal("Owners map is nil")
	}
	if len(om.Owners) != 0 {
		t.Errorf("expected empty Owners map, got %d entries", len(om.Owners))
	}
	if len(om.Rules) != 0 {
		t.Errorf("expected empty Rules, got %d entries", len(om.Rules))
	}
}

func TestGetOwner_NoData(t *testing.T) {
	om := NewOwnershipMap()
	result := om.GetOwner("src/main.go")
	if result != nil {
		t.Errorf("expected nil for untracked file, got %+v", result)
	}
}

func TestGetOwner_DirectMatch(t *testing.T) {
	om := NewOwnershipMap()
	om.Owners["src/auth/login.go"] = &FileOwnership{
		Path:         "src/auth/login.go",
		PrimaryOwner: "alice",
		Contributors: []Contributor{
			{Name: "alice", Commits: 10, Percentage: 66.7},
			{Name: "bob", Commits: 5, Percentage: 33.3},
		},
		TotalCommits: 15,
	}

	fo := om.GetOwner("src/auth/login.go")
	if fo == nil {
		t.Fatal("expected FileOwnership, got nil")
	}
	if fo.PrimaryOwner != "alice" {
		t.Errorf("expected primary owner 'alice', got %q", fo.PrimaryOwner)
	}
	if fo.TotalCommits != 15 {
		t.Errorf("expected 15 total commits, got %d", fo.TotalCommits)
	}
}

func TestGetOwner_CodeownersRule(t *testing.T) {
	om := NewOwnershipMap()
	om.Rules = []OwnerRule{
		{Pattern: "*.go", Owners: []string{"goTeam"}, Source: "codeowners"},
		{Pattern: "src/auth/", Owners: []string{"securityTeam"}, Source: "codeowners"},
	}

	// Test glob match on extension
	fo := om.GetOwner("pkg/util.go")
	if fo == nil {
		t.Fatal("expected FileOwnership from codeowners rule, got nil")
	}
	if fo.PrimaryOwner != "goTeam" {
		t.Errorf("expected owner 'goTeam', got %q", fo.PrimaryOwner)
	}

	// Test directory match
	fo = om.GetOwner("src/auth/token.go")
	if fo == nil {
		t.Fatal("expected FileOwnership from codeowners dir rule, got nil")
	}
	if fo.PrimaryOwner != "securityTeam" {
		t.Errorf("expected owner 'securityTeam', got %q", fo.PrimaryOwner)
	}
}

func TestLoadCodeowners(t *testing.T) {
	// Create a temporary CODEOWNERS file
	tmpDir := t.TempDir()
	codeownersPath := filepath.Join(tmpDir, "CODEOWNERS")
	content := `# This is a comment
*.go @goTeam
src/auth/ @alice @bob
src/api/** @carol

# Another comment
docs/ @docTeam
`
	if err := os.WriteFile(codeownersPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	om := NewOwnershipMap()
	err := om.LoadCodeowners(codeownersPath)
	if err != nil {
		t.Fatalf("LoadCodeowners failed: %v", err)
	}

	if len(om.Rules) != 4 {
		t.Fatalf("expected 4 rules, got %d", len(om.Rules))
	}

	// Check first rule
	if om.Rules[0].Pattern != "*.go" {
		t.Errorf("rule 0 pattern: expected '*.go', got %q", om.Rules[0].Pattern)
	}
	if len(om.Rules[0].Owners) != 1 || om.Rules[0].Owners[0] != "goTeam" {
		t.Errorf("rule 0 owners: expected [goTeam], got %v", om.Rules[0].Owners)
	}
	if om.Rules[0].Source != "codeowners" {
		t.Errorf("rule 0 source: expected 'codeowners', got %q", om.Rules[0].Source)
	}

	// Check multi-owner rule
	if len(om.Rules[1].Owners) != 2 {
		t.Fatalf("rule 1 expected 2 owners, got %d", len(om.Rules[1].Owners))
	}
	if om.Rules[1].Owners[0] != "alice" || om.Rules[1].Owners[1] != "bob" {
		t.Errorf("rule 1 owners: expected [alice bob], got %v", om.Rules[1].Owners)
	}
}

func TestGetOwnersByDirectory(t *testing.T) {
	om := NewOwnershipMap()
	om.Owners["src/auth/login.go"] = &FileOwnership{
		Path:         "src/auth/login.go",
		PrimaryOwner: "alice",
		Contributors: []Contributor{
			{Name: "alice", Commits: 10, Percentage: 100},
		},
		TotalCommits: 10,
	}
	om.Owners["src/auth/token.go"] = &FileOwnership{
		Path:         "src/auth/token.go",
		PrimaryOwner: "alice",
		Contributors: []Contributor{
			{Name: "alice", Commits: 8, Percentage: 80},
			{Name: "bob", Commits: 2, Percentage: 20},
		},
		TotalCommits: 10,
	}
	om.Owners["src/api/handler.go"] = &FileOwnership{
		Path:         "src/api/handler.go",
		PrimaryOwner: "bob",
		Contributors: []Contributor{
			{Name: "bob", Commits: 15, Percentage: 100},
		},
		TotalCommits: 15,
	}

	result := om.GetOwnersByDirectory("src")
	if len(result) != 2 {
		t.Fatalf("expected 2 directories, got %d: %v", len(result), result)
	}
	if result["src/auth"] != "alice" {
		t.Errorf("expected src/auth owner 'alice', got %q", result["src/auth"])
	}
	if result["src/api"] != "bob" {
		t.Errorf("expected src/api owner 'bob', got %q", result["src/api"])
	}
}

func TestFindExpertFor(t *testing.T) {
	om := NewOwnershipMap()
	om.Owners["src/auth/login.go"] = &FileOwnership{
		Path:         "src/auth/login.go",
		PrimaryOwner: "alice",
		Contributors: []Contributor{
			{Name: "alice", Commits: 10, Percentage: 66.7},
			{Name: "bob", Commits: 5, Percentage: 33.3},
		},
		TotalCommits: 15,
	}

	// Direct file match
	expert := om.FindExpertFor("src/auth/login.go")
	if expert != "alice" {
		t.Errorf("expected expert 'alice', got %q", expert)
	}

	// Same directory fallback
	expert = om.FindExpertFor("src/auth/newfile.go")
	if expert != "alice" {
		t.Errorf("expected directory expert 'alice', got %q", expert)
	}

	// CODEOWNERS takes priority
	om.Rules = []OwnerRule{
		{Pattern: "src/auth/", Owners: []string{"securityTeam"}, Source: "codeowners"},
	}
	expert = om.FindExpertFor("src/auth/login.go")
	if expert != "securityTeam" {
		t.Errorf("expected codeowners expert 'securityTeam', got %q", expert)
	}
}

func TestSuggestReviewers(t *testing.T) {
	om := NewOwnershipMap()
	om.Owners["src/auth/login.go"] = &FileOwnership{
		Path:         "src/auth/login.go",
		PrimaryOwner: "alice",
		Contributors: []Contributor{
			{Name: "alice", Commits: 10, Percentage: 66.7},
			{Name: "bob", Commits: 5, Percentage: 33.3},
		},
		TotalCommits: 15,
	}
	om.Owners["src/api/handler.go"] = &FileOwnership{
		Path:         "src/api/handler.go",
		PrimaryOwner: "bob",
		Contributors: []Contributor{
			{Name: "bob", Commits: 12, Percentage: 80},
			{Name: "carol", Commits: 3, Percentage: 20},
		},
		TotalCommits: 15,
	}

	reviewers := om.SuggestReviewers([]string{"src/auth/login.go", "src/api/handler.go"})
	if len(reviewers) == 0 {
		t.Fatal("expected reviewers, got none")
	}

	// Bob should be first (10+5 from auth = 5 + 12 from api = 17 total)
	if reviewers[0] != "bob" {
		t.Errorf("expected top reviewer 'bob', got %q", reviewers[0])
	}
	// alice should be second (10 commits)
	if len(reviewers) < 2 || reviewers[1] != "alice" {
		t.Errorf("expected second reviewer 'alice', got %v", reviewers)
	}
}

func TestDetectBusFactorRisk(t *testing.T) {
	om := NewOwnershipMap()
	om.Owners["src/auth/token.go"] = &FileOwnership{
		Path:         "src/auth/token.go",
		PrimaryOwner: "alice",
		Contributors: []Contributor{
			{Name: "alice", Commits: 20, Percentage: 100},
		},
		TotalCommits: 20,
	}
	om.Owners["src/api/handler.go"] = &FileOwnership{
		Path:         "src/api/handler.go",
		PrimaryOwner: "bob",
		Contributors: []Contributor{
			{Name: "bob", Commits: 10, Percentage: 66.7},
			{Name: "carol", Commits: 5, Percentage: 33.3},
		},
		TotalCommits: 15,
	}
	om.Owners["src/config/config.go"] = &FileOwnership{
		Path:         "src/config/config.go",
		PrimaryOwner: "carol",
		Contributors: []Contributor{
			{Name: "carol", Commits: 8, Percentage: 100},
		},
		TotalCommits: 8,
	}

	risks := om.DetectBusFactorRisk()
	if len(risks) != 2 {
		t.Fatalf("expected 2 bus factor risks, got %d: %v", len(risks), risks)
	}

	// Should be sorted alphabetically
	if !strings.Contains(risks[0], "src/auth/token.go") {
		t.Errorf("expected first risk to mention src/auth/token.go, got %q", risks[0])
	}
	if !strings.Contains(risks[0], "@alice") {
		t.Errorf("expected first risk to mention @alice, got %q", risks[0])
	}
	if !strings.Contains(risks[1], "src/config/config.go") {
		t.Errorf("expected second risk to mention src/config/config.go, got %q", risks[1])
	}
}

func TestFormatOwnership(t *testing.T) {
	om := NewOwnershipMap()
	om.Owners["src/auth/login.go"] = &FileOwnership{
		Path:         "src/auth/login.go",
		PrimaryOwner: "alice",
		Contributors: []Contributor{
			{Name: "alice", Commits: 20, Percentage: 100},
		},
		TotalCommits: 20,
	}
	om.Owners["src/api/handler.go"] = &FileOwnership{
		Path:         "src/api/handler.go",
		PrimaryOwner: "bob",
		Contributors: []Contributor{
			{Name: "bob", Commits: 10, Percentage: 100},
		},
		TotalCommits: 10,
	}

	output := om.FormatOwnership(5)

	if !strings.Contains(output, "Code Ownership:") {
		t.Error("output missing header")
	}
	if !strings.Contains(output, "────────────────────────────────") {
		t.Error("output missing separator line")
	}
	if !strings.Contains(output, "@alice") {
		t.Error("output missing @alice")
	}
	if !strings.Contains(output, "@bob") {
		t.Error("output missing @bob")
	}
	// alice has 20/30 = 67% commits
	if !strings.Contains(output, "67%") {
		t.Errorf("output missing expected percentage, got:\n%s", output)
	}
}

func TestFormatOwnership_UnownedFiles(t *testing.T) {
	om := NewOwnershipMap()
	om.Owners["src/legacy/old.go"] = &FileOwnership{
		Path:         "src/legacy/old.go",
		PrimaryOwner: "",
		Contributors: []Contributor{},
		TotalCommits: 0,
	}

	output := om.FormatOwnership(5)
	if !strings.Contains(output, "Unowned files: 1") {
		t.Errorf("expected unowned files section, got:\n%s", output)
	}
	if !strings.Contains(output, "src/legacy/old.go") {
		t.Errorf("expected unowned file path, got:\n%s", output)
	}
}

func TestMatchGlobPattern(t *testing.T) {
	tests := []struct {
		pattern string
		path    string
		want    bool
	}{
		// Simple filename glob
		{"*.go", "main.go", true},
		{"*.go", "src/main.go", true},
		{"*.go", "main.py", false},

		// Directory pattern (trailing /)
		{"src/auth/", "src/auth/login.go", true},
		{"src/auth/", "src/auth/sub/file.go", true},
		{"src/auth/", "src/api/handler.go", false},

		// Double star
		{"src/**", "src/auth/login.go", true},
		{"src/**", "src/deep/nested/file.go", true},
		{"src/**/*.go", "src/auth/login.go", true},

		// Anchored pattern (leading /)
		{"/Makefile", "Makefile", true},
		{"/Makefile", "sub/Makefile", false},

		// Question mark
		{"?.go", "a.go", true},
		{"?.go", "ab.go", false},
	}

	for _, tt := range tests {
		got := matchGlobPattern(tt.pattern, tt.path)
		if got != tt.want {
			t.Errorf("matchGlobPattern(%q, %q) = %v, want %v", tt.pattern, tt.path, got, tt.want)
		}
	}
}

func TestBuildFromGitHistory(t *testing.T) {
	// Create a temporary git repo with some history
	tmpDir := t.TempDir()

	// Initialize git repo
	runGit := func(args ...string) {
		t.Helper()
		cmd := strings.Join(args, " ")
		_ = cmd
		c := newGitCmd(tmpDir, args...)
		out, err := c.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v failed: %v\n%s", args, err, out)
		}
	}

	runGit("init")
	runGit("config", "user.email", "alice@example.com")
	runGit("config", "user.name", "alice")

	// Create a file and commit
	if err := os.WriteFile(filepath.Join(tmpDir, "main.go"), []byte("package main"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit("add", "main.go")
	runGit("commit", "-m", "initial commit")

	// Second commit by same author
	if err := os.WriteFile(filepath.Join(tmpDir, "util.go"), []byte("package util"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit("add", "util.go")
	runGit("commit", "-m", "add util")

	// Change author and commit
	runGit("config", "user.name", "bob")
	runGit("config", "user.email", "bob@example.com")
	if err := os.WriteFile(filepath.Join(tmpDir, "main.go"), []byte("package main\nfunc main() {}"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit("add", "main.go")
	runGit("commit", "-m", "update main")

	// Build ownership map
	om := NewOwnershipMap()
	err := om.BuildFromGitHistory(tmpDir)
	if err != nil {
		t.Fatalf("BuildFromGitHistory failed: %v", err)
	}

	// Check main.go has 2 contributors
	mainOwnership := om.GetOwner("main.go")
	if mainOwnership == nil {
		t.Fatal("expected ownership for main.go")
	}
	if len(mainOwnership.Contributors) != 2 {
		t.Errorf("expected 2 contributors for main.go, got %d", len(mainOwnership.Contributors))
	}
	if mainOwnership.TotalCommits != 2 {
		t.Errorf("expected 2 total commits for main.go, got %d", mainOwnership.TotalCommits)
	}

	// util.go should have only alice
	utilOwnership := om.GetOwner("util.go")
	if utilOwnership == nil {
		t.Fatal("expected ownership for util.go")
	}
	if utilOwnership.PrimaryOwner != "alice" {
		t.Errorf("expected primary owner 'alice' for util.go, got %q", utilOwnership.PrimaryOwner)
	}
}

func newGitCmd(dir string, args ...string) *gitTestCmd {
	return &gitTestCmd{dir: dir, args: args}
}

type gitTestCmd struct {
	dir  string
	args []string
}

func (g *gitTestCmd) CombinedOutput() ([]byte, error) {
	cmd := exec.CommandContext(context.Background(), "git", g.args...)
	cmd.Dir = g.dir
	cmd.Env = append(
		os.Environ(),
		"GIT_CONFIG_COUNT=2",
		"GIT_CONFIG_KEY_0=commit.gpgsign",
		"GIT_CONFIG_VALUE_0=false",
		"GIT_CONFIG_KEY_1=tag.gpgsign",
		"GIT_CONFIG_VALUE_1=false",
	)
	return cmd.CombinedOutput()
}

func TestSuggestReviewers_WithCodeowners(t *testing.T) {
	om := NewOwnershipMap()
	om.Rules = []OwnerRule{
		{Pattern: "src/auth/", Owners: []string{"securityTeam", "alice"}, Source: "codeowners"},
	}
	om.Owners["src/auth/login.go"] = &FileOwnership{
		Path:         "src/auth/login.go",
		PrimaryOwner: "bob",
		Contributors: []Contributor{
			{Name: "bob", Commits: 3, Percentage: 100},
		},
		TotalCommits: 3,
	}

	reviewers := om.SuggestReviewers([]string{"src/auth/login.go"})
	if len(reviewers) == 0 {
		t.Fatal("expected reviewers")
	}
	// securityTeam gets weight 2 from CODEOWNERS, bob gets weight 3 from history
	// Both should appear
	found := make(map[string]bool)
	for _, r := range reviewers {
		found[r] = true
	}
	if !found["securityTeam"] {
		t.Error("expected securityTeam in reviewers")
	}
	if !found["bob"] {
		t.Error("expected bob in reviewers")
	}
}

func TestGetOwnersByDirectory_RootFilter(t *testing.T) {
	om := NewOwnershipMap()
	om.Owners["src/auth/login.go"] = &FileOwnership{
		Path:         "src/auth/login.go",
		PrimaryOwner: "alice",
		Contributors: []Contributor{
			{Name: "alice", Commits: 5, Percentage: 100},
		},
		TotalCommits: 5,
	}
	om.Owners["pkg/util/helper.go"] = &FileOwnership{
		Path:         "pkg/util/helper.go",
		PrimaryOwner: "bob",
		Contributors: []Contributor{
			{Name: "bob", Commits: 3, Percentage: 100},
		},
		TotalCommits: 3,
	}

	// Filter to only src
	result := om.GetOwnersByDirectory("src")
	if len(result) != 1 {
		t.Fatalf("expected 1 directory under src, got %d: %v", len(result), result)
	}
	if result["src/auth"] != "alice" {
		t.Errorf("expected src/auth owner 'alice', got %q", result["src/auth"])
	}

	// Filter to "." should return all
	result = om.GetOwnersByDirectory(".")
	if len(result) != 2 {
		t.Fatalf("expected 2 directories, got %d: %v", len(result), result)
	}
}

func TestDetectBusFactorRisk_NoRisk(t *testing.T) {
	om := NewOwnershipMap()
	om.Owners["src/shared.go"] = &FileOwnership{
		Path:         "src/shared.go",
		PrimaryOwner: "alice",
		Contributors: []Contributor{
			{Name: "alice", Commits: 5, Percentage: 50},
			{Name: "bob", Commits: 5, Percentage: 50},
		},
		TotalCommits: 10,
	}

	risks := om.DetectBusFactorRisk()
	if len(risks) != 0 {
		t.Errorf("expected no bus factor risks, got %d: %v", len(risks), risks)
	}
}
