package convodag

import (
	"os"
	"path/filepath"
	"testing"
)

func tempDB(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	return filepath.Join(dir, "test.db")
}

func mustNew(t *testing.T) *DAG {
	t.Helper()
	dag, err := New(tempDB(t), "sess-1")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { dag.Close() })
	return dag
}

func TestNewCreatesDatabase(t *testing.T) {
	dbPath := tempDB(t)
	dag, err := New(dbPath, "sess-1")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer dag.Close()

	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		t.Fatal("expected database file to be created")
	}
}

func TestAppendRoot(t *testing.T) {
	dag := mustNew(t)

	node, err := dag.Append("", "user", "hello")
	if err != nil {
		t.Fatalf("Append root: %v", err)
	}
	if node.ID == "" {
		t.Fatal("expected non-empty ID")
	}
	if node.ParentID != "" {
		t.Fatalf("root node should have empty ParentID, got %q", node.ParentID)
	}
	if node.Role != "user" {
		t.Fatalf("expected role 'user', got %q", node.Role)
	}
	if node.Content != "hello" {
		t.Fatalf("expected content 'hello', got %q", node.Content)
	}
	if len(node.ID) != 8 {
		t.Fatalf("expected 8-char ID, got %q (len %d)", node.ID, len(node.ID))
	}
}

func TestAppendChild(t *testing.T) {
	dag := mustNew(t)

	root, err := dag.Append("", "user", "hello")
	if err != nil {
		t.Fatalf("Append root: %v", err)
	}

	child, err := dag.Append(root.ID, "assistant", "world")
	if err != nil {
		t.Fatalf("Append child: %v", err)
	}
	if child.ParentID != root.ID {
		t.Fatalf("expected parent %q, got %q", root.ID, child.ParentID)
	}
}

func TestAppendInvalidParent(t *testing.T) {
	dag := mustNew(t)

	_, err := dag.Append("nonexistent", "user", "hello")
	if err == nil {
		t.Fatal("expected error for invalid parent")
	}
}

func TestHeadAdvancesOnAppend(t *testing.T) {
	dag := mustNew(t)

	n1, _ := dag.Append("", "user", "first")
	n2, _ := dag.Append(n1.ID, "assistant", "second")
	n3, _ := dag.Append(n2.ID, "user", "third")

	head, err := dag.Head()
	if err != nil {
		t.Fatalf("Head: %v", err)
	}
	if head.ID != n3.ID {
		t.Fatalf("expected head %q, got %q", n3.ID, head.ID)
	}
}

func TestHeadEmptySession(t *testing.T) {
	dag := mustNew(t)

	_, err := dag.Head()
	if err == nil {
		t.Fatal("expected error for empty session head")
	}
}

func TestHistory(t *testing.T) {
	dag := mustNew(t)

	n1, _ := dag.Append("", "system", "you are helpful")
	n2, _ := dag.Append(n1.ID, "user", "hi")
	n3, _ := dag.Append(n2.ID, "assistant", "hello!")
	n4, _ := dag.Append(n3.ID, "user", "how are you?")

	history, err := dag.History(n4.ID)
	if err != nil {
		t.Fatalf("History: %v", err)
	}
	if len(history) != 4 {
		t.Fatalf("expected 4 nodes in history, got %d", len(history))
	}

	// Root should be first.
	if history[0].ID != n1.ID {
		t.Fatalf("expected root %q first, got %q", n1.ID, history[0].ID)
	}
	// Leaf should be last.
	if history[3].ID != n4.ID {
		t.Fatalf("expected leaf %q last, got %q", n4.ID, history[3].ID)
	}

	// Verify ordering.
	expected := []string{n1.ID, n2.ID, n3.ID, n4.ID}
	for i, node := range history {
		if node.ID != expected[i] {
			t.Fatalf("history[%d]: expected %q, got %q", i, expected[i], node.ID)
		}
	}
}

func TestHistoryFromMiddle(t *testing.T) {
	dag := mustNew(t)

	n1, _ := dag.Append("", "user", "one")
	n2, _ := dag.Append(n1.ID, "assistant", "two")
	dag.Append(n2.ID, "user", "three")

	// History from middle node should not include third.
	history, err := dag.History(n2.ID)
	if err != nil {
		t.Fatalf("History: %v", err)
	}
	if len(history) != 2 {
		t.Fatalf("expected 2 nodes, got %d", len(history))
	}
}

