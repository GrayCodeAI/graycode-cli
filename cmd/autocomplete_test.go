package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCompleteSlashCommand(t *testing.T) {
	ac := &Autocompleter{
		SlashCommands: []string{"/commit", "/compact", "/config", "/context", "/copy", "/cost", "/council"},
		usageCount:    make(map[string]int),
		fileMTimes:    make(map[string]time.Time),
	}

	suggestions := ac.CompleteSlashCommand("/com")
	if len(suggestions) == 0 {
		t.Fatal("expected suggestions for /com prefix")
	}

	// Should match /commit, /compact, /compress
	found := make(map[string]bool)
	for _, s := range suggestions {
		found[s.Text] = true
		if s.Category != "command" {
			t.Errorf("expected category 'command', got %q for %q", s.Category, s.Text)
		}
	}

	if !found["/commit"] {
		t.Error("expected /commit in suggestions")
	}
	if !found["/compact"] {
		t.Error("expected /compact in suggestions")
	}
}

func TestCompleteSlashCommandWithDescription(t *testing.T) {
	ac := &Autocompleter{
		SlashCommands: []string{"/commit", "/compact"},
		usageCount:    make(map[string]int),
		fileMTimes:    make(map[string]time.Time),
	}

	suggestions := ac.CompleteSlashCommand("/commit")
	if len(suggestions) == 0 {
		t.Fatal("expected suggestions for /commit")
	}

	for _, s := range suggestions {
		if s.Text == "/commit" && s.Description == "" {
			t.Error("expected description for /commit")
		}
	}
}

func TestCompleteSlashCommandUsageBoost(t *testing.T) {
	ac := &Autocompleter{
		SlashCommands: []string{"/commit", "/compact"},
		usageCount:    map[string]int{"/compact": 10},
		fileMTimes:    make(map[string]time.Time),
	}

	suggestions := ac.CompleteSlashCommand("/com")
	if len(suggestions) < 2 {
		t.Fatal("expected at least 2 suggestions")
	}

	// After ranking, /compact should be boosted
	ranked := ac.RankSuggestions(suggestions)
	if ranked[0].Text != "/compact" {
		t.Errorf("expected /compact to be ranked first due to usage boost, got %q", ranked[0].Text)
	}
}

func TestCompleteFilePath(t *testing.T) {
	ac := &Autocompleter{
		Files: []string{
			"cmd/autocomplete.go",
			"cmd/autocomplete_test.go",
			"cmd/root.go",
			"main.go",
			"engine/loop.go",
		},
		usageCount: make(map[string]int),
		fileMTimes: make(map[string]time.Time),
	}

	suggestions := ac.CompleteFilePath("cmd/auto")
	if len(suggestions) == 0 {
		t.Fatal("expected file suggestions for 'cmd/auto'")
	}

	found := false
	for _, s := range suggestions {
		if s.Text == "cmd/autocomplete.go" {
			found = true
			if s.Category != "file" {
				t.Errorf("expected category 'file', got %q", s.Category)
			}
		}
	}
	if !found {
		t.Error("expected cmd/autocomplete.go in suggestions")
	}
}

func TestCompleteFilePathRecentlyModified(t *testing.T) {
	now := time.Now()
	ac := &Autocompleter{
		Files: []string{
			"old_file.go",
			"new_file.go",
		},
		usageCount: make(map[string]int),
		fileMTimes: map[string]time.Time{
			"old_file.go": now.Add(-30 * 24 * time.Hour), // 30 days old
			"new_file.go": now.Add(-10 * time.Minute),    // 10 minutes old
		},
	}

	suggestions := ac.CompleteFilePath("")
	ranked := ac.RankSuggestions(suggestions)

	if len(ranked) < 2 {
		t.Fatal("expected at least 2 suggestions")
	}

	// new_file.go should be ranked higher
	if ranked[0].Text != "new_file.go" {
		t.Errorf("expected new_file.go first (recently modified), got %q", ranked[0].Text)
	}
}

