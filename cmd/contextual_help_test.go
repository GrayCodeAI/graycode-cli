package cmd

import (
	"strings"
	"sync"
	"testing"
)

func TestNewContextualHelp(t *testing.T) {
	ch := NewContextualHelp()

	if ch == nil {
		t.Fatal("NewContextualHelp returned nil")
	}

	if len(ch.Entries) < 50 {
		t.Errorf("expected at least 50 help entries, got %d", len(ch.Entries))
	}
}

func TestGetHelp_ExactMatch(t *testing.T) {
	ch := NewContextualHelp()

	entry := ch.GetHelp("/commit")
	if entry == nil {
		t.Fatal("GetHelp(/commit) returned nil")
	}
	if entry.Topic != "/commit" {
		t.Errorf("expected topic /commit, got %s", entry.Topic)
	}
	if entry.Summary == "" {
		t.Error("expected non-empty summary")
	}
	if entry.Detail == "" {
		t.Error("expected non-empty detail")
	}
	if len(entry.Examples) == 0 {
		t.Error("expected at least one example")
	}
	if len(entry.Related) == 0 {
		t.Error("expected at least one related topic")
	}
	if entry.Category == "" {
		t.Error("expected non-empty category")
	}
}

func TestGetHelp_CaseInsensitive(t *testing.T) {
	ch := NewContextualHelp()

	entry := ch.GetHelp("/COMMIT")
	if entry == nil {
		t.Fatal("case-insensitive GetHelp failed")
	}
	if entry.Topic != "/commit" {
		t.Errorf("expected topic /commit, got %s", entry.Topic)
	}
}

func TestGetHelp_NotFound(t *testing.T) {
	ch := NewContextualHelp()

	entry := ch.GetHelp("nonexistent-topic-xyz")
	if entry != nil {
		t.Error("expected nil for nonexistent topic")
	}
}

func TestSearchHelp_EmptyQuery(t *testing.T) {
	ch := NewContextualHelp()

	results := ch.SearchHelp("")
	if results != nil {
		t.Error("expected nil for empty query")
	}
}

func TestSearchHelp_FindsCommit(t *testing.T) {
	ch := NewContextualHelp()

	results := ch.SearchHelp("commit")
	if len(results) == 0 {
		t.Fatal("expected results for 'commit' query")
	}

	// /commit should be in the results
	found := false
	for _, entry := range results {
		if entry.Topic == "/commit" {
			found = true
			break
		}
	}
	if !found {
		t.Error("/commit not found in search results for 'commit'")
	}
}

func TestSearchHelp_MultiTermQuery(t *testing.T) {
	ch := NewContextualHelp()

	results := ch.SearchHelp("fix test")
	if len(results) == 0 {
		t.Fatal("expected results for 'fix test' query")
	}

	// Should find "how to fix tests" or /fix or /test
	foundRelevant := false
	for _, entry := range results {
		if strings.Contains(entry.Topic, "fix") || strings.Contains(entry.Topic, "test") {
			foundRelevant = true
			break
		}
	}
	if !foundRelevant {
		t.Error("expected relevant results for 'fix test'")
	}
}

func TestSearchHelp_RankedResults(t *testing.T) {
	ch := NewContextualHelp()

	results := ch.SearchHelp("/diff")
	if len(results) == 0 {
		t.Fatal("expected results for '/diff' query")
	}

	// Exact topic match should rank first
	if results[0].Topic != "/diff" {
		t.Errorf("expected /diff to be first result, got %s", results[0].Topic)
	}
}

func TestSearchHelp_FuzzyMatch(t *testing.T) {
	ch := NewContextualHelp()

	// Search for a partial/fuzzy term
	results := ch.SearchHelp("commi")
	if len(results) == 0 {
		t.Fatal("expected fuzzy results for 'commi'")
	}

	foundCommit := false
	for _, entry := range results {
		if entry.Topic == "/commit" || strings.Contains(entry.Topic, "commit") {
			foundCommit = true
			break
		}
	}
	if !foundCommit {
		t.Error("expected to find commit-related entry with fuzzy search 'commi'")
	}
}

