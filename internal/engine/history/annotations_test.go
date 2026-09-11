package history

import (
	"strings"
	"sync"
	"testing"
)

func TestAnnotationManager_New(t *testing.T) {
	am := NewAnnotationManager()
	if am == nil {
		t.Fatal("expected non-nil AnnotationManager")
	}
	if am.Annotations == nil {
		t.Fatal("expected non-nil Annotations map")
	}
	if len(am.Annotations) != 0 {
		t.Fatalf("expected empty map, got %d entries", len(am.Annotations))
	}
}

func TestAnnotationManager_Add(t *testing.T) {
	am := NewAnnotationManager()

	a := am.Add("src/main.go", 10, "Needs error handling", "note", "agent")
	if a == nil {
		t.Fatal("expected non-nil annotation")
	}
	if a.ID == "" {
		t.Fatal("expected non-empty ID")
	}
	if a.File != "src/main.go" {
		t.Fatalf("expected file 'src/main.go', got %q", a.File)
	}
	if a.Line != 10 {
		t.Fatalf("expected line 10, got %d", a.Line)
	}
	if a.Content != "Needs error handling" {
		t.Fatalf("expected content 'Needs error handling', got %q", a.Content)
	}
	if a.Type != "note" {
		t.Fatalf("expected type 'note', got %q", a.Type)
	}
	if a.Author != "agent" {
		t.Fatalf("expected author 'agent', got %q", a.Author)
	}
	if a.Resolved {
		t.Fatal("expected annotation to not be resolved")
	}
	if a.CreatedAt.IsZero() {
		t.Fatal("expected non-zero CreatedAt")
	}

	anns := am.GetForFile("src/main.go")
	if len(anns) != 1 {
		t.Fatalf("expected 1 annotation, got %d", len(anns))
	}
}

func TestAnnotationManager_AddMultiple(t *testing.T) {
	am := NewAnnotationManager()

	am.Add("src/main.go", 10, "First", "note", "agent")
	am.Add("src/main.go", 20, "Second", "todo", "user")
	am.Add("src/other.go", 5, "Third", "warning", "agent")

	if len(am.GetForFile("src/main.go")) != 2 {
		t.Fatalf("expected 2 annotations for main.go")
	}
	if len(am.GetForFile("src/other.go")) != 1 {
		t.Fatalf("expected 1 annotation for other.go")
	}
	if len(am.GetAll()) != 3 {
		t.Fatalf("expected 3 total annotations")
	}
}

func TestAnnotationManager_Remove(t *testing.T) {
	am := NewAnnotationManager()

	a1 := am.Add("src/main.go", 10, "First", "note", "agent")
	am.Add("src/main.go", 20, "Second", "todo", "user")

	am.Remove(a1.ID)

	anns := am.GetForFile("src/main.go")
	if len(anns) != 1 {
		t.Fatalf("expected 1 annotation after remove, got %d", len(anns))
	}
	if anns[0].Content != "Second" {
		t.Fatalf("expected remaining annotation to be 'Second', got %q", anns[0].Content)
	}
}

func TestAnnotationManager_RemoveLastAnnotation(t *testing.T) {
	am := NewAnnotationManager()

	a := am.Add("src/main.go", 10, "Only one", "note", "agent")
	am.Remove(a.ID)

	anns := am.GetForFile("src/main.go")
	if len(anns) != 0 {
		t.Fatalf("expected 0 annotations after removing last, got %d", len(anns))
	}
	// File entry should be cleaned up.
	if _, exists := am.Annotations["src/main.go"]; exists {
		t.Fatal("expected file entry to be removed from map")
	}
}

func TestAnnotationManager_RemoveNonexistent(t *testing.T) {
	am := NewAnnotationManager()
	am.Add("src/main.go", 10, "Something", "note", "agent")

	// Should not panic.
	am.Remove("nonexistent-id")

	if len(am.GetAll()) != 1 {
		t.Fatal("expected annotations to be unchanged")
	}
}

func TestAnnotationManager_Resolve(t *testing.T) {
	am := NewAnnotationManager()

	a := am.Add("src/main.go", 10, "Fix this", "todo", "agent")
	if a.Resolved {
		t.Fatal("expected annotation to start unresolved")
	}

	am.Resolve(a.ID)

	anns := am.GetForFile("src/main.go")
	if !anns[0].Resolved {
		t.Fatal("expected annotation to be resolved")
	}
}

func TestAnnotationManager_GetUnresolved(t *testing.T) {
	am := NewAnnotationManager()

	a1 := am.Add("src/main.go", 10, "First", "note", "agent")
	am.Add("src/main.go", 20, "Second", "todo", "user")
	am.Add("src/other.go", 5, "Third", "warning", "agent")

	am.Resolve(a1.ID)

	unresolved := am.GetUnresolved()
	if len(unresolved) != 2 {
		t.Fatalf("expected 2 unresolved, got %d", len(unresolved))
	}
}

