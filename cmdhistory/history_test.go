package cmdhistory

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func tempDB(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	return filepath.Join(dir, "history.db")
}

func mustNew(t *testing.T) *Store {
	t.Helper()
	store, err := New(tempDB(t))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { store.Close() })
	return store
}

func intPtr(v int) *int { return &v }

// --- basic record and retrieve ---

func TestNewCreatesDatabase(t *testing.T) {
	dbPath := tempDB(t)
	store, err := New(dbPath)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer store.Close()

	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		t.Fatal("expected database file to be created")
	}
}

func TestRecordAndRecent(t *testing.T) {
	store := mustNew(t)

	err := store.Record(Entry{
		Command:   "go build ./...",
		ExitCode:  0,
		Duration:  2 * time.Second,
		CWD:       "/home/user/project",
		GitBranch: "main",
		SessionID: "sess-1",
	})
	if err != nil {
		t.Fatalf("Record: %v", err)
	}

	entries, err := store.Recent(10)
	if err != nil {
		t.Fatalf("Recent: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}

	e := entries[0]
	if e.Command != "go build ./..." {
		t.Fatalf("expected command 'go build ./...', got %q", e.Command)
	}
	if e.ExitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", e.ExitCode)
	}
	if e.Duration != 2*time.Second {
		t.Fatalf("expected duration 2s, got %v", e.Duration)
	}
	if e.CWD != "/home/user/project" {
		t.Fatalf("expected cwd '/home/user/project', got %q", e.CWD)
	}
	if e.GitBranch != "main" {
		t.Fatalf("expected branch 'main', got %q", e.GitBranch)
	}
	if e.SessionID != "sess-1" {
		t.Fatalf("expected session 'sess-1', got %q", e.SessionID)
	}
	if len(e.ID) != 8 {
		t.Fatalf("expected 8-char ID, got %q (len %d)", e.ID, len(e.ID))
	}
}

func TestRecordAutoGeneratesIDAndTimestamp(t *testing.T) {
	store := mustNew(t)

	err := store.Record(Entry{Command: "ls"})
	if err != nil {
		t.Fatalf("Record: %v", err)
	}

	entries, err := store.Recent(1)
	if err != nil {
		t.Fatalf("Recent: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].ID == "" {
		t.Fatal("expected auto-generated ID")
	}
	if entries[0].CreatedAt.IsZero() {
		t.Fatal("expected auto-generated timestamp")
	}
}

// --- FTS5 search ---

func TestSearchFTS5Basic(t *testing.T) {
	store := mustNew(t)

	commands := []string{
		"go test ./...",
		"go build -v ./cmd/server",
		"git status",
		"git commit -m 'fix bug'",
		"docker compose up -d",
	}
	for _, cmd := range commands {
		if err := store.Record(Entry{Command: cmd}); err != nil {
			t.Fatalf("Record %q: %v", cmd, err)
		}
	}

	results, err := store.Search("go", SearchOpts{})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results for 'go', got %d", len(results))
	}

	results, err = store.Search("git", SearchOpts{})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results for 'git', got %d", len(results))
	}

	results, err = store.Search("docker", SearchOpts{})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result for 'docker', got %d", len(results))
	}
}

func TestSearchFTS5PrefixMatch(t *testing.T) {
	store := mustNew(t)

	if err := store.Record(Entry{Command: "docker compose up -d"}); err != nil {
		t.Fatalf("Record: %v", err)
	}
	if err := store.Record(Entry{Command: "docker build ."}); err != nil {
		t.Fatalf("Record: %v", err)
	}

	// "dock" should match both via prefix expansion.
	results, err := store.Search("dock", SearchOpts{})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results for 'dock', got %d", len(results))
	}
}

func TestSearchFTS5MultipleWords(t *testing.T) {
	store := mustNew(t)

	if err := store.Record(Entry{Command: "go test -race ./..."}); err != nil {
		t.Fatalf("Record: %v", err)
	}
	if err := store.Record(Entry{Command: "go build ./cmd/server"}); err != nil {
		t.Fatalf("Record: %v", err)
	}
	if err := store.Record(Entry{Command: "npm test"}); err != nil {
		t.Fatalf("Record: %v", err)
	}

	// "go test" should match only the first entry.
	results, err := store.Search("go test", SearchOpts{})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result for 'go test', got %d", len(results))
	}
	if results[0].Command != "go test -race ./..." {
		t.Fatalf("expected 'go test -race ./...', got %q", results[0].Command)
	}
}

func TestSearchEmptyQuery(t *testing.T) {
	store := mustNew(t)

	results, err := store.Search("", SearchOpts{})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if results != nil {
		t.Fatalf("expected nil for empty query, got %d results", len(results))
	}
}

// --- filter by exit code ---

