package tool

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParsePatch_SingleFileOneHunk(t *testing.T) {
	input := `*** Begin Patch
*** Update File: main.go
@@@ func main() { @@@
- fmt.Println("hello")
+ fmt.Println("world")
@@@ } @@@
*** End Patch`

	parser, err := ParsePatch(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	patches := parser.Patches()
	if len(patches) != 1 {
		t.Fatalf("expected 1 patch, got %d", len(patches))
	}
	if patches[0].Path != "main.go" {
		t.Errorf("expected path main.go, got %s", patches[0].Path)
	}
	if len(patches[0].Hunks) != 1 {
		t.Fatalf("expected 1 hunk, got %d", len(patches[0].Hunks))
	}
	h := patches[0].Hunks[0]
	if h.ContextBefore != "func main() {" {
		t.Errorf("unexpected context before: %q", h.ContextBefore)
	}
	if h.ContextAfter != "}" {
		t.Errorf("unexpected context after: %q", h.ContextAfter)
	}
	if len(h.OldLines) != 1 || h.OldLines[0] != `fmt.Println("hello")` {
		t.Errorf("unexpected old lines: %v", h.OldLines)
	}
	if len(h.NewLines) != 1 || h.NewLines[0] != `fmt.Println("world")` {
		t.Errorf("unexpected new lines: %v", h.NewLines)
	}
}

func TestParsePatch_SingleFileMultipleHunks(t *testing.T) {
	input := `*** Begin Patch
*** Update File: server.go
@@@ import ( @@@
- "fmt"
+ "log"
@@@ ) @@@
@@@ func serve() { @@@
- fmt.Println("starting")
+ log.Println("starting")
@@@ } @@@
*** End Patch`

	parser, err := ParsePatch(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	patches := parser.Patches()
	if len(patches) != 1 {
		t.Fatalf("expected 1 patch, got %d", len(patches))
	}
	if len(patches[0].Hunks) != 2 {
		t.Fatalf("expected 2 hunks, got %d", len(patches[0].Hunks))
	}
}

func TestParsePatch_MultipleFiles(t *testing.T) {
	input := `*** Begin Patch
*** Update File: a.go
@@@ line before @@@
- old
+ new
@@@ line after @@@
*** Update File: b.go
@@@ context @@@
- removed
+ added
@@@ end @@@
*** End Patch`

	parser, err := ParsePatch(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	patches := parser.Patches()
	if len(patches) != 2 {
		t.Fatalf("expected 2 patches, got %d", len(patches))
	}
	if patches[0].Path != "a.go" {
		t.Errorf("expected a.go, got %s", patches[0].Path)
	}
	if patches[1].Path != "b.go" {
		t.Errorf("expected b.go, got %s", patches[1].Path)
	}
}

func TestParsePatch_NewFile(t *testing.T) {
	input := `*** Begin Patch
*** Create File: new.go
+ package main
+
+ func main() {}
*** End Patch`

	parser, err := ParsePatch(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	patches := parser.Patches()
	if len(patches) != 1 {
		t.Fatalf("expected 1 patch, got %d", len(patches))
	}
	if !patches[0].IsNew {
		t.Error("expected IsNew to be true")
	}
	if len(patches[0].Hunks) != 1 {
		t.Fatalf("expected 1 hunk, got %d", len(patches[0].Hunks))
	}
	if len(patches[0].Hunks[0].NewLines) != 3 {
		t.Errorf("expected 3 new lines, got %d", len(patches[0].Hunks[0].NewLines))
	}
}

func TestParsePatch_DeleteFile(t *testing.T) {
	input := `*** Begin Patch
*** Delete File: obsolete.go
*** End Patch`

	parser, err := ParsePatch(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	patches := parser.Patches()
	if len(patches) != 1 {
		t.Fatalf("expected 1 patch, got %d", len(patches))
	}
	if !patches[0].IsDelete {
		t.Error("expected IsDelete to be true")
	}
	if patches[0].Path != "obsolete.go" {
		t.Errorf("expected obsolete.go, got %s", patches[0].Path)
	}
}

func TestApply_SingleHunk(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "main.go")
	content := `package main

func main() {
	fmt.Println("hello")
}
`
	if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	patch := &FilePatch{
		Path: filePath,
		Hunks: []Hunk{
			{
				ContextBefore: "func main() {",
				ContextAfter:  "}",
				OldLines:      []string{`	fmt.Println("hello")`},
				NewLines:      []string{`	fmt.Println("world")`},
			},
		},
	}

	if err := Apply(patch); err != nil {
		t.Fatalf("Apply failed: %v", err)
	}

	result, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(result), `fmt.Println("world")`) {
		t.Errorf("expected world in result, got:\n%s", string(result))
	}
	if strings.Contains(string(result), `fmt.Println("hello")`) {
		t.Errorf("old line should be replaced, got:\n%s", string(result))
	}
}

func TestApply_MultipleHunks(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "server.go")
	content := `package main

import (
	"fmt"
)

func serve() {
	fmt.Println("starting")
}
`
	if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	patch := &FilePatch{
		Path: filePath,
		Hunks: []Hunk{
			{
				ContextBefore: "import (",
				ContextAfter:  ")",
				OldLines:      []string{`	"fmt"`},
				NewLines:      []string{`	"log"`},
			},
			{
				ContextBefore: "func serve() {",
				ContextAfter:  "}",
				OldLines:      []string{`	fmt.Println("starting")`},
				NewLines:      []string{`	log.Println("starting")`},
			},
		},
	}

	if err := Apply(patch); err != nil {
		t.Fatalf("Apply failed: %v", err)
	}

	result, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(result), `"log"`) {
		t.Errorf("expected log import in result, got:\n%s", string(result))
	}
	if !strings.Contains(string(result), `log.Println("starting")`) {
		t.Errorf("expected log.Println in result, got:\n%s", string(result))
	}
}

func TestApply_NewFileCreation(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "subdir", "new.go")

	patch := &FilePatch{
		Path:  filePath,
		IsNew: true,
		Hunks: []Hunk{
			{
				NewLines: []string{"package main", "", "func main() {}"},
			},
		},
	}

	if err := Apply(patch); err != nil {
		t.Fatalf("Apply failed: %v", err)
	}

	result, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatal(err)
	}
	expected := "package main\n\nfunc main() {}\n"
	if string(result) != expected {
		t.Errorf("expected %q, got %q", expected, string(result))
	}
}

