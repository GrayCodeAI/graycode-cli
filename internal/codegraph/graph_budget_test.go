package codegraph

import (
	"testing"
)

// ---------------------------------------------------------------------------
// BuildMultiViewGraph - AST view details
// ---------------------------------------------------------------------------

func TestBuildMultiViewGraph_ASTNodeFields(t *testing.T) {
	source := []byte(`package main

func hello() {}
`)
	mvg, err := BuildMultiViewGraph("main.go", source)
	if err != nil {
		t.Fatal(err)
	}

	var helloNode *ASTNode
	for _, n := range mvg.AST.Nodes {
		if n.Name == "hello" {
			helloNode = &n
			break
		}
	}
	if helloNode == nil {
		t.Fatal("expected to find 'hello' AST node")
	}
	if helloNode.Type != "func" {
		t.Errorf("type = %q, want 'func'", helloNode.Type)
	}
	if helloNode.File != "main.go" {
		t.Errorf("file = %q, want 'main.go'", helloNode.File)
	}
	if helloNode.StartPos <= 0 {
		t.Errorf("startPos = %d, want > 0", helloNode.StartPos)
	}
}

func TestBuildMultiViewGraph_TypeVarDetection(t *testing.T) {
	source := []byte(`package main

type Config struct{}

var Version = "1.0"
`)
	mvg, err := BuildMultiViewGraph("main.go", source)
	if err != nil {
		t.Fatal(err)
	}

	types := map[string]bool{}
	for _, n := range mvg.AST.Nodes {
		types[n.Type] = true
	}
	if !types["type"] {
		t.Error("expected AST node type 'type'")
	}
	if !types["var"] {
		t.Error("expected AST node type 'var'")
	}
}

// ---------------------------------------------------------------------------
// BuildMultiViewGraph - Call view
// ---------------------------------------------------------------------------

func TestBuildMultiViewGraph_CallEdges(t *testing.T) {
	source := []byte(`package main

func greet() string {
	return "hi"
}

func main() {
	greet()
}
`)
	mvg, err := BuildMultiViewGraph("main.go", source)
	if err != nil {
		t.Fatal(err)
	}

	if mvg.Call == nil {
		t.Fatal("expected non-nil Call view")
	}

	if len(mvg.Call.Nodes) < 2 {
		t.Errorf("expected >= 2 call nodes, got %d", len(mvg.Call.Nodes))
	}

	found := false
	for _, e := range mvg.Call.Edges {
		if containsStr(e.Callee, "greet") && containsStr(e.Caller, "main") {
			found = true
		}
	}
	if !found {
		t.Error("expected call edge from main to greet")
	}
}

func TestBuildMultiViewGraph_CallViewExportFlag(t *testing.T) {
	source := []byte(`package main

func Exported() {}
func unexported() {}

func main() {
	Exported()
	unexported()
}
`)
	mvg, err := BuildMultiViewGraph("main.go", source)
	if err != nil {
		t.Fatal(err)
	}

	for _, n := range mvg.Call.Nodes {
		if n.Name == "Exported" && !n.IsExport {
			t.Error("Exported should have IsExport=true")
		}
		if n.Name == "unexported" && n.IsExport {
			t.Error("unexported should have IsExport=false")
		}
	}
}

// ---------------------------------------------------------------------------
// BuildMultiViewGraph - CFG view
// ---------------------------------------------------------------------------

func TestBuildMultiViewGraph_CFGBranchAndLoop(t *testing.T) {
	source := []byte(`package main

func check(x int) string {
	if x > 0 {
		return "positive"
	}
	for i := 0; i < x; i++ {
		_ = i
	}
	return "other"
}
`)
	mvg, err := BuildMultiViewGraph("main.go", source)
	if err != nil {
		t.Fatal(err)
	}

	if mvg.CFG == nil {
		t.Fatal("expected non-nil CFG view")
	}

	nodeTypes := map[string]bool{}
	for _, n := range mvg.CFG.Nodes {
		nodeTypes[n.Type] = true
	}
	if !nodeTypes["entry"] {
		t.Error("expected 'entry' CFG node")
	}
}

// ---------------------------------------------------------------------------
// BuildMultiViewGraph - DFG view
// ---------------------------------------------------------------------------

func TestBuildMultiViewGraph_DFGVarsDetail(t *testing.T) {
	source := []byte(`package main

func process() {
	x := 10
	y := x + 1
	_ = y
}
`)
	mvg, err := BuildMultiViewGraph("main.go", source)
	if err != nil {
		t.Fatal(err)
	}

	if mvg.DFG == nil {
		t.Fatal("expected non-nil DFG view")
	}
	if len(mvg.DFG.Nodes) == 0 {
		t.Error("expected DFG nodes for variable definitions and uses")
	}
}

// ---------------------------------------------------------------------------
// BuildMultiViewGraph - error cases
// ---------------------------------------------------------------------------

func TestBuildMultiViewGraph_InvalidGoSource(t *testing.T) {
	source := []byte(`not valid go code {{{`)
	_, err := BuildMultiViewGraph("bad.go", source)
	if err == nil {
		t.Error("expected error for invalid Go source")
	}
}