func TestBranches(t *testing.T) {
	dag := mustNew(t)

	root, _ := dag.Append("", "user", "question")
	c1, _ := dag.Append(root.ID, "assistant", "answer A")
	c2, _ := dag.Append(root.ID, "assistant", "answer B")

	branches, err := dag.Branches(root.ID)
	if err != nil {
		t.Fatalf("Branches: %v", err)
	}
	if len(branches) != 2 {
		t.Fatalf("expected 2 branches, got %d", len(branches))
	}

	ids := map[string]bool{branches[0].ID: true, branches[1].ID: true}
	if !ids[c1.ID] || !ids[c2.ID] {
		t.Fatalf("expected children %q and %q, got %v", c1.ID, c2.ID, ids)
	}
}

func TestBranchesEmpty(t *testing.T) {
	dag := mustNew(t)

	leaf, _ := dag.Append("", "user", "leaf")
	branches, err := dag.Branches(leaf.ID)
	if err != nil {
		t.Fatalf("Branches: %v", err)
	}
	if len(branches) != 0 {
		t.Fatalf("expected 0 branches, got %d", len(branches))
	}
}

func TestFork(t *testing.T) {
	dag := mustNew(t)

	n1, _ := dag.Append("", "user", "hello")
	n2, _ := dag.Append(n1.ID, "assistant", "hi there")
	n3, _ := dag.Append(n2.ID, "user", "tell me a joke")

	// Fork from n2 — creates an alternative to n3.
	fork, err := dag.Fork(n2.ID)
	if err != nil {
		t.Fatalf("Fork: %v", err)
	}
	if fork.ID == n2.ID {
		t.Fatal("fork should have a new ID")
	}
	if fork.ParentID != n2.ParentID {
		t.Fatalf("fork parent should be %q, got %q", n2.ParentID, fork.ParentID)
	}
	if fork.Content != n2.Content {
		t.Fatalf("fork should copy content, got %q", fork.Content)
	}
	if fork.Metadata["forked_from"] != n2.ID {
		t.Fatal("fork should record forked_from in metadata")
	}

	// Head should now point to the fork.
	head, _ := dag.Head()
	if head.ID != fork.ID {
		t.Fatalf("head should be fork %q, got %q", fork.ID, head.ID)
	}

	// Can append to the fork.
	alt, err := dag.Append(fork.ID, "user", "tell me a story instead")
	if err != nil {
		t.Fatalf("Append after fork: %v", err)
	}

	// The alternative history should not include n3.
	altHistory, _ := dag.History(alt.ID)
	for _, node := range altHistory {
		if node.ID == n3.ID {
			t.Fatal("alternative branch should not contain original n3")
		}
	}
}

func TestSetHead(t *testing.T) {
	dag := mustNew(t)

	n1, _ := dag.Append("", "user", "first")
	n2, _ := dag.Append(n1.ID, "assistant", "second")

	// Head is at n2.
	head, _ := dag.Head()
	if head.ID != n2.ID {
		t.Fatalf("expected head %q, got %q", n2.ID, head.ID)
	}

	// Move head back to n1.
	if err := dag.SetHead(n1.ID); err != nil {
		t.Fatalf("SetHead: %v", err)
	}
	head, _ = dag.Head()
	if head.ID != n1.ID {
		t.Fatalf("expected head %q after SetHead, got %q", n1.ID, head.ID)
	}
}

func TestSetHeadInvalidNode(t *testing.T) {
	dag := mustNew(t)

	err := dag.SetHead("nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent node")
	}
}

func TestPrune(t *testing.T) {
	dag := mustNew(t)

	n1, _ := dag.Append("", "user", "root")
	n2, _ := dag.Append(n1.ID, "assistant", "branch A")
	n3, _ := dag.Append(n2.ID, "user", "deep A")
	n4, _ := dag.Append(n1.ID, "assistant", "branch B")

	// Prune branch A (n2 and its descendant n3).
	if err := dag.Prune(n2.ID); err != nil {
		t.Fatalf("Prune: %v", err)
	}

	// n2 should be gone.
	_, err := dag.History(n2.ID)
	if err == nil {
		t.Fatal("expected error after pruning n2")
	}
	// n3 should be gone.
	_, err = dag.History(n3.ID)
	if err == nil {
		t.Fatal("expected error after pruning n3")
	}
	// n4 should still exist.
	history, err := dag.History(n4.ID)
	if err != nil {
		t.Fatalf("History(n4): %v", err)
	}
	if len(history) != 2 {
		t.Fatalf("expected 2 nodes in n4 history, got %d", len(history))
	}
}

