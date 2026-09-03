package permissions

import (
	"path/filepath"
	"testing"

	"github.com/GrayCodeAI/graycode-cli/internal/permissions/stableid"
)

func TestStableRuleStoreResetAdvancesGeneration(t *testing.T) {
	store := NewStableRuleStore(filepath.Join(t.TempDir(), "rules.json"))
	if _, ok := store.Remember(stableid.KindCommand, "git status", "git status", stableid.Allow); !ok {
		t.Fatal("Remember failed")
	}
	if !store.Reset() {
		t.Fatal("Reset should report that it removed rules")
	}
	if got := store.List(); len(got) != 0 {
		t.Fatalf("expected empty store, got %d rules", len(got))
	}
	if _, ok := store.Remember(stableid.KindCommand, "git diff", "git diff", stableid.Allow); !ok {
		t.Fatal("Remember after reset failed")
	}
	if got := store.List()[0].ID; got <= 2 {
		t.Fatalf("expected reset to prevent ID reuse, got %d", got)
	}
}