func TestSuggestHelp_ErrorContext(t *testing.T) {
	ch := NewContextualHelp()

	suggestions := ch.SuggestHelp("error: something failed")
	if len(suggestions) == 0 {
		t.Fatal("expected suggestions for error context")
	}

	// Should suggest fix-related commands or error entries
	foundRelevant := false
	for _, entry := range suggestions {
		if entry.Category == "errors" || entry.Topic == "/fix" || entry.Topic == "/bugfind" {
			foundRelevant = true
			break
		}
	}
	if !foundRelevant {
		t.Error("expected error-related suggestions")
	}
}

func TestSuggestHelp_SpecificError(t *testing.T) {
	ch := NewContextualHelp()

	suggestions := ch.SuggestHelp("error: rate limit exceeded")
	if len(suggestions) == 0 {
		t.Fatal("expected suggestions for rate limit error")
	}

	foundRateLimit := false
	for _, entry := range suggestions {
		if entry.Topic == "error: rate limit" {
			foundRateLimit = true
			break
		}
	}
	if !foundRateLimit {
		t.Error("expected rate limit help in suggestions")
	}
}

func TestSuggestHelp_FirstSession(t *testing.T) {
	ch := NewContextualHelp()

	suggestions := ch.SuggestHelp("first session - new user onboarding")
	if len(suggestions) == 0 {
		t.Fatal("expected onboarding suggestions")
	}

	// Should include /status and /config
	topics := make(map[string]bool)
	for _, entry := range suggestions {
		topics[entry.Topic] = true
	}

	if !topics["/status"] {
		t.Error("expected /status in onboarding suggestions")
	}
	if !topics["/config"] {
		t.Error("expected /config in onboarding suggestions")
	}
}

func TestSuggestHelp_ConfigContext(t *testing.T) {
	ch := NewContextualHelp()

	suggestions := ch.SuggestHelp("user ran /config")
	if len(suggestions) == 0 {
		t.Fatal("expected configuration suggestions")
	}

	allConfig := true
	for _, entry := range suggestions {
		if entry.Category != "configuration" {
			allConfig = false
			break
		}
	}
	if !allConfig {
		t.Error("expected all suggestions to be configuration entries")
	}
}

func TestSuggestHelp_TestContext(t *testing.T) {
	ch := NewContextualHelp()

	suggestions := ch.SuggestHelp("running tests")
	if len(suggestions) == 0 {
		t.Fatal("expected test-related suggestions")
	}

	foundTest := false
	for _, entry := range suggestions {
		if entry.Topic == "/test" {
			foundTest = true
			break
		}
	}
	if !foundTest {
		t.Error("expected /test in suggestions")
	}
}

func TestSuggestHelp_GitContext(t *testing.T) {
	ch := NewContextualHelp()

	suggestions := ch.SuggestHelp("working with git branches")
	if len(suggestions) == 0 {
		t.Fatal("expected git-related suggestions")
	}

	topics := make(map[string]bool)
	for _, entry := range suggestions {
		topics[entry.Topic] = true
	}
	if !topics["/branch"] {
		t.Error("expected /branch in git suggestions")
	}
	if !topics["/commit"] {
		t.Error("expected /commit in git suggestions")
	}
}

func TestSuggestHelp_EmptyContext(t *testing.T) {
	ch := NewContextualHelp()

	suggestions := ch.SuggestHelp("")
	if suggestions != nil {
		t.Error("expected nil for empty context")
	}
}

func TestFormatHelp(t *testing.T) {
	ch := NewContextualHelp()

	entry := ch.GetHelp("/commit")
	formatted := ch.FormatHelp(entry)

	if formatted == "" {
		t.Fatal("FormatHelp returned empty string")
	}

	// Check structure
	if !strings.Contains(formatted, "/commit") {
		t.Error("formatted help missing topic")
	}
	if !strings.Contains(formatted, "Auto-commit with AI message") {
		t.Error("formatted help missing summary")
	}
	if !strings.Contains(formatted, "─") {
		t.Error("formatted help missing separator")
	}
	if !strings.Contains(formatted, "Examples:") {
		t.Error("formatted help missing examples section")
	}
	if !strings.Contains(formatted, "Related:") {
		t.Error("formatted help missing related section")
	}
	if !strings.Contains(formatted, "/diff") {
		t.Error("formatted help missing related topics")
	}
}

