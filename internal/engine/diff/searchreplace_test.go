package diff

import (
	"strings"
	"testing"
)

// TestParseSearchReplace_Single parses a single well-formed block.
func TestParseSearchReplace_Single(t *testing.T) {
	input := `<<<<<<< SEARCH
old line
=======
new line
>>>>>>> REPLACE`

	blocks := ParseSearchReplace(input)
	if len(blocks) != 1 {
		t.Fatalf("expected 1 block, got %d", len(blocks))
	}
	if blocks[0].Search != "old line" {
		t.Errorf("Search = %q, want %q", blocks[0].Search, "old line")
	}
	if blocks[0].Replace != "new line" {
		t.Errorf("Replace = %q, want %q", blocks[0].Replace, "new line")
	}
}

// TestParseSearchReplace_Multiple parses two blocks separated by prose.
func TestParseSearchReplace_Multiple(t *testing.T) {
	input := `Here is the first change:
<<<<<<< SEARCH
func Foo() {}
=======
func Foo() error { return nil }
>>>>>>> REPLACE

And here is the second:
<<<<<<< SEARCH
const X = 1
=======
const X = 42
>>>>>>> REPLACE
`

	blocks := ParseSearchReplace(input)
	if len(blocks) != 2 {
		t.Fatalf("expected 2 blocks, got %d", len(blocks))
	}
	if blocks[0].Search != "func Foo() {}" {
		t.Errorf("block[0].Search = %q", blocks[0].Search)
	}
	if !strings.Contains(blocks[0].Replace, "error") {
		t.Errorf("block[0].Replace should mention 'error', got %q", blocks[0].Replace)
	}
	if blocks[1].Search != "const X = 1" {
		t.Errorf("block[1].Search = %q", blocks[1].Search)
	}
	if blocks[1].Replace != "const X = 42" {
		t.Errorf("block[1].Replace = %q", blocks[1].Replace)
	}
}

// TestParseSearchReplace_EmptyReplace handles a deletion block (empty replace).
func TestParseSearchReplace_EmptyReplace(t *testing.T) {
	input := `<<<<<<< SEARCH
dead code
=======
>>>>>>> REPLACE`

	blocks := ParseSearchReplace(input)
	if len(blocks) != 1 {
		t.Fatalf("expected 1 block, got %d", len(blocks))
	}
	if blocks[0].Search != "dead code" {
		t.Errorf("Search = %q", blocks[0].Search)
	}
	if blocks[0].Replace != "" {
		t.Errorf("Replace should be empty for deletion block, got %q", blocks[0].Replace)
	}
}

// TestParseSearchReplace_ProseAround verifies that surrounding prose is ignored.
func TestParseSearchReplace_ProseAround(t *testing.T) {
	input := `I would suggest the following change to fix the bug:

` + "```go" + `
// ... existing code unchanged ...
` + "```" + `

<<<<<<< SEARCH
x := 1
=======
x := 2
>>>>>>> REPLACE

Let me know if you have questions.`

	blocks := ParseSearchReplace(input)
	if len(blocks) != 1 {
		t.Fatalf("expected 1 block, got %d", len(blocks))
	}
	if blocks[0].Search != "x := 1" {
		t.Errorf("Search = %q", blocks[0].Search)
	}
}

// TestParseSearchReplace_MalformedMissingDivider verifies incomplete blocks are skipped.
func TestParseSearchReplace_MalformedMissingDivider(t *testing.T) {
	input := `<<<<<<< SEARCH
orphaned
no divider or closing marker`

	blocks := ParseSearchReplace(input)
	if len(blocks) != 0 {
		t.Fatalf("malformed block should be skipped, got %d blocks", len(blocks))
	}
}

// ── ApplySearchReplace tests ──────────────────────────────────────────────────

// TestApplySearchReplace_Single applies a single block successfully.
func TestApplySearchReplace_Single(t *testing.T) {
	content := "Hello, world!\nGoodbye, world!\n"
	blocks := []SearchReplaceBlock{
		{Search: "Hello, world!", Replace: "Hello, Go!"},
	}
	got, err := ApplySearchReplace(content, blocks)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "Hello, Go!\nGoodbye, world!\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// TestApplySearchReplace_Multiple applies two blocks sequentially.
func TestApplySearchReplace_Multiple(t *testing.T) {
	content := "alpha\nbeta\ngamma\n"
	blocks := []SearchReplaceBlock{
		{Search: "alpha", Replace: "ALPHA"},
		{Search: "beta", Replace: "BETA"},
	}
	got, err := ApplySearchReplace(content, blocks)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "ALPHA\nBETA\ngamma\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// TestApplySearchReplace_SearchNotFound returns an error when SEARCH is absent.
func TestApplySearchReplace_SearchNotFound(t *testing.T) {
	content := "the quick brown fox\n"
	blocks := []SearchReplaceBlock{
		{Search: "lazy dog", Replace: "fast cat"},
	}
	_, err := ApplySearchReplace(content, blocks)
	if err == nil {
		t.Fatal("expected error for missing SEARCH, got nil")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error should mention 'not found', got: %v", err)
	}
}

// TestApplySearchReplace_AmbiguousSearch returns an error when SEARCH matches multiple times.
func TestApplySearchReplace_AmbiguousSearch(t *testing.T) {
	content := "foo\nfoo\nbar\n"
	blocks := []SearchReplaceBlock{
		{Search: "foo", Replace: "baz"},
	}
	_, err := ApplySearchReplace(content, blocks)
	if err == nil {
		t.Fatal("expected error for ambiguous SEARCH, got nil")
	}
	if !strings.Contains(err.Error(), "ambiguous") {
		t.Errorf("error should mention 'ambiguous', got: %v", err)
	}
}

// TestApplySearchReplace_EmptyReplace deletes the matched text.
func TestApplySearchReplace_EmptyReplace(t *testing.T) {
	content := "keep this\ndelete this\nkeep that\n"
	blocks := []SearchReplaceBlock{
		{Search: "delete this\n", Replace: ""},
	}
	got, err := ApplySearchReplace(content, blocks)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "keep this\nkeep that\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// TestApplySearchReplace_NoBlocks returns content unchanged.
func TestApplySearchReplace_NoBlocks(t *testing.T) {
	content := "unchanged\n"
	got, err := ApplySearchReplace(content, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != content {
		t.Errorf("got %q, want %q", got, content)
	}
}

// TestRoundTrip parses then applies blocks from realistic LLM output.
func TestRoundTrip(t *testing.T) {
	llmOutput := `Here are the changes needed:

<<<<<<< SEARCH
func add(a, b int) int {
	return a - b
}
=======
func add(a, b int) int {
	return a + b
}
>>>>>>> REPLACE
`

	originalFile := `package calc

func add(a, b int) int {
	return a - b
}
`

	blocks := ParseSearchReplace(llmOutput)
	if len(blocks) != 1 {
		t.Fatalf("expected 1 block, got %d", len(blocks))
	}

	result, err := ApplySearchReplace(originalFile, blocks)
	if err != nil {
		t.Fatalf("ApplySearchReplace error: %v", err)
	}

	if !strings.Contains(result, "return a + b") {
		t.Errorf("fix not applied; result:\n%s", result)
	}
	if strings.Contains(result, "return a - b") {
		t.Errorf("old code not removed; result:\n%s", result)
	}
}