func TestAnnotationManager_InjectAnnotationsGo(t *testing.T) {
	am := NewAnnotationManager()
	am.Add("main.go", 2, "Check error here", "warning", "agent")

	content := "package main\n\nfunc main() {\n}\n"
	result := am.InjectAnnotations("main.go", content)

	if !strings.Contains(result, "// [hawk:warning] Check error here") {
		t.Fatalf("expected injected annotation, got:\n%s", result)
	}

	lines := strings.Split(result, "\n")
	// The annotation should be inserted above line 2 (0-indexed: index 1).
	if lines[1] != "// [hawk:warning] Check error here" {
		t.Fatalf("expected annotation at line index 1, got %q", lines[1])
	}
	// Original line 2 content should now be at index 2.
	if lines[2] != "" {
		t.Fatalf("expected original line 2 content at index 2, got %q", lines[2])
	}
}

func TestAnnotationManager_InjectAnnotationsPython(t *testing.T) {
	am := NewAnnotationManager()
	am.Add("script.py", 1, "Add type hints", "todo", "agent")

	content := "def hello():\n    pass\n"
	result := am.InjectAnnotations("script.py", content)

	if !strings.Contains(result, "# [hawk:todo] Add type hints") {
		t.Fatalf("expected python-style annotation, got:\n%s", result)
	}
}

func TestAnnotationManager_InjectAnnotationsJS(t *testing.T) {
	am := NewAnnotationManager()
	am.Add("app.ts", 3, "Potential XSS", "warning", "agent")

	content := "const a = 1;\nconst b = 2;\nconst c = a + b;\n"
	result := am.InjectAnnotations("app.ts", content)

	if !strings.Contains(result, "// [hawk:warning] Potential XSS") {
		t.Fatalf("expected JS/TS-style annotation, got:\n%s", result)
	}
}

func TestAnnotationManager_InjectAnnotationsResolved(t *testing.T) {
	am := NewAnnotationManager()
	a := am.Add("main.go", 2, "Already fixed", "note", "agent")
	am.Resolve(a.ID)

	content := "package main\n\nfunc main() {\n}\n"
	result := am.InjectAnnotations("main.go", content)

	if strings.Contains(result, "[hawk:") {
		t.Fatalf("resolved annotations should not be injected, got:\n%s", result)
	}
}

func TestAnnotationManager_InjectAnnotationsMultiple(t *testing.T) {
	am := NewAnnotationManager()
	am.Add("main.go", 1, "First note", "note", "agent")
	am.Add("main.go", 3, "Second note", "todo", "agent")

	content := "line1\nline2\nline3\nline4\n"
	result := am.InjectAnnotations("main.go", content)

	lines := strings.Split(result, "\n")
	// Line 1 annotation above line1, line 3 annotation above line3 (shifted).
	if lines[0] != "// [hawk:note] First note" {
		t.Fatalf("expected first annotation at index 0, got %q", lines[0])
	}
	// After inserting first annotation, original line3 is now at index 3,
	// and the second annotation goes above line3 (original), which becomes index 3.
	// With descending sort, line 3 annotation is inserted first, then line 1.
	// So: index 0 = annotation for L1, index 1 = "line1", index 2 = "line2",
	// index 3 = annotation for L3, index 4 = "line3"
	if lines[3] != "// [hawk:todo] Second note" {
		t.Fatalf("expected second annotation at index 3, got %q", lines[3])
	}
}

func TestAnnotation_StripAnnotations(t *testing.T) {
	content := `package main

// [hawk:note] This is a note
func main() {
	// [hawk:warning] Watch out
	fmt.Println("hello")
}
`
	result := StripAnnotations(content)

	if strings.Contains(result, "[hawk:") {
		t.Fatalf("expected all hawk annotations stripped, got:\n%s", result)
	}
	if !strings.Contains(result, "package main") {
		t.Fatal("expected non-annotation content preserved")
	}
	if !strings.Contains(result, `fmt.Println("hello")`) {
		t.Fatal("expected non-annotation content preserved")
	}
}

func TestAnnotation_StripAnnotationsPython(t *testing.T) {
	content := `# [hawk:todo] Add type hints
def hello():
    pass
`
	result := StripAnnotations(content)

	if strings.Contains(result, "[hawk:") {
		t.Fatalf("expected hawk annotation stripped, got:\n%s", result)
	}
	if !strings.Contains(result, "def hello():") {
		t.Fatal("expected non-annotation content preserved")
	}
}