func TestFormatHelp_NilEntry(t *testing.T) {
	ch := NewContextualHelp()

	formatted := ch.FormatHelp(nil)
	if formatted != "" {
		t.Error("expected empty string for nil entry")
	}
}

func TestFormatHelp_HeaderFormat(t *testing.T) {
	ch := NewContextualHelp()

	entry := ch.GetHelp("/commit")
	formatted := ch.FormatHelp(entry)
	lines := strings.Split(formatted, "\n")

	if len(lines) < 3 {
		t.Fatal("expected at least 3 lines in formatted output")
	}

	// First line should be "topic — summary"
	if !strings.Contains(lines[0], " — ") {
		t.Errorf("first line should contain ' — ', got: %s", lines[0])
	}

	// Second line should be separator
	if !strings.HasPrefix(lines[1], "─") {
		t.Errorf("second line should be separator, got: %s", lines[1])
	}
}

func TestFormatSearchResults(t *testing.T) {
	ch := NewContextualHelp()

	results := ch.SearchHelp("commit")
	formatted := ch.FormatSearchResults(results)

	if formatted == "" {
		t.Fatal("FormatSearchResults returned empty string")
	}

	if !strings.Contains(formatted, "result(s)") {
		t.Error("missing result count header")
	}

	// Should contain numbered entries
	if !strings.Contains(formatted, "1.") {
		t.Error("missing numbered entries")
	}
}

func TestFormatSearchResults_Empty(t *testing.T) {
	ch := NewContextualHelp()

	formatted := ch.FormatSearchResults(nil)
	if formatted != "No results found." {
		t.Errorf("expected 'No results found.', got: %s", formatted)
	}

	formatted = ch.FormatSearchResults([]*HelpEntry{})
	if formatted != "No results found." {
		t.Errorf("expected 'No results found.', got: %s", formatted)
	}
}

func TestGetCategories(t *testing.T) {
	ch := NewContextualHelp()

	categories := ch.GetCategories()
	if len(categories) == 0 {
		t.Fatal("expected at least one category")
	}

	// Should be sorted
	for i := 1; i < len(categories); i++ {
		if categories[i] < categories[i-1] {
			t.Errorf("categories not sorted: %s before %s", categories[i-1], categories[i])
		}
	}

	// Should include known categories
	catSet := make(map[string]bool)
	for _, c := range categories {
		catSet[c] = true
	}

	expectedCats := []string{"slash-commands", "common-tasks", "errors", "configuration", "tools"}
	for _, expected := range expectedCats {
		if !catSet[expected] {
			t.Errorf("missing expected category: %s", expected)
		}
	}
}

func TestListByCategory(t *testing.T) {
	ch := NewContextualHelp()

	entries := ch.ListByCategory("slash-commands")
	if len(entries) == 0 {
		t.Fatal("expected entries in slash-commands category")
	}

	for _, entry := range entries {
		if entry.Category != "slash-commands" {
			t.Errorf("entry %s has wrong category: %s", entry.Topic, entry.Category)
		}
	}

	// Should be sorted by topic
	for i := 1; i < len(entries); i++ {
		if entries[i].Topic < entries[i-1].Topic {
			t.Errorf("entries not sorted: %s before %s", entries[i-1].Topic, entries[i].Topic)
		}
	}
}

func TestListByCategory_CaseInsensitive(t *testing.T) {
	ch := NewContextualHelp()

	entries := ch.ListByCategory("Slash-Commands")
	if len(entries) == 0 {
		t.Fatal("case-insensitive category lookup failed")
	}
}

