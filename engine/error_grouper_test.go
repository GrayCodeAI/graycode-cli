package engine

import (
	"strings"
	"sync"
	"testing"
	"time"
)

func TestNormalizeError(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "strips file paths",
			input:    "cannot find /usr/local/lib/foo.go",
			expected: "cannot find <path>",
		},
		{
			name:     "strips line numbers with colon",
			input:    "error at main.go:42: undefined variable",
			expected: "error at main.go<line> undefined variable",
		},
		{
			name:     "strips line keyword",
			input:    "syntax error on line 123",
			expected: "syntax error on <line>",
		},
		{
			name:     "strips hex addresses",
			input:    "nil pointer dereference at 0x7ffeefbff000",
			expected: "nil pointer dereference at <addr>",
		},
		{
			name:     "strips quoted strings",
			input:    `undefined: "myVariable" in scope`,
			expected:  "undefined: <str> in scope",
		},
		{
			name:     "strips large numbers",
			input:    "timeout after 30000ms",
			expected: "timeout after <num>ms",
		},
		{
			name:     "preserves error structure",
			input:    "nil pointer dereference",
			expected: "nil pointer dereference",
		},
		{
			name:     "collapses whitespace",
			input:    "error   in   processing",
			expected: "error in processing",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := NormalizeError(tt.input)
			if result != tt.expected {
				t.Errorf("NormalizeError(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestNormalizeErrorGroupsSimilar(t *testing.T) {
	// Two similar errors with different specifics should normalize the same
	err1 := "cannot find package \"github.com/foo/bar\" in /home/user/go/src"
	err2 := "cannot find package \"github.com/baz/qux\" in /opt/go/src"

	norm1 := NormalizeError(err1)
	norm2 := NormalizeError(err2)

	if norm1 != norm2 {
		t.Errorf("similar errors normalized differently:\n  %q\n  %q", norm1, norm2)
	}
}

func TestNewErrorGrouper(t *testing.T) {
	eg := NewErrorGrouper()
	if eg == nil {
		t.Fatal("NewErrorGrouper returned nil")
	}
	if eg.Groups == nil {
		t.Fatal("Groups map is nil")
	}
	if len(eg.Groups) != 0 {
		t.Errorf("expected empty groups, got %d", len(eg.Groups))
	}
}

func TestAdd(t *testing.T) {
	eg := NewErrorGrouper()

	group := eg.Add("nil pointer dereference at main.go:42", "main.go", 42, "func main()")
	if group == nil {
		t.Fatal("Add returned nil group")
	}
	if group.Count != 1 {
		t.Errorf("expected count 1, got %d", group.Count)
	}
	if group.Status != "active" {
		t.Errorf("expected status active, got %s", group.Status)
	}
	if len(group.Instances) != 1 {
		t.Errorf("expected 1 instance, got %d", len(group.Instances))
	}
	if group.Instances[0].File != "main.go" {
		t.Errorf("expected file main.go, got %s", group.Instances[0].File)
	}
	if group.Instances[0].Line != 42 {
		t.Errorf("expected line 42, got %d", group.Instances[0].Line)
	}
}

func TestAddGroupsSimilarErrors(t *testing.T) {
	eg := NewErrorGrouper()

	g1 := eg.Add("nil pointer dereference at main.go:42", "main.go", 42, "")
	g2 := eg.Add("nil pointer dereference at handler.go:99", "handler.go", 99, "")

	if g1.ID != g2.ID {
		t.Errorf("similar errors placed in different groups: %s vs %s", g1.ID, g2.ID)
	}
	if g2.Count != 2 {
		t.Errorf("expected count 2 after second add, got %d", g2.Count)
	}
	if len(g2.Instances) != 2 {
		t.Errorf("expected 2 instances, got %d", len(g2.Instances))
	}
}

func TestAddRecurringStatus(t *testing.T) {
	eg := NewErrorGrouper()

	group := eg.Add("import cycle detected", "pkg.go", 1, "")
	eg.MarkResolved(group.ID, "extracted interface")

	// Same error reappears
	group2 := eg.Add("import cycle detected", "other.go", 5, "")
	if group2.Status != "recurring" {
		t.Errorf("expected status recurring after resolved error reappears, got %s", group2.Status)
	}
}

func TestFindGroup(t *testing.T) {
	eg := NewErrorGrouper()

	eg.Add("undefined variable foo", "main.go", 10, "")

	// Find with same normalized pattern
	found := eg.FindGroup("undefined variable bar")
	// These won't match because "foo" and "bar" are short words not quoted
	// Let's use a case that will match
	eg.Add(`cannot find "x" in scope`, "a.go", 1, "")
	found = eg.FindGroup(`cannot find "y" in scope`)
	if found == nil {
		t.Fatal("FindGroup returned nil for known pattern")
	}
}

func TestFindGroupNotFound(t *testing.T) {
	eg := NewErrorGrouper()

	found := eg.FindGroup("some unknown error")
	if found != nil {
		t.Error("FindGroup should return nil for unknown error")
	}
}

func TestMarkResolved(t *testing.T) {
	eg := NewErrorGrouper()

	group := eg.Add("import cycle not allowed", "pkg.go", 1, "")
	eg.MarkResolved(group.ID, "extracted interface to break cycle")

	eg.mu.RLock()
	resolved := eg.Groups[group.ID]
	eg.mu.RUnlock()

	if resolved.Status != "resolved" {
		t.Errorf("expected status resolved, got %s", resolved.Status)
	}
	if resolved.Resolution != "extracted interface to break cycle" {
		t.Errorf("unexpected resolution: %s", resolved.Resolution)
	}
}

func TestMarkResolvedNonExistent(t *testing.T) {
	eg := NewErrorGrouper()
	// Should not panic
	eg.MarkResolved("nonexistent_id", "some fix")
}

func TestIsKnown(t *testing.T) {
	eg := NewErrorGrouper()

	eg.Add("connection refused on port 8080", "server.go", 15, "")

	if !eg.IsKnown("connection refused on port 9090") {
		t.Error("IsKnown should return true for similar error with different port")
	}
	if eg.IsKnown("completely different error") {
		t.Error("IsKnown should return false for unrelated error")
	}
}

func TestGetResolution(t *testing.T) {
	eg := NewErrorGrouper()

	group := eg.Add("missing return at end of function", "handler.go", 55, "")
	eg.MarkResolved(group.ID, "added explicit return statement")

	res := eg.GetResolution("missing return at end of function")
	if res != "added explicit return statement" {
		t.Errorf("unexpected resolution: %q", res)
	}
}

func TestGetResolutionUnknown(t *testing.T) {
	eg := NewErrorGrouper()

	res := eg.GetResolution("unknown error")
	if res != "" {
		t.Errorf("expected empty resolution for unknown error, got %q", res)
	}
}

func TestGetResolutionUnresolved(t *testing.T) {
	eg := NewErrorGrouper()

	eg.Add("some active error", "file.go", 1, "")
	res := eg.GetResolution("some active error")
	if res != "" {
		t.Errorf("expected empty resolution for active error, got %q", res)
	}
}

func TestGetActive(t *testing.T) {
	eg := NewErrorGrouper()

	eg.Add("error one", "a.go", 1, "")
	g2 := eg.Add("error two", "b.go", 2, "")
	eg.Add("error three", "c.go", 3, "")

	// Resolve one
	eg.MarkResolved(g2.ID, "fixed")

	active := eg.GetActive()
	if len(active) != 2 {
		t.Errorf("expected 2 active groups, got %d", len(active))
	}

	for _, g := range active {
		if g.Status == "resolved" {
			t.Errorf("resolved group should not be in active list: %s", g.ID)
		}
	}
}

func TestGetActiveIncludesRecurring(t *testing.T) {
	eg := NewErrorGrouper()

	group := eg.Add("recurring problem", "x.go", 1, "")
	eg.MarkResolved(group.ID, "thought it was fixed")
	eg.Add("recurring problem", "y.go", 2, "") // triggers recurring

	active := eg.GetActive()
	if len(active) != 1 {
		t.Fatalf("expected 1 active group, got %d", len(active))
	}
	if active[0].Status != "recurring" {
		t.Errorf("expected recurring status, got %s", active[0].Status)
	}
}

func TestFormatGroups(t *testing.T) {
	eg := NewErrorGrouper()

	eg.Add("nil pointer dereference at token.go:10", "token.go", 10, "")
	eg.Add("nil pointer dereference at handler.go:25", "handler.go", 25, "")

	g2 := eg.Add("import cycle not allowed", "pkg.go", 1, "")
	eg.MarkResolved(g2.ID, "extracted interface to break cycle")

	output := eg.FormatGroups()

	if !strings.Contains(output, "Error Groups") {
		t.Error("output missing header")
	}
	if !strings.Contains(output, "─────────────────────────") {
		t.Error("output missing separator")
	}
	if !strings.Contains(output, "2 instances") {
		t.Error("output should show 2 instances for nil pointer group")
	}
	if !strings.Contains(output, "token.go") {
		t.Error("output should list token.go in files")
	}
	if !strings.Contains(output, "handler.go") {
		t.Error("output should list handler.go in files")
	}
	if !strings.Contains(output, "resolved") {
		t.Error("output should show resolved status")
	}
	if !strings.Contains(output, "extracted interface to break cycle") {
		t.Error("output should show resolution")
	}
}

func TestFormatGroupsEmpty(t *testing.T) {
	eg := NewErrorGrouper()
	output := eg.FormatGroups()

	if !strings.Contains(output, "0 active") {
		t.Error("empty grouper should show 0 active")
	}
}

func TestPrune(t *testing.T) {
	eg := NewErrorGrouper()

	// Add and resolve a group
	group := eg.Add("old error", "old.go", 1, "")
	eg.MarkResolved(group.ID, "fixed long ago")

	// Manually set last seen to the past
	eg.mu.Lock()
	eg.Groups[group.ID].LastSeen = time.Now().Add(-48 * time.Hour)
	eg.mu.Unlock()

	// Add a recent active group
	eg.Add("new error", "new.go", 1, "")

	// Prune anything older than 24h
	eg.Prune(24 * time.Hour)

	eg.mu.RLock()
	defer eg.mu.RUnlock()

	if len(eg.Groups) != 1 {
		t.Errorf("expected 1 group after prune, got %d", len(eg.Groups))
	}
}

func TestPruneKeepsActiveOldGroups(t *testing.T) {
	eg := NewErrorGrouper()

	// Add an active group with old timestamp
	eg.Add("persistent error", "stub.go", 1, "")
	eg.mu.Lock()
	for _, g := range eg.Groups {
		g.LastSeen = time.Now().Add(-48 * time.Hour)
	}
	eg.mu.Unlock()

	eg.Prune(24 * time.Hour)

	eg.mu.RLock()
	defer eg.mu.RUnlock()

	if len(eg.Groups) != 1 {
		t.Errorf("prune should not remove active groups, got %d remaining", len(eg.Groups))
	}
}

func TestConcurrentAccess(t *testing.T) {
	eg := NewErrorGrouper()
	var wg sync.WaitGroup

	// Concurrent writes
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			eg.Add("concurrent error in processing", "file.go", n, "")
		}(i)
	}

	// Concurrent reads
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			eg.IsKnown("concurrent error in processing")
			eg.GetActive()
			eg.FindGroup("concurrent error in processing")
		}()
	}

	wg.Wait()

	group := eg.FindGroup("concurrent error in processing")
	if group == nil {
		t.Fatal("group should exist after concurrent adds")
	}
	if group.Count != 100 {
		t.Errorf("expected 100 instances, got %d", group.Count)
	}
}

