package intelligence

import (
	"strings"
	"sync"
	"testing"
)

func TestNewCapabilityRegistry(t *testing.T) {
	r := NewCapabilityRegistry()

	if r == nil {
		t.Fatal("NewCapabilityRegistry returned nil")
	}

	if len(r.Capabilities) < 30 {
		t.Errorf("expected at least 30 built-in capabilities, got %d", len(r.Capabilities))
	}

	if len(r.Categories) == 0 {
		t.Error("expected non-empty categories map")
	}
}

func TestGetCapability(t *testing.T) {
	r := NewCapabilityRegistry()

	cap := r.GetCapability("code_write")
	if cap == nil {
		t.Fatal("expected code_write capability")
	}
	if cap.Name != "Write Code" {
		t.Errorf("expected Name 'Write Code', got %q", cap.Name)
	}
	if cap.Category != "Code" {
		t.Errorf("expected Category 'Code', got %q", cap.Category)
	}
	if cap.Complexity != "moderate" {
		t.Errorf("expected Complexity 'moderate', got %q", cap.Complexity)
	}

	none := r.GetCapability("nonexistent")
	if none != nil {
		t.Error("expected nil for nonexistent capability")
	}
}

func TestListByCategory(t *testing.T) {
	r := NewCapabilityRegistry()

	codeCaps := r.ListByCategory("Code")
	if len(codeCaps) == 0 {
		t.Fatal("expected Code category to have capabilities")
	}

	for _, c := range codeCaps {
		if c.Category != "Code" {
			t.Errorf("capability %s has category %q, expected 'Code'", c.ID, c.Category)
		}
	}

	// Non-existent category returns nil
	none := r.ListByCategory("NonExistent")
	if none != nil {
		t.Error("expected nil for non-existent category")
	}
}

func TestCanDo(t *testing.T) {
	r := NewCapabilityRegistry()

	tests := []struct {
		task     string
		expectID string
	}{
		{"write a new file", "code_write"},
		{"fix this bug", "bug_fix"},
		{"run tests", "run_tests"},
		{"commit changes", "git_commit"},
		{"search for function", "search_code"},
	}

	for _, tc := range tests {
		results := r.CanDo(tc.task)
		if len(results) == 0 {
			t.Errorf("CanDo(%q) returned no results", tc.task)
			continue
		}

		found := false
		for _, cap := range results {
			if cap.ID == tc.expectID {
				found = true
				break
			}
		}
		if !found {
			ids := make([]string, len(results))
			for i, c := range results {
				ids[i] = c.ID
			}
			t.Errorf("CanDo(%q) expected %s in results, got %v", tc.task, tc.expectID, ids)
		}
	}
}

func TestCanDoDisabledExcluded(t *testing.T) {
	r := NewCapabilityRegistry()
	r.Disable("code_write")

	results := r.CanDo("write new code files")
	for _, cap := range results {
		if cap.ID == "code_write" {
			t.Error("disabled capability code_write should not appear in CanDo results")
		}
	}
}

func TestFormatHelp(t *testing.T) {
	r := NewCapabilityRegistry()
	help := r.FormatHelp()

	if !strings.Contains(help, "graycode Capabilities:") {
		t.Error("FormatHelp missing header")
	}
	if !strings.Contains(help, "═") {
		t.Error("FormatHelp missing separator")
	}
	if !strings.Contains(help, "Code:") {
		t.Error("FormatHelp missing Code category")
	}
	if !strings.Contains(help, "code_write") {
		t.Error("FormatHelp missing code_write")
	}
	if !strings.Contains(help, "—") {
		t.Error("FormatHelp missing dash separator")
	}
}

func TestFormatCapability(t *testing.T) {
	r := NewCapabilityRegistry()
	cap := r.GetCapability("bug_fix")
	if cap == nil {
		t.Fatal("expected bug_fix capability")
	}

	formatted := r.FormatCapability(cap)

	if !strings.Contains(formatted, "Fix Bugs") {
		t.Error("formatted output missing name")
	}
	if !strings.Contains(formatted, "bug_fix") {
		t.Error("formatted output missing ID")
	}
	if !strings.Contains(formatted, "Code") {
		t.Error("formatted output missing category")
	}
	if !strings.Contains(formatted, "complex") {
		t.Error("formatted output missing complexity")
	}
	if !strings.Contains(formatted, "Examples:") {
		t.Error("formatted output missing examples section")
	}

	// Nil capability
	empty := r.FormatCapability(nil)
	if empty != "" {
		t.Error("FormatCapability(nil) should return empty string")
	}
}