func TestListByCategory_NotFound(t *testing.T) {
	ch := NewContextualHelp()

	entries := ch.ListByCategory("nonexistent-category")
	if len(entries) != 0 {
		t.Error("expected empty result for nonexistent category")
	}
}

func TestHelpEntry_AllFieldsPopulated(t *testing.T) {
	ch := NewContextualHelp()

	for topic, entry := range ch.Entries {
		if entry.Topic == "" {
			t.Errorf("entry %s has empty topic", topic)
		}
		if entry.Summary == "" {
			t.Errorf("entry %s has empty summary", topic)
		}
		if entry.Detail == "" {
			t.Errorf("entry %s has empty detail", topic)
		}
		if len(entry.Examples) == 0 {
			t.Errorf("entry %s has no examples", topic)
		}
		if len(entry.Related) == 0 {
			t.Errorf("entry %s has no related topics", topic)
		}
		if entry.Category == "" {
			t.Errorf("entry %s has empty category", topic)
		}
	}
}

func TestConcurrentAccess(t *testing.T) {
	ch := NewContextualHelp()
	var wg sync.WaitGroup

	// Run concurrent reads
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			switch n % 5 {
			case 0:
				ch.GetHelp("/commit")
			case 1:
				ch.SearchHelp("test")
			case 2:
				ch.SuggestHelp("error occurred")
			case 3:
				ch.GetCategories()
			case 4:
				ch.ListByCategory("slash-commands")
			}
		}(i)
	}

	wg.Wait()
}

func TestSearchHelp_CategoryFilter(t *testing.T) {
	ch := NewContextualHelp()

	// Searching for "errors" should find entries in the errors category
	results := ch.SearchHelp("errors")
	if len(results) == 0 {
		t.Fatal("expected results for 'errors' query")
	}

	foundErrorCategory := false
	for _, entry := range results {
		if entry.Category == "errors" {
			foundErrorCategory = true
			break
		}
	}
	if !foundErrorCategory {
		t.Error("expected to find error category entries")
	}
}

func TestSearchHelp_ToolSearch(t *testing.T) {
	ch := NewContextualHelp()

	results := ch.SearchHelp("daemon")
	if len(results) == 0 {
		t.Fatal("expected results for 'daemon'")
	}

	found := false
	for _, entry := range results {
		if entry.Topic == "tool: daemon" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected to find 'tool: daemon' entry")
	}
}

func TestSuggestHelp_FallbackToSearch(t *testing.T) {
	ch := NewContextualHelp()

	// Context that doesn't match any specific pattern should fallback to search
	suggestions := ch.SuggestHelp("repomap structure")
	if len(suggestions) == 0 {
		t.Fatal("expected fallback search suggestions")
	}
}

func TestFormatHelp_ExamplesIndented(t *testing.T) {
	ch := NewContextualHelp()

	entry := ch.GetHelp("/commit")
	formatted := ch.FormatHelp(entry)

	lines := strings.Split(formatted, "\n")
	inExamples := false
	for _, line := range lines {
		if strings.Contains(line, "Examples:") {
			inExamples = true
			continue
		}
		if inExamples && line != "" && !strings.HasPrefix(line, "\n") && !strings.HasPrefix(line, "Related:") {
			if !strings.HasPrefix(line, "  ") {
				t.Errorf("example line not indented: %q", line)
			}
		}
		if strings.HasPrefix(line, "Related:") {
			break
		}
	}
}

func TestMinimumEntryCount(t *testing.T) {
	ch := NewContextualHelp()

	categories := ch.GetCategories()
	totalEntries := 0
	for _, cat := range categories {
		entries := ch.ListByCategory(cat)
		totalEntries += len(entries)
	}

	if totalEntries < 50 {
		t.Errorf("expected at least 50 entries across all categories, got %d", totalEntries)
	}
}

func TestSearchHelp_NoFalsePositives(t *testing.T) {
	ch := NewContextualHelp()

	// Very specific query that shouldn't match everything
	results := ch.SearchHelp("zzzzxyzzy")
	if len(results) != 0 {
		t.Errorf("expected no results for gibberish query, got %d", len(results))
	}
}
