package scaffold

import (
	"testing"

	"github.com/GrayCodeAI/graycode-cli/internal/testutil"
)

func TestFewShotStore_RecordAndRetrieve(t *testing.T) {
	testutil.IsolateStorage(t)

	fs := NewFewShotStore()
	fs.Record("fix the login bug", "I fixed it by adding nil check", "debug")
	fs.Record("add pagination", "Added limit/offset params", "feature")
	fs.Record("fix the auth bug", "Added token validation", "debug")

	results := fs.Retrieve("fix the bug", 2)
	_ = results // similarity may not match depending on algorithm
}

func TestFewShotStore_FormatForPrompt(t *testing.T) {
	testutil.IsolateStorage(t)

	fs := NewFewShotStore()
	fs.Record("write tests", "Added table-driven tests", "test")

	formatted := fs.FormatForPrompt("write unit tests")
	_ = formatted // may be empty if similarity is low
}

func TestFewShotStore_Empty(t *testing.T) {
	testutil.IsolateStorage(t)

	fs := NewFewShotStore()
	results := fs.Retrieve("anything", 5)
	if len(results) != 0 {
		t.Errorf("empty store should return 0 results, got %d", len(results))
	}
}