func TestPruneMovesHead(t *testing.T) {
	dag := mustNew(t)

	n1, _ := dag.Append("", "user", "root")
	n2, _ := dag.Append(n1.ID, "assistant", "child")
	n3, _ := dag.Append(n2.ID, "user", "grandchild")

	// Head is at n3.
	head, _ := dag.Head()
	if head.ID != n3.ID {
		t.Fatalf("expected head %q, got %q", n3.ID, head.ID)
	}

	// Prune n2 (and therefore n3). Head should move to n1.
	if err := dag.Prune(n2.ID); err != nil {
		t.Fatalf("Prune: %v", err)
	}

	head, err := dag.Head()
	if err != nil {
		t.Fatalf("Head after prune: %v", err)
	}
	if head.ID != n1.ID {
		t.Fatalf("expected head to move to %q, got %q", n1.ID, head.ID)
	}
}

func TestPruneRoot(t *testing.T) {
	dag := mustNew(t)

	n1, _ := dag.Append("", "user", "root")
	dag.Append(n1.ID, "assistant", "child")

	// Pruning root should remove everything.
	if err := dag.Prune(n1.ID); err != nil {
		t.Fatalf("Prune root: %v", err)
	}

	// Head should be gone.
	_, err := dag.Head()
	if err == nil {
		t.Fatal("expected no head after pruning root")
	}
}

func TestMultipleSessions(t *testing.T) {
	dbPath := tempDB(t)

	dag1, err := New(dbPath, "sess-1")
	if err != nil {
		t.Fatalf("New sess-1: %v", err)
	}
	defer dag1.Close()

	dag2, err := New(dbPath, "sess-2")
	if err != nil {
		t.Fatalf("New sess-2: %v", err)
	}
	defer dag2.Close()

	n1, _ := dag1.Append("", "user", "session 1 message")
	n2, _ := dag2.Append("", "user", "session 2 message")

	head1, _ := dag1.Head()
	head2, _ := dag2.Head()

	if head1.ID != n1.ID {
		t.Fatalf("sess-1 head: expected %q, got %q", n1.ID, head1.ID)
	}
	if head2.ID != n2.ID {
		t.Fatalf("sess-2 head: expected %q, got %q", n2.ID, head2.ID)
	}

	// Branches in one session should not leak to the other.
	branches1, _ := dag1.Branches("")
	branches2, _ := dag2.Branches("")
	// Root-level branches should only have one each.
	if len(branches1) != 1 {
		t.Fatalf("sess-1 root branches: expected 1, got %d", len(branches1))
	}
	if len(branches2) != 1 {
		t.Fatalf("sess-2 root branches: expected 1, got %d", len(branches2))
	}
}

func TestMetadataRoundTrip(t *testing.T) {
	dag := mustNew(t)

	node, _ := dag.Append("", "user", "test")
	retrieved, err := dag.getNode(node.ID)
	if err != nil {
		t.Fatalf("getNode: %v", err)
	}
	if retrieved.Metadata == nil {
		t.Fatal("expected non-nil metadata")
	}
}

func TestLongConversation(t *testing.T) {
	dag := mustNew(t)

	var lastID string
	for i := 0; i < 100; i++ {
		role := "user"
		if i%2 == 1 {
			role = "assistant"
		}
		node, err := dag.Append(lastID, role, "message")
		if err != nil {
			t.Fatalf("Append %d: %v", i, err)
		}
		lastID = node.ID
	}

	history, err := dag.History(lastID)
	if err != nil {
		t.Fatalf("History: %v", err)
	}
	if len(history) != 100 {
		t.Fatalf("expected 100 nodes, got %d", len(history))
	}

	// Root is first.
	if history[0].ParentID != "" {
		t.Fatal("first node should have empty parent")
	}
}
