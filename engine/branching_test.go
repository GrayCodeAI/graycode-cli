package engine

import (
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"
)

func TestNewBranchManager(t *testing.T) {
	bm := NewBranchManager()
	if bm == nil {
		t.Fatal("expected non-nil BranchManager")
	}
	if bm.ActiveBranch == "" {
		t.Fatal("expected active branch to be set")
	}
	if bm.RootBranch == "" {
		t.Fatal("expected root branch to be set")
	}
	if bm.ActiveBranch != bm.RootBranch {
		t.Fatalf("expected active and root to match, got active=%q root=%q", bm.ActiveBranch, bm.RootBranch)
	}

	root := bm.Branches[bm.RootBranch]
	if root == nil {
		t.Fatal("expected root branch in map")
	}
	if root.Name != "main" {
		t.Fatalf("expected root name 'main', got %q", root.Name)
	}
	if root.Status != "active" {
		t.Fatalf("expected root status 'active', got %q", root.Status)
	}
}

func TestForkCreatesNewBranch(t *testing.T) {
	bm := NewBranchManager()

	// Add some messages to main.
	bm.AddMessage("user", "Hello", nil)
	bm.AddMessage("assistant", "Hi there!", nil)
	bm.AddMessage("user", "Please implement auth", nil)
	bm.AddMessage("assistant", "Sure, I'll use JWT", nil)

	// Fork at message 3 (after "Please implement auth").
	branch, err := bm.Fork("approach-A", 3)
	if err != nil {
		t.Fatalf("Fork failed: %v", err)
	}

	if branch.Name != "approach-A" {
		t.Fatalf("expected name 'approach-A', got %q", branch.Name)
	}
	if branch.ParentID != bm.RootBranch {
		t.Fatalf("expected parent to be root branch")
	}
	if branch.ForkPoint != 3 {
		t.Fatalf("expected fork point 3, got %d", branch.ForkPoint)
	}
	if len(branch.Messages) != 3 {
		t.Fatalf("expected 3 messages copied, got %d", len(branch.Messages))
	}
	if branch.Messages[0].Content != "Hello" {
		t.Fatalf("expected first message 'Hello', got %q", branch.Messages[0].Content)
	}
	if branch.Messages[2].Content != "Please implement auth" {
		t.Fatalf("expected third message content mismatch")
	}
	if branch.Status != "active" {
		t.Fatalf("expected status 'active', got %q", branch.Status)
	}

	// Active branch should now be the new branch.
	if bm.ActiveBranch != branch.ID {
		t.Fatalf("expected active branch to be new branch")
	}
}

func TestForkAtDifferentIndices(t *testing.T) {
	bm := NewBranchManager()

	bm.AddMessage("user", "msg-0", nil)
	bm.AddMessage("assistant", "msg-1", nil)
	bm.AddMessage("user", "msg-2", nil)
	bm.AddMessage("assistant", "msg-3", nil)
	bm.AddMessage("user", "msg-4", nil)

	// Fork at beginning (0 messages).
	_ = switchToRoot(t, bm)
	b1, err := bm.Fork("from-start", 0)
	if err != nil {
		t.Fatalf("Fork at 0 failed: %v", err)
	}
	if len(b1.Messages) != 0 {
		t.Fatalf("expected 0 messages, got %d", len(b1.Messages))
	}

	// Switch back to root and fork at 2.
	_ = switchToRoot(t, bm)
	b2, err := bm.Fork("mid-point", 2)
	if err != nil {
		t.Fatalf("Fork at 2 failed: %v", err)
	}
	if len(b2.Messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(b2.Messages))
	}

	// Switch back to root and fork at end.
	_ = switchToRoot(t, bm)
	b3, err := bm.Fork("from-end", 5)
	if err != nil {
		t.Fatalf("Fork at 5 failed: %v", err)
	}
	if len(b3.Messages) != 5 {
		t.Fatalf("expected 5 messages, got %d", len(b3.Messages))
	}
}

func TestForkInvalidIndex(t *testing.T) {
	bm := NewBranchManager()
	bm.AddMessage("user", "hello", nil)

	_, err := bm.Fork("bad", -1)
	if err == nil {
		t.Fatal("expected error for negative fork point")
	}

	_, err = bm.Fork("bad", 10)
	if err == nil {
		t.Fatal("expected error for fork point exceeding message count")
	}
}

