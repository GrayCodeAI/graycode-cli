package engine

import (
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/GrayCodeAI/hawk/internal/tool"
)

// deadlockTimeout bounds the regression tests below. Both Append and Replace
// system-context paths used to be able to leave s.mu locked; we assert here
// that any path that takes s.mu also releases it well within this window.
const deadlockTimeout = 5 * time.Second

// TestAppendSystemContext_Persists confirms AppendSystemContext writes the
// merged system prompt through to the persistence service. Regression guard
// for a regression where the deferred-unlock pattern skipped SetSystem.
func TestAppendSystemContext_Persists(t *testing.T) {
	t.Parallel()
	registry := tool.NewRegistry()
	sess := NewSession("test", "test-model", "base system", registry)

	sess.AppendSystemContext("extra context")

	want := "base system\n\nextra context"
	if got := sess.Persistence().System(); got != want {
		t.Fatalf("Persistence().System() = %q, want %q", got, want)
	}
}

// TestAppendSystemContext_DedupeEmpty ensures empty/whitespace input is a no-op
// (no persistence churn, no spurious newline joins).
func TestAppendSystemContext_DedupeEmpty(t *testing.T) {
	t.Parallel()
	registry := tool.NewRegistry()
	sess := NewSession("test", "test-model", "base", registry)

	before := sess.Persistence().System()
	sess.AppendSystemContext("   \n\t  ")
	after := sess.Persistence().System()

	if before != after {
		t.Fatalf("AppendSystemContext with blank input mutated system prompt: %q -> %q", before, after)
	}
}

// TestReplaceSystemContextSection_AppendBranch_Persists covers the "header
// not found, append" branch that previously had two bugs: it returned while
// holding s.mu (deadlock) and skipped persist.SetSystem. Both must be fixed.
func TestReplaceSystemContextSection_AppendBranch_Persists(t *testing.T) {
	t.Parallel()
	registry := tool.NewRegistry()
	sess := NewSession("test", "test-model", "base system", registry)

	const header = "## Missing Section\n"
	const content = "## Missing Section\nfreshly inserted body"

	sess.ReplaceSystemContextSection(header, content)

	// The append branch must persist the merged system prompt, not just
	// mutate the in-memory field.
	want := "base system\n\n" + content
	if got := sess.Persistence().System(); got != want {
		t.Fatalf("Persistence().System() = %q, want %q", got, want)
	}

	// The next call to AppendSystemContext must NOT deadlock. This is the
	// core regression: previously the append-fallback branch returned
	// without unlocking s.mu, so anything else taking s.mu would hang.
	done := make(chan struct{})
	go func() {
		defer close(done)
		sess.AppendSystemContext("more context")
	}()
	select {
	case <-done:
	case <-time.After(deadlockTimeout):
		t.Fatal("AppendSystemContext deadlocked after ReplaceSystemContextSection append branch")
	}

	merged := sess.Persistence().System()
	if !strings.Contains(merged, "more context") {
		t.Fatalf("expected follow-up AppendSystemContext to succeed; got %q", merged)
	}
}

// TestReplaceSystemContextSection_ReplaceBranch_Persists covers the "header
// found, replace" branch and asserts persistence is kept in sync.
func TestReplaceSystemContextSection_ReplaceBranch_Persists(t *testing.T) {
	t.Parallel()
	registry := tool.NewRegistry()
	const initial = "base system\n\n## Existing\nold body\n\n## Tail\nkeep me"
	sess := NewSession("test", "test-model", initial, registry)

	const header = "## Existing\n"
	const content = "## Existing\nnew body"
	sess.ReplaceSystemContextSection(header, content)

	got := sess.Persistence().System()
	if !strings.Contains(got, "new body") {
		t.Fatalf("replace branch did not update persisted system: %q", got)
	}
	if strings.Contains(got, "old body") {
		t.Fatalf("replace branch left stale content in persisted system: %q", got)
	}
	if !strings.Contains(got, "keep me") {
		t.Fatalf("replace branch clobbered trailing section: %q", got)
	}
}

// TestSystemContext_ConcurrentNoDeadlock runs Append and Replace concurrently
// to ensure neither branch takes s.mu without releasing it, and the new
// unlock-under-deferral pattern is safe under parallel traffic.
func TestSystemContext_ConcurrentNoDeadlock(t *testing.T) {
	t.Parallel()
	registry := tool.NewRegistry()
	sess := NewSession("test", "test-model", "base", registry)

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			sess.AppendSystemContext(strings.Repeat("a", n+1))
		}(i)
	}
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			header := "## H" + strings.Repeat("x", n+1) + "\n"
			content := header + "body " + strings.Repeat("y", n+1)
			sess.ReplaceSystemContextSection(header, content)
		}(i)
	}

	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(deadlockTimeout):
		t.Fatal("concurrent Append/ReplaceSystemContextSection deadlocked")
	}
}