func TestCompleteFromHistory(t *testing.T) {
	ac := &Autocompleter{
		History:    []string{"go test ./...", "go build", "git status", "go test -race ./..."},
		usageCount: make(map[string]int),
		fileMTimes: make(map[string]time.Time),
	}

	suggestions := ac.CompleteFromHistory("go")
	if len(suggestions) == 0 {
		t.Fatal("expected history suggestions for 'go'")
	}

	// Should return most recent first
	if suggestions[0].Text != "go test -race ./..." {
		t.Errorf("expected most recent 'go' entry first, got %q", suggestions[0].Text)
	}

	for _, s := range suggestions {
		if s.Category != "history" {
			t.Errorf("expected category 'history', got %q", s.Category)
		}
	}
}

func TestCompleteFromHistoryDeduplication(t *testing.T) {
	ac := &Autocompleter{
		History:    []string{"go test", "go build", "go test", "go test"},
		usageCount: make(map[string]int),
		fileMTimes: make(map[string]time.Time),
	}

	suggestions := ac.CompleteFromHistory("go")
	// "go test" should appear only once
	count := 0
	for _, s := range suggestions {
		if s.Text == "go test" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("expected 'go test' to appear once (deduplicated), appeared %d times", count)
	}
}

func TestFuzzyMatchExactPrefix(t *testing.T) {
	matched, score := FuzzyMatch("/com", "/commit")
	if !matched {
		t.Fatal("expected /com to match /commit")
	}
	if score <= 0 {
		t.Error("expected positive score for prefix match")
	}

	// Exact prefix should score higher than non-prefix subsequence
	_, prefixScore := FuzzyMatch("/com", "/commit")
	_, subseqScore := FuzzyMatch("/com", "/council")
	if prefixScore <= subseqScore {
		t.Errorf("prefix match should score higher: prefix=%f, subsequence=%f", prefixScore, subseqScore)
	}
}

func TestFuzzyMatchSubsequence(t *testing.T) {
	matched, score := FuzzyMatch("gts", "go test ./...")
	if !matched {
		t.Fatal("expected 'gts' to match 'go test ./...' as subsequence")
	}
	if score <= 0 {
		t.Error("expected positive score for subsequence match")
	}
}

func TestFuzzyMatchNoMatch(t *testing.T) {
	matched, _ := FuzzyMatch("xyz", "/commit")
	if matched {
		t.Error("expected no match for 'xyz' against '/commit'")
	}
}

func TestFuzzyMatchEmptyInput(t *testing.T) {
	matched, score := FuzzyMatch("", "/commit")
	if !matched {
		t.Error("empty input should match everything")
	}
	if score != 0.0 {
		t.Error("empty input should have zero score")
	}
}

func TestFuzzyMatchEmptyCandidate(t *testing.T) {
	matched, _ := FuzzyMatch("abc", "")
	if matched {
		t.Error("should not match empty candidate")
	}
}

func TestFuzzyMatchConsecutiveBonus(t *testing.T) {
	// "com" in "/commit" has consecutive matches
	_, consecutiveScore := FuzzyMatch("com", "/commit")
	// "cmt" in "/commit" has non-consecutive
	_, nonConsecutiveScore := FuzzyMatch("cmt", "/commit")

	if consecutiveScore <= nonConsecutiveScore {
		t.Errorf("consecutive matches should score higher: consecutive=%f, non-consecutive=%f",
			consecutiveScore, nonConsecutiveScore)
	}
}

func TestRankSuggestions(t *testing.T) {
	ac := &Autocompleter{
		usageCount: make(map[string]int),
		fileMTimes: make(map[string]time.Time),
	}

	suggestions := []Suggestion{
		{Text: "beta", Score: 0.5},
		{Text: "alpha", Score: 0.9},
		{Text: "gamma", Score: 0.5},
		{Text: "delta", Score: 0.8},
	}

	ranked := ac.RankSuggestions(suggestions)

	// Should be sorted by score desc, then alphabetical
	expected := []string{"alpha", "delta", "beta", "gamma"}
	for i, exp := range expected {
		if ranked[i].Text != exp {
			t.Errorf("position %d: expected %q, got %q", i, exp, ranked[i].Text)
		}
	}
}