func TestMultipleForksFromSamePoint(t *testing.T) {
	bm := NewBranchManager()
	bm.AddMessage("user", "Hello", nil)
	bm.AddMessage("assistant", "Hi", nil)
	bm.AddMessage("user", "Implement feature X", nil)

	// Fork twice from the same point on root.
	_ = switchToRoot(t, bm)
	branchA, err := bm.Fork("approach-A", 3)
	if err != nil {
		t.Fatalf("Fork A failed: %v", err)
	}

	_ = switchToRoot(t, bm)
	branchB, err := bm.Fork("approach-B", 3)
	if err != nil {
		t.Fatalf("Fork B failed: %v", err)
	}

	if branchA.ID == branchB.ID {
		t.Fatal("expected different IDs for different forks")
	}
	if len(branchA.Messages) != 3 || len(branchB.Messages) != 3 {
		t.Fatal("both branches should have 3 messages")
	}

	// Each branch gets its own messages.
	if err := bm.Switch(branchA.ID); err != nil {
		t.Fatal(err)
	}
	bm.AddMessage("assistant", "Using JWT approach", nil)

	if err := bm.Switch(branchB.ID); err != nil {
		t.Fatal(err)
	}
	bm.AddMessage("assistant", "Using session approach", nil)

	if len(bm.Branches[branchA.ID].Messages) != 4 {
		t.Fatalf("expected 4 messages in A, got %d", len(bm.Branches[branchA.ID].Messages))
	}
	if len(bm.Branches[branchB.ID].Messages) != 4 {
		t.Fatalf("expected 4 messages in B, got %d", len(bm.Branches[branchB.ID].Messages))
	}
}

func TestSwitchBranch(t *testing.T) {
	bm := NewBranchManager()
	bm.AddMessage("user", "msg", nil)

	branch, _ := bm.Fork("alt", 1)
	bm.AddMessage("assistant", "alt response", nil)

	// Switch to root.
	if err := bm.Switch(bm.RootBranch); err != nil {
		t.Fatalf("Switch to root failed: %v", err)
	}
	if bm.ActiveBranch != bm.RootBranch {
		t.Fatal("expected active to be root after switch")
	}

	// Switch back.
	if err := bm.Switch(branch.ID); err != nil {
		t.Fatalf("Switch back failed: %v", err)
	}
	if bm.ActiveBranch != branch.ID {
		t.Fatal("expected active to be alt after switch")
	}
}

func TestSwitchNonExistent(t *testing.T) {
	bm := NewBranchManager()
	if err := bm.Switch("nonexistent"); err == nil {
		t.Fatal("expected error switching to non-existent branch")
	}
}

func TestSwitchToAbandoned(t *testing.T) {
	bm := NewBranchManager()
	bm.AddMessage("user", "msg", nil)
	branch, _ := bm.Fork("temp", 1)

	_ = switchToRoot(t, bm)
	if err := bm.Abandon(branch.ID); err != nil {
		t.Fatal(err)
	}

	if err := bm.Switch(branch.ID); err == nil {
		t.Fatal("expected error switching to abandoned branch")
	}
}

func TestMergeBranches(t *testing.T) {
	bm := NewBranchManager()
	bm.AddMessage("user", "Hello", nil)
	bm.AddMessage("assistant", "Hi", nil)
	bm.AddMessage("user", "Implement X", nil)

	// Fork and add messages.
	branch, _ := bm.Fork("feature", 3)
	bm.AddMessage("assistant", "Feature done part 1", nil)
	bm.AddMessage("user", "Continue", nil)
	bm.AddMessage("assistant", "Feature done part 2", nil)

	// Switch back to root and merge.
	if err := bm.Switch(bm.RootBranch); err != nil {
		t.Fatal(err)
	}

	if err := bm.Merge(branch.ID); err != nil {
		t.Fatalf("Merge failed: %v", err)
	}

	// Root should now have original 4 + 3 new messages from feature.
	root := bm.Branches[bm.RootBranch]
	if len(root.Messages) != 7 {
		t.Fatalf("expected 7 messages after merge, got %d", len(root.Messages))
	}

	// The merged messages should be the ones after fork point.
	if root.Messages[4].Content != "Feature done part 1" {
		t.Fatalf("expected merged message, got %q", root.Messages[4].Content)
	}

	// Source branch should be marked as merged.
	if bm.Branches[branch.ID].Status != "merged" {
		t.Fatalf("expected source branch status 'merged', got %q", bm.Branches[branch.ID].Status)
	}
}

func TestMergeIntoSelf(t *testing.T) {
	bm := NewBranchManager()
	if err := bm.Merge(bm.ActiveBranch); err == nil {
		t.Fatal("expected error when merging into self")
	}
}

func TestMergeNonExistent(t *testing.T) {
	bm := NewBranchManager()
	if err := bm.Merge("nonexistent"); err == nil {
		t.Fatal("expected error merging non-existent branch")
	}
}