func TestFmtGroupDuration(t *testing.T) {
	tests := []struct {
		d        time.Duration
		expected string
	}{
		{30 * time.Second, "30s ago"},
		{2 * time.Minute, "2m ago"},
		{3 * time.Hour, "3h ago"},
		{48 * time.Hour, "2d ago"},
	}

	for _, tt := range tests {
		result := fmtGroupDuration(tt.d)
		if result != tt.expected {
			t.Errorf("fmtGroupDuration(%v) = %q, want %q", tt.d, result, tt.expected)
		}
	}
}

func TestUniqueFiles(t *testing.T) {
	instances := []ErrorInstance{
		{File: "a.go"},
		{File: "b.go"},
		{File: "a.go"},
		{File: ""},
		{File: "c.go"},
	}

	files := uniqueFiles(instances)
	if len(files) != 3 {
		t.Errorf("expected 3 unique files, got %d: %v", len(files), files)
	}
}

func TestGroupIDDeterministic(t *testing.T) {
	id1 := groupID("nil pointer dereference at <path><line>")
	id2 := groupID("nil pointer dereference at <path><line>")

	if id1 != id2 {
		t.Errorf("groupID not deterministic: %s vs %s", id1, id2)
	}

	id3 := groupID("import cycle not allowed")
	if id1 == id3 {
		t.Error("different patterns should not produce same ID")
	}
}