func TestBuildMultiViewGraph_MultipleFunctions(t *testing.T) {
	source := []byte(`package main

func a() {}
func b() {}
func c() {}
`)
	mvg, err := BuildMultiViewGraph("multi.go", source)
	if err != nil {
		t.Fatal(err)
	}

	funcCount := 0
	for _, n := range mvg.AST.Nodes {
		if n.Type == "func" {
			funcCount++
		}
	}
	if funcCount != 3 {
		t.Errorf("expected 3 func nodes, got %d", funcCount)
	}
}

// ---------------------------------------------------------------------------
// HybridSearch (CodeVectorStore)
// ---------------------------------------------------------------------------

func TestHybridSearch_MergesVectorAndKeyword(t *testing.T) {
	cvs := NewCodeVectorStore()

	nodes := []Node{
		{ID: "n1", Kind: "function", Name: "HandleAuth", QualifiedName: "auth.HandleAuth", FilePath: "auth.go", Language: "go"},
		{ID: "n2", Kind: "function", Name: "ProcessData", QualifiedName: "data.ProcessData", FilePath: "data.go", Language: "go"},
		{ID: "n3", Kind: "function", Name: "ValidateToken", QualifiedName: "auth.ValidateToken", FilePath: "auth.go", Language: "go"},
	}

	for _, n := range nodes {
		cvs.IndexNode(t.Context(), n)
	}

	keywordResults := []Node{nodes[0]}
	results, err := cvs.HybridSearch(t.Context(), "auth handler", 5, keywordResults)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) == 0 {
		t.Error("expected at least 1 result from hybrid search")
	}
}

func TestHybridSearch_EmptyKeywordResults(t *testing.T) {
	cvs := NewCodeVectorStore()
	cvs.IndexNode(t.Context(), Node{
		ID: "n1", Kind: "function", Name: "Test", QualifiedName: "test.Test", FilePath: "test.go", Language: "go",
	})

	results, err := cvs.HybridSearch(t.Context(), "test", 5, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) == 0 {
		t.Error("expected results even with empty keyword input")
	}
}

func TestHybridSearch_EmptyVectorStore(t *testing.T) {
	cvs := NewCodeVectorStore()

	results, err := cvs.HybridSearch(t.Context(), "anything", 5, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results from empty store, got %d", len(results))
	}
}

func TestHybridSearch_LimitRespected(t *testing.T) {
	cvs := NewCodeVectorStore()

	for i := 0; i < 20; i++ {
		cvs.IndexNode(t.Context(), Node{
			ID:            "n" + string(rune('a'+i%26)),
			Kind:          "function",
			Name:          "Func" + string(rune('A'+i%26)),
			QualifiedName: "pkg.Func",
			FilePath:      "file.go",
			Language:      "go",
		})
	}

	results, err := cvs.HybridSearch(t.Context(), "Func", 3, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) > 3 {
		t.Errorf("expected at most 3 results, got %d", len(results))
	}
}

// ---------------------------------------------------------------------------
// CosineSimilarity - additional edge cases
// ---------------------------------------------------------------------------

func TestCosineSimilarity_IdenticalVectors(t *testing.T) {
	v := []float32{1.0, 2.0, 3.0}
	sim := CosineSimilarity(v, v)
	if sim < 0.99 {
		t.Errorf("expected similarity ~1.0 for identical vectors, got %f", sim)
	}
}

func TestCosineSimilarity_OrthogonalVectors(t *testing.T) {
	a := []float32{1.0, 0.0, 0.0}
	b := []float32{0.0, 1.0, 0.0}
	sim := CosineSimilarity(a, b)
	if sim > 0.01 {
		t.Errorf("expected similarity ~0.0 for orthogonal vectors, got %f", sim)
	}
}

func TestCosineSimilarity_OppositeVectors(t *testing.T) {
	a := []float32{1.0, 0.0}
	b := []float32{-1.0, 0.0}
	sim := CosineSimilarity(a, b)
	if sim > -0.99 {
		t.Errorf("expected similarity ~-1.0 for opposite vectors, got %f", sim)
	}
}

// ---------------------------------------------------------------------------
// Node struct
// ---------------------------------------------------------------------------

func TestNode_Fields(t *testing.T) {
	n := Node{
		ID:            "file.go:Func",
		Kind:          "function",
		Name:          "Func",
		QualifiedName: "pkg.Func",
		FilePath:      "file.go",
		Language:      "go",
		StartLine:     10,
		EndLine:       20,
		Signature:     "func Func() error",
		Docstring:     "Func does something.",
		Visibility:    "public",
		IsExported:    true,
	}
	if n.ID != "file.go:Func" {
		t.Errorf("ID = %q", n.ID)
	}
	if !n.IsExported {
		t.Error("expected IsExported=true")
	}
	if n.StartLine != 10 || n.EndLine != 20 {
		t.Errorf("lines = %d-%d", n.StartLine, n.EndLine)
	}
}

// ---------------------------------------------------------------------------
// Helper
// ---------------------------------------------------------------------------

func containsStr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