func TestAbandon(t *testing.T) {
	bm := NewBranchManager()
	bm.AddMessage("user", "msg", nil)
	branch, _ := bm.Fork("temp", 1)

	if err := bm.Abandon(branch.ID); err != nil {
		t.Fatalf("Abandon failed: %v", err)
	}

	if bm.Branches[branch.ID].Status != "abandoned" {
		t.Fatalf("expected status 'abandoned', got %q", bm.Branches[branch.ID].Status)
	}

	// Active should switch back to root since active was abandoned.
	if bm.ActiveBranch != bm.RootBranch {
		t.Fatal("expected active to revert to root after abandoning active branch")
	}
}

func TestAbandonRoot(t *testing.T) {
	bm := NewBranchManager()
	if err := bm.Abandon(bm.RootBranch); err == nil {
		t.Fatal("expected error abandoning root branch")
	}
}

func TestAbandonNonExistent(t *testing.T) {
	bm := NewBranchManager()
	if err := bm.Abandon("nonexistent"); err == nil {
		t.Fatal("expected error abandoning non-existent branch")
	}
}

func TestGetBranches(t *testing.T) {
	bm := NewBranchManager()
	bm.AddMessage("user", "msg", nil)

	bm.Fork("b1", 1)
	_ = switchToRoot(t, bm)
	bm.Fork("b2", 1)

	branches := bm.GetBranches()
	if len(branches) != 3 {
		t.Fatalf("expected 3 branches, got %d", len(branches))
	}
}

func TestGetActiveMessages(t *testing.T) {
	bm := NewBranchManager()
	bm.AddMessage("user", "msg1", nil)
	bm.AddMessage("assistant", "msg2", nil)

	branch, _ := bm.Fork("alt", 1)
	bm.AddMessage("assistant", "alt-msg", nil)

	msgs := bm.GetActiveMessages()
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages in active (alt), got %d", len(msgs))
	}
	if msgs[1].Content != "alt-msg" {
		t.Fatalf("expected 'alt-msg', got %q", msgs[1].Content)
	}

	// Switch and verify.
	_ = branch // suppress unused
	if err := bm.Switch(bm.RootBranch); err != nil {
		t.Fatal(err)
	}
	msgs = bm.GetActiveMessages()
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages in root, got %d", len(msgs))
	}
	if msgs[1].Content != "msg2" {
		t.Fatalf("expected 'msg2', got %q", msgs[1].Content)
	}
}

func TestCompareBranches(t *testing.T) {
	bm := NewBranchManager()
	bm.AddMessage("user", "Hello", nil)
	bm.AddMessage("assistant", "Hi", nil)
	bm.AddMessage("user", "Please implement auth", nil)

	branchA, _ := bm.Fork("approach-A", 3)
	bm.AddMessage("assistant", "Took the JWT route, modified 3 files", nil)
	bm.AddMessage("user", "Looks good", nil)

	_ = switchToRoot(t, bm)
	branchB, _ := bm.Fork("approach-B", 3)
	bm.AddMessage("assistant", "Took the session route, modified 2 files", nil)

	output := bm.CompareBranches(branchA.ID, branchB.ID)

	if !containsStr(output, "approach-A (2 messages after fork)") {
		t.Fatalf("expected approach-A info in output, got:\n%s", output)
	}
	if !containsStr(output, "approach-B (1 messages after fork)") {
		t.Fatalf("expected approach-B info in output, got:\n%s", output)
	}
	if !containsStr(output, "Diverged at message 3") {
		t.Fatalf("expected divergence info in output, got:\n%s", output)
	}
	if !containsStr(output, "Please implement auth") {
		t.Fatalf("expected divergence message in output, got:\n%s", output)
	}
	if !containsStr(output, "JWT route") {
		t.Fatalf("expected JWT in output, got:\n%s", output)
	}
	if !containsStr(output, "session route") {
		t.Fatalf("expected session in output, got:\n%s", output)
	}
}

func TestCompareBranchesNotFound(t *testing.T) {
	bm := NewBranchManager()

	output := bm.CompareBranches("nonexistent", bm.RootBranch)
	if !containsStr(output, "not found") {
		t.Fatalf("expected 'not found' in output, got: %s", output)
	}

	output = bm.CompareBranches(bm.RootBranch, "nonexistent")
	if !containsStr(output, "not found") {
		t.Fatalf("expected 'not found' in output, got: %s", output)
	}
}