func TestEnableDisable(t *testing.T) {
	r := NewCapabilityRegistry()

	cap := r.GetCapability("code_write")
	if !cap.Enabled {
		t.Fatal("expected code_write to start enabled")
	}

	r.Disable("code_write")
	cap = r.GetCapability("code_write")
	if cap.Enabled {
		t.Error("expected code_write to be disabled")
	}

	r.Enable("code_write")
	cap = r.GetCapability("code_write")
	if !cap.Enabled {
		t.Error("expected code_write to be re-enabled")
	}

	// No panic on nonexistent
	r.Disable("nonexistent")
	r.Enable("nonexistent")
}

func TestSearch(t *testing.T) {
	r := NewCapabilityRegistry()

	results := r.Search("git")
	if len(results) == 0 {
		t.Fatal("expected results for 'git' search")
	}
	for _, cap := range results {
		lower := strings.ToLower(cap.ID + cap.Name + cap.Description + cap.Category)
		found := strings.Contains(lower, "git")
		if !found {
			// Check examples
			for _, ex := range cap.Examples {
				if strings.Contains(strings.ToLower(ex), "git") {
					found = true
					break
				}
			}
		}
		if !found {
			t.Errorf("search result %s does not contain 'git'", cap.ID)
		}
	}

	// Search returns sorted results
	for i := 1; i < len(results); i++ {
		if results[i-1].ID > results[i].ID {
			t.Errorf("results not sorted: %s > %s", results[i-1].ID, results[i].ID)
		}
	}

	// Empty search term should return nothing or everything depending on implementation
	empty := r.Search("zzzznonexistent")
	if len(empty) != 0 {
		t.Errorf("expected no results for nonsense query, got %d", len(empty))
	}
}

func TestGetCategories(t *testing.T) {
	r := NewCapabilityRegistry()

	cats := r.GetCategories()
	if len(cats) == 0 {
		t.Fatal("expected non-empty categories")
	}

	// Verify sorted
	for i := 1; i < len(cats); i++ {
		if cats[i-1] > cats[i] {
			t.Errorf("categories not sorted: %s > %s", cats[i-1], cats[i])
		}
	}

	// Expected categories present
	expected := []string{"Code", "Testing", "Git", "Files"}
	for _, exp := range expected {
		found := false
		for _, cat := range cats {
			if cat == exp {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected category %q not found in %v", exp, cats)
		}
	}
}

func TestCapabilitiesConcurrentAccess(t *testing.T) {
	r := NewCapabilityRegistry()

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(4)
		go func() {
			defer wg.Done()
			r.GetCapability("code_write")
		}()
		go func() {
			defer wg.Done()
			r.ListByCategory("Code")
		}()
		go func() {
			defer wg.Done()
			r.Search("test")
		}()
		go func() {
			defer wg.Done()
			r.CanDo("write tests")
		}()
	}
	wg.Wait()
}

func TestCapabilityFields(t *testing.T) {
	r := NewCapabilityRegistry()

	for id, cap := range r.Capabilities {
		if cap.ID == "" {
			t.Errorf("capability at key %q has empty ID", id)
		}
		if cap.ID != id {
			t.Errorf("capability ID %q does not match map key %q", cap.ID, id)
		}
		if cap.Name == "" {
			t.Errorf("capability %s has empty Name", id)
		}
		if cap.Description == "" {
			t.Errorf("capability %s has empty Description", id)
		}
		if cap.Category == "" {
			t.Errorf("capability %s has empty Category", id)
		}
		if cap.Complexity == "" {
			t.Errorf("capability %s has empty Complexity", id)
		}

		validComplexity := map[string]bool{
			"trivial": true, "simple": true, "moderate": true, "complex": true,
		}
		if !validComplexity[cap.Complexity] {
			t.Errorf("capability %s has invalid Complexity %q", id, cap.Complexity)
		}

		if len(cap.Tools) == 0 {
			t.Errorf("capability %s has no tools", id)
		}
		if len(cap.Examples) == 0 {
			t.Errorf("capability %s has no examples", id)
		}
	}
}

func TestCategoryConsistency(t *testing.T) {
	r := NewCapabilityRegistry()

	// Every capability's category should have the capability ID in the Categories map
	for id, cap := range r.Capabilities {
		ids, ok := r.Categories[cap.Category]
		if !ok {
			t.Errorf("capability %s category %q not in Categories map", id, cap.Category)
			continue
		}
		found := false
		for _, cid := range ids {
			if cid == id {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("capability %s not listed in Categories[%q]", id, cap.Category)
		}
	}

	// Every ID in Categories should exist in Capabilities
	for cat, ids := range r.Categories {
		for _, id := range ids {
			if _, ok := r.Capabilities[id]; !ok {
				t.Errorf("Categories[%q] references non-existent capability %s", cat, id)
			}
		}
	}
}