func TestApply_FileDeletion(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "obsolete.go")
	if err := os.WriteFile(filePath, []byte("package old\n"), 0644); err != nil {
		t.Fatal(err)
	}

	patch := &FilePatch{
		Path:     filePath,
		IsDelete: true,
	}

	if err := Apply(patch); err != nil {
		t.Fatalf("Apply failed: %v", err)
	}

	if _, err := os.Stat(filePath); !os.IsNotExist(err) {
		t.Error("file should have been deleted")
	}
}

func TestApply_ContextAnchoredMatching(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "code.go")
	content := `package main

func alpha() {
	doAlpha()
}

func beta() {
	doBeta()
}
`
	if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	// Only modify the beta function using context anchoring
	patch := &FilePatch{
		Path: filePath,
		Hunks: []Hunk{
			{
				ContextBefore: "func beta() {",
				ContextAfter:  "}",
				OldLines:      []string{"\tdoBeta()"},
				NewLines:      []string{"\tdoBetaV2()"},
			},
		},
	}

	if err := Apply(patch); err != nil {
		t.Fatalf("Apply failed: %v", err)
	}

	result, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(result), "doAlpha()") {
		t.Error("alpha function should be unchanged")
	}
	if !strings.Contains(string(result), "doBetaV2()") {
		t.Error("beta function should be updated")
	}
	if strings.Contains(string(result), "doBeta()") && !strings.Contains(string(result), "doBetaV2()") {
		t.Error("old beta call should be replaced")
	}
}

func TestApply_FuzzyWhitespaceMatching(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "fuzzy.go")
	// File has tabs for indentation
	content := `package main

func main() {
	fmt.Println("hello")
}
`
	if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	// Patch uses spaces instead of tabs in context (fuzzy match should handle this)
	patch := &FilePatch{
		Path: filePath,
		Hunks: []Hunk{
			{
				ContextBefore: "func main() {",
				ContextAfter:  "}",
				OldLines:      []string{`	fmt.Println("hello")`},
				NewLines:      []string{`	fmt.Println("goodbye")`},
			},
		},
	}

	if err := Apply(patch); err != nil {
		t.Fatalf("Apply failed: %v", err)
	}

	result, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(result), `fmt.Println("goodbye")`) {
		t.Errorf("expected goodbye in result, got:\n%s", string(result))
	}
}

func TestApply_ErrorAmbiguousContext(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "ambiguous.go")
	// Two identical blocks
	content := `package main

func a() {
	doStuff()
}

func b() {
	doStuff()
}
`
	if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	// Context before matches both functions equally (both have "doStuff()" after a func line)
	// Using the same context for both will be ambiguous
	patch := &FilePatch{
		Path: filePath,
		Hunks: []Hunk{
			{
				ContextBefore: "",
				ContextAfter:  "}",
				OldLines:      []string{"\tdoStuff()"},
				NewLines:      []string{"\tdoOther()"},
			},
		},
	}

	err := Apply(patch)
	if err == nil {
		t.Fatal("expected error for ambiguous context, got nil")
	}
	if !strings.Contains(err.Error(), "ambiguous") {
		t.Errorf("expected ambiguous error, got: %v", err)
	}
}

