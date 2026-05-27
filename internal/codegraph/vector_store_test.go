package codegraph

import (
	"context"
	"testing"
)

func TestNewVectorStore(t *testing.T) {
	t.Parallel()
	vs := NewVectorStore(128)
	if vs == nil {
		t.Fatal("NewVectorStore returned nil")
	}
	if vs.dim != 128 {
		t.Errorf("expected dim 128, got %d", vs.dim)
	}
	if vs.collection == nil {
		t.Error("collection should be initialized")
	}
	if vs.collection.name != "codegraph" {
		t.Errorf("expected collection name 'codegraph', got %q", vs.collection.name)
	}
}

func TestVectorStore_Add(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	vs := NewVectorStore(4)

	embedding := []float32{1.0, 0.0, 0.0, 0.0}
	meta := map[string]string{"name": "test"}
	err := vs.Add(ctx, "node1", embedding, meta)
	if err != nil {
		t.Fatalf("Add failed: %v", err)
	}

	count := vs.Count(ctx)
	if count != 1 {
		t.Errorf("expected count 1, got %d", count)
	}
}

func TestVectorStore_AddDimensionMismatch(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	vs := NewVectorStore(4)

	// Wrong dimension
	err := vs.Add(ctx, "node1", []float32{1.0, 0.0}, nil)
	if err == nil {
		t.Error("expected error for dimension mismatch")
	}
}

func TestVectorStore_Search(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	vs := NewVectorStore(4)

	// Add some vectors
	vs.Add(ctx, "a", []float32{1.0, 0.0, 0.0, 0.0}, map[string]string{"name": "a"})
	vs.Add(ctx, "b", []float32{0.0, 1.0, 0.0, 0.0}, map[string]string{"name": "b"})
	vs.Add(ctx, "c", []float32{0.9, 0.1, 0.0, 0.0}, map[string]string{"name": "c"})

	// Search for something close to "a"
	results, err := vs.Search(ctx, []float32{1.0, 0.0, 0.0, 0.0}, 2)
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected at least 1 result")
	}
	if results[0].ID != "a" {
		t.Errorf("expected top result 'a', got %q", results[0].ID)
	}
}

func TestVectorStore_SearchDimensionMismatch(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	vs := NewVectorStore(4)

	_, err := vs.Search(ctx, []float32{1.0, 0.0}, 5)
	if err == nil {
		t.Error("expected error for dimension mismatch")
	}
}

func TestVectorStore_Delete(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	vs := NewVectorStore(4)

	vs.Add(ctx, "node1", []float32{1.0, 0.0, 0.0, 0.0}, nil)
	if vs.Count(ctx) != 1 {
		t.Fatal("expected count 1 after add")
	}

	err := vs.Delete(ctx, "node1")
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}
	if vs.Count(ctx) != 0 {
		t.Errorf("expected count 0 after delete, got %d", vs.Count(ctx))
	}
}

func TestVectorStore_CountEmpty(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	vs := NewVectorStore(4)
	if vs.Count(ctx) != 0 {
		t.Errorf("expected count 0, got %d", vs.Count(ctx))
	}
}

func TestVectorStore_SearchEmpty(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	vs := NewVectorStore(4)

	results, err := vs.Search(ctx, []float32{1.0, 0.0, 0.0, 0.0}, 5)
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d", len(results))
	}
}

func TestVectorStore_SearchKGreaterThanCount(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	vs := NewVectorStore(4)

	vs.Add(ctx, "a", []float32{1.0, 0.0, 0.0, 0.0}, nil)

	results, err := vs.Search(ctx, []float32{1.0, 0.0, 0.0, 0.0}, 100)
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}
	if len(results) != 1 {
		t.Errorf("expected 1 result (capped to store size), got %d", len(results))
	}
}

func TestNewCodeVectorStore(t *testing.T) {
	t.Parallel()
	cvs := NewCodeVectorStore()
	if cvs == nil {
		t.Fatal("NewCodeVectorStore returned nil")
	}
	if cvs.store == nil {
		t.Error("store should be initialized")
	}
	if cvs.nodes == nil {
		t.Error("nodes map should be initialized")
	}
}

func TestCodeVectorStore_IndexNode(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	cvs := NewCodeVectorStore()

	node := Node{
		ID:            "file.go:HandleRequest",
		Kind:          "function",
		Name:          "HandleRequest",
		QualifiedName: "server::HandleRequest",
		FilePath:      "server/handler.go",
		Language:      "go",
		Signature:     "func HandleRequest(w http.ResponseWriter, r *http.Request)",
	}

	err := cvs.IndexNode(ctx, node)
	if err != nil {
		t.Fatalf("IndexNode failed: %v", err)
	}

	stats := cvs.Stats()
	if stats["nodes"] != 1 {
		t.Errorf("expected 1 node, got %v", stats["nodes"])
	}
}

func TestCodeVectorStore_SearchCode(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	cvs := NewCodeVectorStore()

	node1 := Node{ID: "n1", Kind: "function", Name: "HandleAuth", QualifiedName: "auth::HandleAuth", FilePath: "auth.go", Language: "go"}
	node2 := Node{ID: "n2", Kind: "function", Name: "ProcessData", QualifiedName: "data::ProcessData", FilePath: "data.go", Language: "go"}

	cvs.IndexNode(ctx, node1)
	cvs.IndexNode(ctx, node2)

	results, err := cvs.SearchCode(ctx, "HandleAuth", 5)
	if err != nil {
		t.Fatalf("SearchCode failed: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected at least 1 result")
	}
}

func TestCodeVectorStore_Stats(t *testing.T) {
	t.Parallel()
	cvs := NewCodeVectorStore()
	stats := cvs.Stats()

	if stats["dim"] != EmbeddingDimension {
		t.Errorf("expected dim %d, got %v", EmbeddingDimension, stats["dim"])
	}
	if stats["vectors"] != 0 {
		t.Errorf("expected 0 vectors, got %v", stats["vectors"])
	}
	if stats["nodes"] != 0 {
		t.Errorf("expected 0 nodes, got %v", stats["nodes"])
	}
}
