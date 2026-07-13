package session

import (
	"path/filepath"
	"testing"
)

func TestConversationGraphPersistsBranches(t *testing.T) {
	path := filepath.Join(t.TempDir(), "graph.json")
	graph, err := OpenConversationGraph(path, "session-1")
	if err != nil {
		t.Fatal(err)
	}
	root, err := graph.Append("", "user", "hello")
	if err != nil {
		t.Fatal(err)
	}
	answer, err := graph.Append(root.ID, "assistant", "answer")
	if err != nil {
		t.Fatal(err)
	}
	fork, err := graph.Fork(answer.ID)
	if err != nil {
		t.Fatal(err)
	}
	if fork.Metadata["forked_from"] != answer.ID {
		t.Fatalf("fork metadata = %+v", fork.Metadata)
	}
	if closeErr := graph.Close(); closeErr != nil {
		t.Fatal(closeErr)
	}

	reopened, err := OpenConversationGraph(path, "session-1")
	if err != nil {
		t.Fatal(err)
	}
	head, err := reopened.Head()
	if err != nil {
		t.Fatal(err)
	}
	if head.ID != fork.ID {
		t.Fatalf("head = %q, want %q", head.ID, fork.ID)
	}
	history, err := reopened.History(fork.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != 2 || history[0].ID != root.ID || history[1].ID != fork.ID {
		t.Fatalf("history = %+v", history)
	}
	children, err := reopened.Branches(root.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(children) != 2 {
		t.Fatalf("root children = %d, want 2", len(children))
	}
}