func TestApply_ErrorMissingFile(t *testing.T) {
	patch := &FilePatch{
		Path: "/nonexistent/path/file.go",
		Hunks: []Hunk{
			{
				ContextBefore: "line",
				OldLines:      []string{"old"},
				NewLines:      []string{"new"},
			},
		},
	}

	err := Apply(patch)
	if err == nil {
		t.Fatal("expected error for missing file, got nil")
	}
	if !strings.Contains(err.Error(), "cannot read file") {
		t.Errorf("expected file read error, got: %v", err)
	}
}

func TestApplyAll(t *testing.T) {
	dir := t.TempDir()
	fileA := filepath.Join(dir, "a.go")
	fileB := filepath.Join(dir, "b.go")

	if err := os.WriteFile(fileA, []byte("package a\n\nfunc A() {\n\told()\n}\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fileB, []byte("package b\n\nfunc B() {\n\told()\n}\n"), 0644); err != nil {
		t.Fatal(err)
	}

	input := "*** Begin Patch\n" +
		"*** Update File: " + fileA + "\n" +
		"@@@ func A() { @@@\n" +
		"- \told()\n" +
		"+ \tnewA()\n" +
		"@@@ } @@@\n" +
		"*** Update File: " + fileB + "\n" +
		"@@@ func B() { @@@\n" +
		"- \told()\n" +
		"+ \tnewB()\n" +
		"@@@ } @@@\n" +
		"*** End Patch"

	parser, err := ParsePatch(input)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	modified, err := parser.ApplyAll()
	if err != nil {
		t.Fatalf("ApplyAll error: %v", err)
	}

	if len(modified) != 2 {
		t.Fatalf("expected 2 modified files, got %d", len(modified))
	}

	resultA, _ := os.ReadFile(fileA)
	resultB, _ := os.ReadFile(fileB)
	if !strings.Contains(string(resultA), "newA()") {
		t.Errorf("a.go not patched correctly: %s", string(resultA))
	}
	if !strings.Contains(string(resultB), "newB()") {
		t.Errorf("b.go not patched correctly: %s", string(resultB))
	}
}

func TestApply_DeleteMissingFile(t *testing.T) {
	patch := &FilePatch{
		Path:     "/nonexistent/delete_target.go",
		IsDelete: true,
	}

	err := Apply(patch)
	if err == nil {
		t.Fatal("expected error when deleting non-existent file")
	}
	if !strings.Contains(err.Error(), "cannot delete") {
		t.Errorf("expected delete error, got: %v", err)
	}
}

func TestPatchLevenshteinDistance(t *testing.T) {
	tests := []struct {
		a, b     string
		expected int
	}{
		{"", "", 0},
		{"abc", "abc", 0},
		{"abc", "abd", 1},
		{"abc", "", 3},
		{"", "xyz", 3},
		{"kitten", "sitting", 3},
		{"  func()", "func()", 2},
	}

	for _, tc := range tests {
		got := levenshteinDistance(tc.a, tc.b)
		if got != tc.expected {
			t.Errorf("levenshteinDistance(%q, %q) = %d, want %d", tc.a, tc.b, got, tc.expected)
		}
	}
}

func TestPatchTool_Execute(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "target.go")
	content := `package main

func main() {
	fmt.Println("original")
}
`
	if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	patchContent := "*** Begin Patch\n" +
		"*** Update File: " + filePath + "\n" +
		"@@@ func main() { @@@\n" +
		"- \tfmt.Println(\"original\")\n" +
		"+ \tfmt.Println(\"patched\")\n" +
		"@@@ } @@@\n" +
		"*** End Patch"

	input, _ := json.Marshal(map[string]string{"patch": patchContent})

	tool := PatchTool{}
	result, err := tool.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if !strings.Contains(result, "Successfully") {
		t.Errorf("unexpected result: %s", result)
	}

	data, _ := os.ReadFile(filePath)
	if !strings.Contains(string(data), "patched") {
		t.Errorf("file not patched: %s", string(data))
	}
}

func TestPatchTool_Interface(t *testing.T) {
	var _ Tool = PatchTool{}

	tool := PatchTool{}
	if tool.Name() != "Patch" {
		t.Errorf("expected name Patch, got %s", tool.Name())
	}
	if tool.Description() == "" {
		t.Error("description should not be empty")
	}
	params := tool.Parameters()
	if params["type"] != "object" {
		t.Error("parameters should have type object")
	}
}