func TestRankSuggestionsEmpty(t *testing.T) {
	ac := &Autocompleter{
		usageCount: make(map[string]int),
		fileMTimes: make(map[string]time.Time),
	}
	ranked := ac.RankSuggestions(nil)
	if len(ranked) != 0 {
		t.Error("expected empty result for nil input")
	}
}

func TestCompleteEmptyInput(t *testing.T) {
	ac := &Autocompleter{
		SlashCommands: []string{"/commit", "/compact"},
		Files:         []string{"main.go"},
		usageCount:    make(map[string]int),
		fileMTimes:    make(map[string]time.Time),
	}

	suggestions := ac.Complete("", 0)
	if len(suggestions) != 0 {
		t.Error("expected no suggestions for empty input")
	}
}

func TestCompleteAtPrefixTriggersFileCompletion(t *testing.T) {
	ac := &Autocompleter{
		SlashCommands: []string{"/commit"},
		Files:         []string{"main.go", "cmd/root.go"},
		usageCount:    make(map[string]int),
		fileMTimes:    make(map[string]time.Time),
	}

	suggestions := ac.Complete("@main", 5)
	if len(suggestions) == 0 {
		t.Fatal("expected file suggestions for @ prefix")
	}

	found := false
	for _, s := range suggestions {
		if s.Text == "main.go" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected main.go in suggestions for @main")
	}
}

func TestCompleteSlashPrefixTriggersCommandCompletion(t *testing.T) {
	ac := &Autocompleter{
		SlashCommands: []string{"/commit", "/compact", "/config"},
		Files:         []string{"main.go"},
		usageCount:    make(map[string]int),
		fileMTimes:    make(map[string]time.Time),
	}

	suggestions := ac.Complete("/con", 4)
	found := false
	for _, s := range suggestions {
		if s.Text == "/config" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected /config in suggestions for /con")
	}
}

func TestCompleteFlagPrefix(t *testing.T) {
	ac := &Autocompleter{
		SlashCommands: []string{"/commit"},
		usageCount:    make(map[string]int),
		fileMTimes:    make(map[string]time.Time),
	}

	suggestions := ac.Complete("--mod", 5)
	found := false
	for _, s := range suggestions {
		if s.Text == "--model" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected --model in flag suggestions")
	}
}

func TestRecordInput(t *testing.T) {
	ac := &Autocompleter{
		usageCount: make(map[string]int),
		fileMTimes: make(map[string]time.Time),
	}

	ac.RecordInput("go test ./...")
	ac.RecordInput("/commit")
	ac.RecordInput("  ")      // should be ignored (whitespace only)
	ac.RecordInput("/commit") // second usage

	if len(ac.History) != 3 { // "go test", "/commit", "/commit"
		t.Errorf("expected 3 history entries, got %d", len(ac.History))
	}

	if ac.usageCount["/commit"] != 2 {
		t.Errorf("expected usage count 2 for /commit, got %d", ac.usageCount["/commit"])
	}
}