func TestAnnotation_StripAnnotationsNoAnnotations(t *testing.T) {
	content := "package main\n\nfunc main() {}\n"
	result := StripAnnotations(content)

	if result != content {
		t.Fatalf("expected content unchanged, got:\n%s", result)
	}
}

func TestAnnotation_DetectAnnotations(t *testing.T) {
	content := `package main

// [hawk:note] Claims struct could be simplified
func main() {
	// [hawk:warning] No error check after ParseToken()
	token := ParseToken()
	// [hawk:todo] Add rate limiting here
	handle(token)
}
`
	anns := DetectAnnotations(content)
	if len(anns) != 3 {
		t.Fatalf("expected 3 annotations detected, got %d", len(anns))
	}

	if anns[0].Type != "note" {
		t.Fatalf("expected type 'note', got %q", anns[0].Type)
	}
	if anns[0].Content != "Claims struct could be simplified" {
		t.Fatalf("unexpected content: %q", anns[0].Content)
	}
	if anns[0].Line != 3 {
		t.Fatalf("expected line 3, got %d", anns[0].Line)
	}

	if anns[1].Type != "warning" {
		t.Fatalf("expected type 'warning', got %q", anns[1].Type)
	}
	if anns[1].Line != 5 {
		t.Fatalf("expected line 5, got %d", anns[1].Line)
	}

	if anns[2].Type != "todo" {
		t.Fatalf("expected type 'todo', got %q", anns[2].Type)
	}
	if anns[2].Line != 7 {
		t.Fatalf("expected line 7, got %d", anns[2].Line)
	}
}

func TestAnnotation_DetectAnnotationsPython(t *testing.T) {
	content := "# [hawk:question] Why is this needed?\ndef func():\n    pass\n"
	anns := DetectAnnotations(content)
	if len(anns) != 1 {
		t.Fatalf("expected 1 annotation, got %d", len(anns))
	}
	if anns[0].Type != "question" {
		t.Fatalf("expected type 'question', got %q", anns[0].Type)
	}
	if anns[0].Content != "Why is this needed?" {
		t.Fatalf("unexpected content: %q", anns[0].Content)
	}
}

func TestAnnotation_FormatAnnotations(t *testing.T) {
	anns := []*Annotation{
		{File: "src/auth.go", Line: 15, Type: "note", Content: "Claims struct could be simplified"},
		{File: "src/auth.go", Line: 32, Type: "warning", Content: "No error check after ParseToken()"},
		{File: "src/auth.go", Line: 45, Type: "todo", Content: "Add rate limiting here"},
		{File: "src/auth.go", Line: 67, Type: "question", Content: "Why is this deprecated function still used?"},
	}

	result := FormatAnnotations(anns)

	if !strings.Contains(result, "Annotations for src/auth.go:") {
		t.Fatalf("expected file header, got:\n%s", result)
	}
	if !strings.Contains(result, "L15 [note] Claims struct could be simplified") {
		t.Fatalf("expected L15 note, got:\n%s", result)
	}
	if !strings.Contains(result, "L32 [warning] No error check after ParseToken()") {
		t.Fatalf("expected L32 warning, got:\n%s", result)
	}
	if !strings.Contains(result, "L45 [todo] Add rate limiting here") {
		t.Fatalf("expected L45 todo, got:\n%s", result)
	}
	if !strings.Contains(result, "L67 [question] Why is this deprecated function still used?") {
		t.Fatalf("expected L67 question, got:\n%s", result)
	}
}

func TestAnnotation_FormatAnnotationsEmpty(t *testing.T) {
	result := FormatAnnotations(nil)
	if result != "" {
		t.Fatalf("expected empty string, got %q", result)
	}
}

func TestAnnotationManager_BuildContextFromAnnotations(t *testing.T) {
	am := NewAnnotationManager()
	am.Add("src/auth.go", 15, "Simplify this", "note", "agent")
	am.Add("src/auth.go", 32, "Missing error check", "warning", "agent")

	result := am.BuildContextFromAnnotations("src/auth.go")

	if !strings.Contains(result, "Annotations for src/auth.go:") {
		t.Fatalf("expected file header, got:\n%s", result)
	}
	if !strings.Contains(result, "L15 [note] Simplify this") {
		t.Fatalf("expected L15 note, got:\n%s", result)
	}
	if !strings.Contains(result, "L32 [warning] Missing error check") {
		t.Fatalf("expected L32 warning, got:\n%s", result)
	}
}

func TestAnnotationManager_BuildContextFromAnnotationsResolved(t *testing.T) {
	am := NewAnnotationManager()
	a := am.Add("src/auth.go", 15, "Fixed now", "note", "agent")
	am.Resolve(a.ID)

	result := am.BuildContextFromAnnotations("src/auth.go")
	if !strings.Contains(result, "[resolved]") {
		t.Fatalf("expected [resolved] marker, got:\n%s", result)
	}
}