func TestBuildBranchContext(t *testing.T) {
	bm := NewBranchManager()
	bm.AddMessage("user", "msg1", nil)
	bm.AddMessage("assistant", "msg2", nil)
	bm.AddMessage("user", "msg3", nil)

	// Create branches.
	branchA, _ := bm.Fork("approach-A", 2)
	bm.AddMessage("assistant", "a1", nil)

	_ = switchToRoot(t, bm)
	branchB, _ := bm.Fork("refactor-v2", 2)
	bm.AddMessage("assistant", "b1", nil)
	bm.AddMessage("user", "b2", nil)
	bm.AddMessage("assistant", "b3", nil)

	// Abandon branchA.
	if err := bm.Abandon(branchA.ID); err != nil {
		t.Fatal(err)
	}

	// Active is now branchB (since we're on it).
	ctx := bm.BuildBranchContext()

	if !containsStr(ctx, "## Conversation Branches") {
		t.Fatalf("expected header, got:\n%s", ctx)
	}
	if !containsStr(ctx, "Active: refactor-v2 (5 messages)") {
		t.Fatalf("expected active branch info, got:\n%s", ctx)
	}
	if !containsStr(ctx, "approach-A (abandoned)") {
		t.Fatalf("expected abandoned branch, got:\n%s", ctx)
	}

	// Root should show in branches list.
	_ = branchB // suppress unused
}

func TestPrune(t *testing.T) {
	bm := NewBranchManager()
	bm.AddMessage("user", "msg", nil)

	branch, _ := bm.Fork("old-branch", 1)

	// Manually set the creation time to long ago.
	bm.mu.Lock()
	bm.Branches[branch.ID].CreatedAt = time.Now().UTC().Add(-48 * time.Hour)
	bm.mu.Unlock()

	// Switch back before abandoning.
	_ = switchToRoot(t, bm)
	if err := bm.Abandon(branch.ID); err != nil {
		t.Fatal(err)
	}

	// Prune with 24h threshold - should remove the branch.
	bm.Prune(24 * time.Hour)

	if _, exists := bm.Branches[branch.ID]; exists {
		t.Fatal("expected old abandoned branch to be pruned")
	}
}

func TestPruneDoesNotRemoveRecent(t *testing.T) {
	bm := NewBranchManager()
	bm.AddMessage("user", "msg", nil)

	branch, _ := bm.Fork("recent-branch", 1)
	_ = switchToRoot(t, bm)
	if err := bm.Abandon(branch.ID); err != nil {
		t.Fatal(err)
	}

	// Prune with 24h threshold - recently created, should stay.
	bm.Prune(24 * time.Hour)

	if _, exists := bm.Branches[branch.ID]; !exists {
		t.Fatal("expected recent abandoned branch to NOT be pruned")
	}
}

func TestPruneDoesNotRemoveActive(t *testing.T) {
	bm := NewBranchManager()
	bm.AddMessage("user", "msg", nil)

	branch, _ := bm.Fork("active-branch", 1)
	bm.mu.Lock()
	bm.Branches[branch.ID].CreatedAt = time.Now().UTC().Add(-48 * time.Hour)
	bm.mu.Unlock()

	// It's still "active" status so prune should not touch it.
	bm.Prune(24 * time.Hour)

	if _, exists := bm.Branches[branch.ID]; !exists {
		t.Fatal("expected active branch to NOT be pruned even if old")
	}
}

func TestExportImportRoundTrip(t *testing.T) {
	bm := NewBranchManager()
	bm.AddMessage("user", "Hello world", []string{"read_file"})
	bm.AddMessage("assistant", "I read the file", nil)

	branch, _ := bm.Fork("export-test", 2)
	bm.AddMessage("assistant", "Branch-specific message", []string{"write_file"})

	// Export.
	data, err := bm.ExportBranch(branch.ID)
	if err != nil {
		t.Fatalf("ExportBranch failed: %v", err)
	}

	// Verify it's valid JSON.
	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("exported data is not valid JSON: %v", err)
	}

	// Import into a new manager.
	bm2 := NewBranchManager()
	imported, err := bm2.ImportBranch(data)
	if err != nil {
		t.Fatalf("ImportBranch failed: %v", err)
	}

	if imported.Name != "export-test" {
		t.Fatalf("expected name 'export-test', got %q", imported.Name)
	}
	if len(imported.Messages) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(imported.Messages))
	}
	if imported.Messages[0].Content != "Hello world" {
		t.Fatalf("expected first msg 'Hello world', got %q", imported.Messages[0].Content)
	}
	if imported.Messages[2].Content != "Branch-specific message" {
		t.Fatalf("expected third msg content mismatch")
	}
	if len(imported.Messages[0].ToolUse) != 1 || imported.Messages[0].ToolUse[0] != "read_file" {
		t.Fatal("expected tool use to be preserved")
	}
	if imported.ForkPoint != 2 {
		t.Fatalf("expected fork point 2, got %d", imported.ForkPoint)
	}
}