func TestSearchFilterByExitCode(t *testing.T) {
	store := mustNew(t)

	if err := store.Record(Entry{Command: "go test ./...", ExitCode: 0}); err != nil {
		t.Fatalf("Record: %v", err)
	}
	if err := store.Record(Entry{Command: "go test -race ./...", ExitCode: 1}); err != nil {
		t.Fatalf("Record: %v", err)
	}
	if err := store.Record(Entry{Command: "go vet ./...", ExitCode: 0}); err != nil {
		t.Fatalf("Record: %v", err)
	}

	// Search for "go" with exit code 0.
	results, err := store.Search("go", SearchOpts{ExitCode: intPtr(0)})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results with exit code 0, got %d", len(results))
	}
	for _, r := range results {
		if r.ExitCode != 0 {
			t.Fatalf("expected exit code 0, got %d for %q", r.ExitCode, r.Command)
		}
	}

	// Search for "go" with exit code 1.
	results, err = store.Search("go", SearchOpts{ExitCode: intPtr(1)})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result with exit code 1, got %d", len(results))
	}
}

// --- filter by CWD ---

func TestSearchFilterByCWD(t *testing.T) {
	store := mustNew(t)

	if err := store.Record(Entry{Command: "make build", CWD: "/home/user/projectA"}); err != nil {
		t.Fatalf("Record: %v", err)
	}
	if err := store.Record(Entry{Command: "make test", CWD: "/home/user/projectB"}); err != nil {
		t.Fatalf("Record: %v", err)
	}
	if err := store.Record(Entry{Command: "make lint", CWD: "/home/user/projectA"}); err != nil {
		t.Fatalf("Record: %v", err)
	}

	results, err := store.Search("make", SearchOpts{CWD: "/home/user/projectA"})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results in projectA, got %d", len(results))
	}
	for _, r := range results {
		if r.CWD != "/home/user/projectA" {
			t.Fatalf("expected CWD '/home/user/projectA', got %q", r.CWD)
		}
	}
}

// --- SearchByDir ---

func TestSearchByDir(t *testing.T) {
	store := mustNew(t)

	if err := store.Record(Entry{Command: "ls", CWD: "/tmp"}); err != nil {
		t.Fatalf("Record: %v", err)
	}
	if err := store.Record(Entry{Command: "pwd", CWD: "/tmp"}); err != nil {
		t.Fatalf("Record: %v", err)
	}
	if err := store.Record(Entry{Command: "echo hello", CWD: "/home"}); err != nil {
		t.Fatalf("Record: %v", err)
	}

	results, err := store.SearchByDir("/tmp", 10)
	if err != nil {
		t.Fatalf("SearchByDir: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results in /tmp, got %d", len(results))
	}
	for _, r := range results {
		if r.CWD != "/tmp" {
			t.Fatalf("expected CWD '/tmp', got %q", r.CWD)
		}
	}

	results, err = store.SearchByDir("/home", 10)
	if err != nil {
		t.Fatalf("SearchByDir: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result in /home, got %d", len(results))
	}
}

// --- Recent ---

func TestRecentOrdering(t *testing.T) {
	store := mustNew(t)

	base := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 5; i++ {
		err := store.Record(Entry{
			Command:   fmt.Sprintf("cmd-%d", i),
			CreatedAt: base.Add(time.Duration(i) * time.Minute),
		})
		if err != nil {
			t.Fatalf("Record: %v", err)
		}
	}

	entries, err := store.Recent(3)
	if err != nil {
		t.Fatalf("Recent: %v", err)
	}
	if len(entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(entries))
	}

	// Most recent first.
	if entries[0].Command != "cmd-4" {
		t.Fatalf("expected 'cmd-4' first, got %q", entries[0].Command)
	}
	if entries[1].Command != "cmd-3" {
		t.Fatalf("expected 'cmd-3' second, got %q", entries[1].Command)
	}
	if entries[2].Command != "cmd-2" {
		t.Fatalf("expected 'cmd-2' third, got %q", entries[2].Command)
	}
}

func TestRecentDefaultLimit(t *testing.T) {
	store := mustNew(t)

	for i := 0; i < 30; i++ {
		if err := store.Record(Entry{Command: fmt.Sprintf("cmd-%d", i)}); err != nil {
			t.Fatalf("Record: %v", err)
		}
	}

	// Default limit is 20.
	entries, err := store.Recent(0)
	if err != nil {
		t.Fatalf("Recent: %v", err)
	}
	if len(entries) != 20 {
		t.Fatalf("expected 20 entries with default limit, got %d", len(entries))
	}
}

// --- Stats ---