func TestAnnotationManager_BuildContextFromAnnotationsEmpty(t *testing.T) {
	am := NewAnnotationManager()
	result := am.BuildContextFromAnnotations("nonexistent.go")
	if result != "" {
		t.Fatalf("expected empty string, got %q", result)
	}
}

func TestAnnotationManager_Summary(t *testing.T) {
	am := NewAnnotationManager()

	if am.Summary() != "0 annotations" {
		t.Fatalf("expected '0 annotations', got %q", am.Summary())
	}

	a1 := am.Add("main.go", 10, "A warning", "warning", "agent")
	am.Add("main.go", 20, "A todo", "todo", "agent")
	am.Add("main.go", 30, "Another todo", "todo", "agent")

	result := am.Summary()
	if !strings.Contains(result, "3 annotations") {
		t.Fatalf("expected '3 annotations' in summary, got %q", result)
	}
	if !strings.Contains(result, "3 unresolved") {
		t.Fatalf("expected '3 unresolved' in summary, got %q", result)
	}
	if !strings.Contains(result, "1 warning") {
		t.Fatalf("expected '1 warning' in summary, got %q", result)
	}
	if !strings.Contains(result, "2 todos") {
		t.Fatalf("expected '2 todos' in summary, got %q", result)
	}

	am.Resolve(a1.ID)
	result = am.Summary()
	if !strings.Contains(result, "2 unresolved") {
		t.Fatalf("expected '2 unresolved' after resolving one, got %q", result)
	}
}

func TestAnnotationManager_SummaryAllResolved(t *testing.T) {
	am := NewAnnotationManager()
	a := am.Add("main.go", 10, "Done", "note", "agent")
	am.Resolve(a.ID)

	result := am.Summary()
	if result != "1 annotations (all resolved)" {
		t.Fatalf("expected all resolved summary, got %q", result)
	}
}

func TestAnnotationManager_ConcurrentAccess(t *testing.T) {
	am := NewAnnotationManager()
	var wg sync.WaitGroup

	// Concurrent adds.
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			am.Add("main.go", n, "Concurrent annotation", "note", "agent")
		}(i)
	}
	wg.Wait()

	all := am.GetAll()
	if len(all) != 100 {
		t.Fatalf("expected 100 annotations, got %d", len(all))
	}

	// Concurrent reads while resolving.
	for i := 0; i < 50; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			am.GetAll()
		}()
		go func(id string) {
			defer wg.Done()
			am.Resolve(id)
		}(all[i].ID)
	}
	wg.Wait()

	unresolved := am.GetUnresolved()
	if len(unresolved) != 50 {
		t.Fatalf("expected 50 unresolved, got %d", len(unresolved))
	}
}

func TestAnnotation_CommentPrefix(t *testing.T) {
	tests := []struct {
		file   string
		expect string
	}{
		{"main.go", "//"},
		{"script.py", "#"},
		{"app.ts", "//"},
		{"index.jsx", "//"},
		{"style.css", "/*"},
		{"page.html", "<!--"},
		{"query.sql", "--"},
		{"config.yaml", "#"},
		{"Makefile", "//"},
	}

	for _, tc := range tests {
		got := annotationCommentPrefix(tc.file)
		if got != tc.expect {
			t.Errorf("annotationCommentPrefix(%q) = %q, want %q", tc.file, got, tc.expect)
		}
	}
}

func TestAnnotation_StripAndInjectRoundTrip(t *testing.T) {
	am := NewAnnotationManager()
	am.Add("main.go", 2, "Important note", "note", "agent")
	am.Add("main.go", 4, "Fix this", "todo", "agent")

	original := "package main\n\nimport \"fmt\"\n\nfunc main() {\n\tfmt.Println(\"hello\")\n}\n"

	injected := am.InjectAnnotations("main.go", original)
	if !strings.Contains(injected, "[hawk:note]") {
		t.Fatal("expected annotations in injected content")
	}
	if !strings.Contains(injected, "[hawk:todo]") {
		t.Fatal("expected annotations in injected content")
	}

	stripped := StripAnnotations(injected)
	if stripped != original {
		t.Fatalf("round-trip failed.\nOriginal:\n%s\nStripped:\n%s", original, stripped)
	}
}

func TestAnnotationManager_UniqueIDs(t *testing.T) {
	am := NewAnnotationManager()
	ids := make(map[string]bool)

	for i := 0; i < 100; i++ {
		a := am.Add("main.go", i, "annotation", "note", "agent")
		if ids[a.ID] {
			t.Fatalf("duplicate ID: %s", a.ID)
		}
		ids[a.ID] = true
	}
}