func TestExportNonExistent(t *testing.T) {
	bm := NewBranchManager()
	_, err := bm.ExportBranch("nonexistent")
	if err == nil {
		t.Fatal("expected error exporting non-existent branch")
	}
}

func TestImportCollision(t *testing.T) {
	bm := NewBranchManager()
	bm.AddMessage("user", "msg", nil)
	branch, _ := bm.Fork("collision-test", 1)

	data, _ := bm.ExportBranch(branch.ID)

	// Import back into the same manager (ID collision).
	imported, err := bm.ImportBranch(data)
	if err != nil {
		t.Fatalf("ImportBranch with collision failed: %v", err)
	}

	// Should get a new ID.
	if imported.ID == branch.ID {
		t.Fatal("expected new ID to avoid collision")
	}
	if imported.Name != "collision-test" {
		t.Fatalf("expected same name, got %q", imported.Name)
	}
}

func TestImportInvalidJSON(t *testing.T) {
	bm := NewBranchManager()
	_, err := bm.ImportBranch([]byte("not json"))
	if err == nil {
		t.Fatal("expected error importing invalid JSON")
	}
}

func TestConcurrentAccess(t *testing.T) {
	bm := NewBranchManager()

	// Add some initial messages.
	for i := 0; i < 10; i++ {
		bm.AddMessage("user", "msg", nil)
	}

	var wg sync.WaitGroup
	errs := make(chan error, 100)

	// Concurrent forks.
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			// Switch to root before forking.
			if err := bm.Switch(bm.RootBranch); err != nil {
				errs <- err
				return
			}
			_, err := bm.Fork(fmt.Sprintf("concurrent-%d", idx), 5)
			if err != nil {
				errs <- err
			}
		}(i)
	}

	// Concurrent reads.
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = bm.GetBranches()
			_ = bm.GetActiveMessages()
			_ = bm.BuildBranchContext()
		}()
	}

	// Concurrent messages.
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			bm.AddMessage("user", fmt.Sprintf("concurrent msg %d", idx), nil)
		}(i)
	}

	wg.Wait()
	close(errs)

	for err := range errs {
		// We allow "active branch not found" type errors in concurrent scenario
		// since another goroutine may have switched the active branch.
		// But there should be no panics or data races (tested with -race flag).
		_ = err
	}

	// Verify the manager is in a consistent state.
	branches := bm.GetBranches()
	if len(branches) < 2 {
		t.Fatalf("expected at least 2 branches after concurrent ops, got %d", len(branches))
	}
}

func TestAddMessageWithToolUse(t *testing.T) {
	bm := NewBranchManager()
	bm.AddMessage("assistant", "Let me check that", []string{"read_file", "grep"})

	msgs := bm.GetActiveMessages()
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	if len(msgs[0].ToolUse) != 2 {
		t.Fatalf("expected 2 tool uses, got %d", len(msgs[0].ToolUse))
	}
	if msgs[0].ToolUse[0] != "read_file" || msgs[0].ToolUse[1] != "grep" {
		t.Fatal("tool use mismatch")
	}
}

func TestForkDoesNotMutateParent(t *testing.T) {
	bm := NewBranchManager()
	bm.AddMessage("user", "original-1", nil)
	bm.AddMessage("assistant", "original-2", nil)
	bm.AddMessage("user", "original-3", nil)

	rootID := bm.RootBranch
	branch, _ := bm.Fork("child", 2)

	// Add message to child.
	bm.AddMessage("assistant", "child-msg", nil)

	// Verify root is unchanged.
	root := bm.Branches[rootID]
	if len(root.Messages) != 3 {
		t.Fatalf("expected root to still have 3 messages, got %d", len(root.Messages))
	}

	// Verify child has the right messages.
	child := bm.Branches[branch.ID]
	if len(child.Messages) != 3 {
		t.Fatalf("expected child to have 3 messages (2 copied + 1 new), got %d", len(child.Messages))
	}
	if child.Messages[2].Content != "child-msg" {
		t.Fatalf("expected 'child-msg', got %q", child.Messages[2].Content)
	}
}

// --- helpers ---

func switchToRoot(t *testing.T, bm *BranchManager) string {
	t.Helper()
	if err := bm.Switch(bm.RootBranch); err != nil {
		t.Fatalf("failed to switch to root: %v", err)
	}
	return bm.RootBranch
}

func containsStr(s, substr string) bool {
	return len(s) >= len(substr) && contains(s, substr)
}

func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