func TestStatsEmpty(t *testing.T) {
	store := mustNew(t)

	stats, err := store.Stats()
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}
	if stats.TotalCommands != 0 {
		t.Fatalf("expected 0 total, got %d", stats.TotalCommands)
	}
	if stats.UniqueCommands != 0 {
		t.Fatalf("expected 0 unique, got %d", stats.UniqueCommands)
	}
	if stats.SuccessRate != 0 {
		t.Fatalf("expected 0 success rate, got %f", stats.SuccessRate)
	}
}

func TestStatsComputation(t *testing.T) {
	store := mustNew(t)

	entries := []Entry{
		{Command: "go test ./...", ExitCode: 0, CWD: "/project"},
		{Command: "go test ./...", ExitCode: 0, CWD: "/project"},
		{Command: "go test ./...", ExitCode: 1, CWD: "/project"},
		{Command: "go build ./...", ExitCode: 0, CWD: "/project"},
		{Command: "git status", ExitCode: 0, CWD: "/other"},
	}
	for _, e := range entries {
		if err := store.Record(e); err != nil {
			t.Fatalf("Record: %v", err)
		}
	}

	stats, err := store.Stats()
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}

	if stats.TotalCommands != 5 {
		t.Fatalf("expected 5 total commands, got %d", stats.TotalCommands)
	}
	if stats.UniqueCommands != 3 {
		t.Fatalf("expected 3 unique commands, got %d", stats.UniqueCommands)
	}

	// 4 out of 5 succeeded.
	expectedRate := 0.8
	if stats.SuccessRate < expectedRate-0.001 || stats.SuccessRate > expectedRate+0.001 {
		t.Fatalf("expected success rate ~0.8, got %f", stats.SuccessRate)
	}

	// Top command should be "go test ./..." with count 3.
	if len(stats.TopCommands) == 0 {
		t.Fatal("expected at least one top command")
	}
	if stats.TopCommands[0].Command != "go test ./..." {
		t.Fatalf("expected top command 'go test ./...', got %q", stats.TopCommands[0].Command)
	}
	if stats.TopCommands[0].Count != 3 {
		t.Fatalf("expected top command count 3, got %d", stats.TopCommands[0].Count)
	}

	// Top directory should be "/project" with count 4.
	if len(stats.TopDirectories) == 0 {
		t.Fatal("expected at least one top directory")
	}
	if stats.TopDirectories[0].Dir != "/project" {
		t.Fatalf("expected top dir '/project', got %q", stats.TopDirectories[0].Dir)
	}
	if stats.TopDirectories[0].Count != 4 {
		t.Fatalf("expected top dir count 4, got %d", stats.TopDirectories[0].Count)
	}
}

// --- Search with Since filter ---

func TestSearchFilterBySince(t *testing.T) {
	store := mustNew(t)

	old := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	recent := time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC)

	if err := store.Record(Entry{Command: "go test old", CreatedAt: old}); err != nil {
		t.Fatalf("Record: %v", err)
	}
	if err := store.Record(Entry{Command: "go test new", CreatedAt: recent}); err != nil {
		t.Fatalf("Record: %v", err)
	}

	cutoff := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	results, err := store.Search("go", SearchOpts{Since: cutoff})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result since 2025, got %d", len(results))
	}
	if results[0].Command != "go test new" {
		t.Fatalf("expected 'go test new', got %q", results[0].Command)
	}
}

// --- Search with session filter ---

func TestSearchFilterBySession(t *testing.T) {
	store := mustNew(t)

	if err := store.Record(Entry{Command: "make build", SessionID: "alpha"}); err != nil {
		t.Fatalf("Record: %v", err)
	}
	if err := store.Record(Entry{Command: "make test", SessionID: "beta"}); err != nil {
		t.Fatalf("Record: %v", err)
	}

	results, err := store.Search("make", SearchOpts{SessionID: "alpha"})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result for session alpha, got %d", len(results))
	}
	if results[0].Command != "make build" {
		t.Fatalf("expected 'make build', got %q", results[0].Command)
	}
}

// --- edge cases ---

func TestSearchLimitRespected(t *testing.T) {
	store := mustNew(t)

	for i := 0; i < 20; i++ {
		if err := store.Record(Entry{Command: fmt.Sprintf("git log --%d", i)}); err != nil {
			t.Fatalf("Record: %v", err)
		}
	}

	results, err := store.Search("git", SearchOpts{Limit: 5})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 5 {
		t.Fatalf("expected 5 results with limit, got %d", len(results))
	}
}

func TestDurationRoundTrip(t *testing.T) {
	store := mustNew(t)

	dur := 3*time.Second + 500*time.Millisecond
	if err := store.Record(Entry{Command: "sleep 3.5", Duration: dur}); err != nil {
		t.Fatalf("Record: %v", err)
	}

	entries, err := store.Recent(1)
	if err != nil {
		t.Fatalf("Recent: %v", err)
	}
	if entries[0].Duration != dur {
		t.Fatalf("expected duration %v, got %v", dur, entries[0].Duration)
	}
}