func TestRefreshFiles(t *testing.T) {
	// Create a temp directory with some files
	tmpDir := t.TempDir()
	_ = os.WriteFile(filepath.Join(tmpDir, "main.go"), []byte("package main"), 0o644)
	_ = os.MkdirAll(filepath.Join(tmpDir, "sub"), 0o755)
	_ = os.WriteFile(filepath.Join(tmpDir, "sub", "helper.go"), []byte("package sub"), 0o644)
	// Hidden file should be skipped
	_ = os.WriteFile(filepath.Join(tmpDir, ".hidden"), []byte(""), 0o644)
	// Hidden directory should be skipped
	_ = os.MkdirAll(filepath.Join(tmpDir, ".git"), 0o755)
	_ = os.WriteFile(filepath.Join(tmpDir, ".git", "config"), []byte(""), 0o644)

	ac := GetAutocompleter(tmpDir)

	if len(ac.Files) < 2 {
		t.Fatalf("expected at least 2 files, got %d: %v", len(ac.Files), ac.Files)
	}

	foundMain := false
	foundHelper := false
	foundHidden := false
	foundGit := false
	for _, f := range ac.Files {
		if f == "main.go" {
			foundMain = true
		}
		if f == filepath.Join("sub", "helper.go") {
			foundHelper = true
		}
		if strings.Contains(f, ".hidden") {
			foundHidden = true
		}
		if strings.Contains(f, ".git") {
			foundGit = true
		}
	}

	if !foundMain {
		t.Error("expected main.go in files")
	}
	if !foundHelper {
		t.Error("expected sub/helper.go in files")
	}
	if foundHidden {
		t.Error("hidden file should be excluded")
	}
	if foundGit {
		t.Error(".git directory should be excluded")
	}
}

func TestFormatSuggestions(t *testing.T) {
	suggestions := []Suggestion{
		{Text: "/commit", Description: "Auto-commit with AI message"},
		{Text: "/compact", Description: "Compact context window"},
		{Text: "/config", Description: "Open config panel"},
	}

	output := FormatSuggestions(suggestions, 10)
	if output == "" {
		t.Fatal("expected non-empty output")
	}

	lines := strings.Split(strings.TrimRight(output, "\n"), "\n")
	if len(lines) != 3 {
		t.Errorf("expected 3 lines, got %d", len(lines))
	}

	// Check alignment - all descriptions should start at the same column
	for _, line := range lines {
		if !strings.Contains(line, "/") {
			t.Errorf("expected slash command in line: %q", line)
		}
	}

	// Check that descriptions are present
	if !strings.Contains(output, "Auto-commit with AI message") {
		t.Error("expected description in output")
	}
}

func TestFormatSuggestionsMaxDisplay(t *testing.T) {
	suggestions := make([]Suggestion, 20)
	for i := range suggestions {
		suggestions[i] = Suggestion{Text: "/cmd" + string(rune('a'+i)), Description: "desc"}
	}

	output := FormatSuggestions(suggestions, 5)
	lines := strings.Split(strings.TrimRight(output, "\n"), "\n")
	if len(lines) != 5 {
		t.Errorf("expected 5 lines with maxDisplay=5, got %d", len(lines))
	}
}

func TestFormatSuggestionsEmpty(t *testing.T) {
	output := FormatSuggestions(nil, 10)
	if output != "" {
		t.Error("expected empty output for nil suggestions")
	}
}

func TestCompleteToolContext(t *testing.T) {
	ac := &Autocompleter{
		SlashCommands: []string{"/run", "/tools"},
		Tools:         []string{"bash", "read_file", "write_file", "search"},
		usageCount:    make(map[string]int),
		fileMTimes:    make(map[string]time.Time),
	}

	// After "/run " we should get tool suggestions
	suggestions := ac.Complete("/run ba", 7)
	found := false
	for _, s := range suggestions {
		if s.Text == "bash" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected 'bash' in tool suggestions after '/run '")
	}
}

func TestExtractCurrentToken(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"", ""},
		{"/commit", "/commit"},
		{"hello world", "world"},
		{"@main", "@main"},
		{"go test ", ""},
		{"/run bash", "bash"},
	}

	for _, tt := range tests {
		got := extractCurrentToken(tt.input)
		if got != tt.expected {
			t.Errorf("extractCurrentToken(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestNewAutocompleterInitializesSlashCommands(t *testing.T) {
	tmpDir := t.TempDir()
	ac := GetAutocompleter(tmpDir)

	if len(ac.SlashCommands) == 0 {
		t.Error("expected slash commands to be initialized")
	}

	// Should contain the well-known commands
	found := false
	for _, cmd := range ac.SlashCommands {
		if cmd == "/commit" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected /commit in initialized slash commands")
	}
}
