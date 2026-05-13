package tool

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestNewAutoImporter(t *testing.T) {
	ai := NewAutoImporter()
	if ai == nil {
		t.Fatal("NewAutoImporter returned nil")
	}
	if len(ai.KnownPackages) < 200 {
		t.Errorf("expected 200+ known packages, got %d", len(ai.KnownPackages))
	}

	// Spot-check some stdlib packages.
	checks := map[string]string{
		"fmt":      "fmt",
		"json":     "encoding/json",
		"http":     "net/http",
		"filepath": "path/filepath",
		"context":  "context",
		"sha256":   "crypto/sha256",
		"ast":      "go/ast",
		"parser":   "go/parser",
		"token":    "go/token",
	}
	for sym, expected := range checks {
		if got := ai.KnownPackages[sym]; got != expected {
			t.Errorf("KnownPackages[%q] = %q, want %q", sym, got, expected)
		}
	}

	// Spot-check some third-party packages.
	thirdPartyChecks := map[string]string{
		"chi":     "github.com/go-chi/chi/v5",
		"gin":     "github.com/gin-gonic/gin",
		"cobra":   "github.com/spf13/cobra",
		"viper":   "github.com/spf13/viper",
		"zap":     "go.uber.org/zap",
		"assert":  "github.com/stretchr/testify/assert",
		"sqlx":    "github.com/jmoiron/sqlx",
		"gorm":    "gorm.io/gorm",
		"uuid":    "github.com/google/uuid",
	}
	for sym, expected := range thirdPartyChecks {
		if got := ai.KnownPackages[sym]; got != expected {
			t.Errorf("KnownPackages[%q] = %q, want %q", sym, got, expected)
		}
	}
}

func TestDetectMissing(t *testing.T) {
	ai := NewAutoImporter()

	tests := []struct {
		name     string
		code     string
		expected []string
	}{
		{
			name: "single missing import",
			code: `package main

func main() {
	fmt.Println("hello")
}
`,
			expected: []string{"fmt.Println"},
		},
		{
			name: "multiple missing imports",
			code: `package main

func main() {
	fmt.Println("hello")
	http.Get("http://example.com")
	json.Marshal(nil)
}
`,
			expected: []string{"fmt.Println", "http.Get", "json.Marshal"},
		},
		{
			name: "no missing imports - all imported",
			code: `package main

import (
	"fmt"
	"net/http"
)

func main() {
	fmt.Println("hello")
	http.Get("http://example.com")
}
`,
			expected: nil,
		},
		{
			name: "mixed - some imported some not",
			code: `package main

import "fmt"

func main() {
	fmt.Println("hello")
	json.Marshal(nil)
}
`,
			expected: []string{"json.Marshal"},
		},
		{
			name: "skip variable field access",
			code: `package main

import "fmt"

func main() {
	myVar := struct{ Name string }{"test"}
	fmt.Println(myVar.Name)
}
`,
			expected: nil,
		},
		{
			name: "no package references",
			code: `package main

func main() {
	x := 42
	_ = x
}
`,
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ai.DetectMissing(tt.code)
			if len(got) != len(tt.expected) {
				t.Errorf("DetectMissing() returned %d results, want %d\n  got: %v\n  want: %v",
					len(got), len(tt.expected), got, tt.expected)
				return
			}
			for i, sym := range tt.expected {
				if got[i] != sym {
					t.Errorf("DetectMissing()[%d] = %q, want %q", i, got[i], sym)
				}
			}
		})
	}
}

func TestResolve(t *testing.T) {
	ai := NewAutoImporter()

	tests := []struct {
		name          string
		code          string
		expectedPkgs  []string
		expectedPaths []string
	}{
		{
			name: "resolve fmt",
			code: `package main

func main() {
	fmt.Println("hello")
}
`,
			expectedPkgs:  []string{"fmt"},
			expectedPaths: []string{"fmt"},
		},
		{
			name: "resolve multiple stdlib",
			code: `package main

func main() {
	fmt.Println(strings.Join(os.Args, " "))
}
`,
			expectedPkgs:  []string{"fmt", "os", "strings"},
			expectedPaths: []string{"fmt", "os", "strings"},
		},
		{
			name: "resolve third-party",
			code: `package main

func main() {
	r := chi.NewRouter()
	zap.NewProduction()
	_ = r
}
`,
			expectedPkgs:  []string{"chi", "zap"},
			expectedPaths: []string{"github.com/go-chi/chi/v5", "go.uber.org/zap"},
		},
		{
			name: "no resolution needed",
			code: `package main

import "fmt"

func main() {
	fmt.Println("already imported")
}
`,
			expectedPkgs:  nil,
			expectedPaths: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fixes := ai.Resolve(tt.code)
			if len(fixes) != len(tt.expectedPkgs) {
				t.Errorf("Resolve() returned %d fixes, want %d", len(fixes), len(tt.expectedPkgs))
				for _, f := range fixes {
					t.Logf("  fix: pkg=%s path=%s symbol=%s", f.Package, f.Path, f.Symbol)
				}
				return
			}
			for i, fix := range fixes {
				if fix.Package != tt.expectedPkgs[i] {
					t.Errorf("fix[%d].Package = %q, want %q", i, fix.Package, tt.expectedPkgs[i])
				}
				if fix.Path != tt.expectedPaths[i] {
					t.Errorf("fix[%d].Path = %q, want %q", i, fix.Path, tt.expectedPaths[i])
				}
			}
		})
	}
}

func TestApplyFixes(t *testing.T) {
	ai := NewAutoImporter()

	tests := []struct {
		name     string
		code     string
		fixes    []ImportFix
		contains []string
	}{
		{
			name: "add import to code without imports",
			code: `package main

func main() {
	fmt.Println("hello")
}
`,
			fixes: []ImportFix{
				{Package: "fmt", Path: "fmt", Symbol: "fmt.Println"},
			},
			contains: []string{`"fmt"`},
		},
		{
			name: "add to existing grouped import",
			code: `package main

import (
	"os"
)

func main() {
	os.Exit(1)
	fmt.Println("hello")
}
`,
			fixes: []ImportFix{
				{Package: "fmt", Path: "fmt", Symbol: "fmt.Println"},
			},
			contains: []string{`"fmt"`, `"os"`},
		},
		{
			name: "add external import creates separate group",
			code: `package main

func main() {
	fmt.Println("hello")
	chi.NewRouter()
}
`,
			fixes: []ImportFix{
				{Package: "fmt", Path: "fmt", Symbol: "fmt.Println"},
				{Package: "chi", Path: "github.com/go-chi/chi/v5", Symbol: "chi.NewRouter"},
			},
			contains: []string{`"fmt"`, `"github.com/go-chi/chi/v5"`},
		},
		{
			name: "no fixes returns unchanged code",
			code: `package main

func main() {}
`,
			fixes:    nil,
			contains: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ai.ApplyFixes(tt.code, tt.fixes)
			for _, want := range tt.contains {
				if !strings.Contains(result, want) {
					t.Errorf("ApplyFixes() result missing %q\nGot:\n%s", want, result)
				}
			}
			// Ensure original code body is preserved.
			if tt.fixes != nil && !strings.Contains(result, "func main()") {
				t.Error("ApplyFixes() lost the function body")
			}
		})
	}
}

func TestApplyFixes_GroupsCorrectly(t *testing.T) {
	ai := NewAutoImporter()

	code := `package main

func main() {
	fmt.Println(strings.Join(os.Args, " "))
	chi.NewRouter()
}
`
	fixes := []ImportFix{
		{Package: "fmt", Path: "fmt"},
		{Package: "strings", Path: "strings"},
		{Package: "os", Path: "os"},
		{Package: "chi", Path: "github.com/go-chi/chi/v5"},
	}

	result := ai.ApplyFixes(code, fixes)

	// Verify grouping: stdlib should appear before external.
	fmtIdx := strings.Index(result, `"fmt"`)
	chiIdx := strings.Index(result, `"github.com/go-chi/chi/v5"`)
	if fmtIdx < 0 || chiIdx < 0 {
		t.Fatalf("missing expected imports in result:\n%s", result)
	}
	if fmtIdx > chiIdx {
		t.Error("stdlib imports should come before external imports")
	}

	// There should be a blank line separating the groups.
	between := result[fmtIdx:chiIdx]
	if !strings.Contains(between, "\n\n") {
		t.Error("expected blank line between stdlib and external import groups")
	}
}

func TestSuggestImport(t *testing.T) {
	ai := NewAutoImporter()

	tests := []struct {
		symbol   string
		expected []string
	}{
		{"fmt", []string{"fmt"}},
		{"json", []string{"encoding/json"}},
		{"http", []string{"net/http"}},
		{"chi", []string{"github.com/go-chi/chi/v5"}},
		{"nonexistent_package_xyz", nil},
	}

	for _, tt := range tests {
		t.Run(tt.symbol, func(t *testing.T) {
			suggestions := ai.SuggestImport(tt.symbol)
			if len(tt.expected) == 0 {
				if len(suggestions) != 0 {
					t.Errorf("SuggestImport(%q) = %v, want empty", tt.symbol, suggestions)
				}
				return
			}
			if len(suggestions) == 0 {
				t.Errorf("SuggestImport(%q) returned no suggestions", tt.symbol)
				return
			}
			// Check the first suggestion matches.
			if suggestions[0] != tt.expected[0] {
				t.Errorf("SuggestImport(%q)[0] = %q, want %q", tt.symbol, suggestions[0], tt.expected[0])
			}
		})
	}
}

func TestSuggestImport_PartialMatch(t *testing.T) {
	ai := NewAutoImporter()

	// "sql" should match "sql" directly and potentially partial matches.
	suggestions := ai.SuggestImport("sql")
	if len(suggestions) == 0 {
		t.Fatal("expected at least one suggestion for 'sql'")
	}
	if suggestions[0] != "database/sql" {
		t.Errorf("expected first suggestion to be 'database/sql', got %q", suggestions[0])
	}
}

func TestRegisterPackage(t *testing.T) {
	ai := NewAutoImporter()

	// Register a custom package.
	ai.RegisterPackage("myutil", "github.com/myorg/myutil")

	// Verify it was added.
	if got := ai.KnownPackages["myutil"]; got != "github.com/myorg/myutil" {
		t.Errorf("RegisterPackage failed: got %q, want %q", got, "github.com/myorg/myutil")
	}

	// It should now resolve.
	code := `package main

func main() {
	myutil.DoStuff()
}
`
	fixes := ai.Resolve(code)
	if len(fixes) != 1 {
		t.Fatalf("expected 1 fix, got %d", len(fixes))
	}
	if fixes[0].Path != "github.com/myorg/myutil" {
		t.Errorf("fix.Path = %q, want %q", fixes[0].Path, "github.com/myorg/myutil")
	}
}

func TestRegisterPackage_Override(t *testing.T) {
	ai := NewAutoImporter()

	// Override an existing mapping.
	ai.RegisterPackage("json", "github.com/custom/json")
	if got := ai.KnownPackages["json"]; got != "github.com/custom/json" {
		t.Errorf("override failed: got %q, want %q", got, "github.com/custom/json")
	}
}

func TestFormatFixes(t *testing.T) {
	ai := NewAutoImporter()

	t.Run("no fixes", func(t *testing.T) {
		result := ai.FormatFixes(nil)
		if !strings.Contains(result, "No missing imports") {
			t.Errorf("expected 'No missing imports' message, got %q", result)
		}
	})

	t.Run("with fixes", func(t *testing.T) {
		fixes := []ImportFix{
			{Package: "fmt", Path: "fmt", Symbol: "fmt.Println", Line: 5},
			{Package: "json", Path: "encoding/json", Symbol: "json.Marshal", Line: 10},
		}
		result := ai.FormatFixes(fixes)
		if !strings.Contains(result, "2 missing import(s)") {
			t.Errorf("expected count in output, got %q", result)
		}
		if !strings.Contains(result, `"fmt"`) {
			t.Error("missing fmt in output")
		}
		if !strings.Contains(result, `"encoding/json"`) {
			t.Error("missing encoding/json in output")
		}
		if !strings.Contains(result, "line 5") {
			t.Error("missing line number for fmt")
		}
		if !strings.Contains(result, "line 10") {
			t.Error("missing line number for json")
		}
	})
}

func TestAutoImportTool_Interface(t *testing.T) {
	tool := NewAutoImportTool()

	if tool.Name() != "AutoImport" {
		t.Errorf("Name() = %q, want %q", tool.Name(), "AutoImport")
	}
	if tool.Description() == "" {
		t.Error("Description() should not be empty")
	}
	params := tool.Parameters()
	if params == nil {
		t.Fatal("Parameters() returned nil")
	}
	props, ok := params["properties"].(map[string]interface{})
	if !ok {
		t.Fatal("Parameters() missing properties")
	}
	if _, ok := props["code"]; !ok {
		t.Error("Parameters() missing 'code' property")
	}
}

func TestAutoImportTool_Execute_Report(t *testing.T) {
	tool := NewAutoImportTool()

	code := `package main

func main() {
	fmt.Println("hello")
	json.Marshal(nil)
}
`
	input, _ := json.Marshal(map[string]interface{}{
		"code":  code,
		"apply": false,
	})

	result, err := tool.Execute(nil, input)
	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}
	if !strings.Contains(result, "missing import") {
		t.Errorf("expected 'missing import' in result, got %q", result)
	}
	if !strings.Contains(result, "fmt") {
		t.Error("result should mention fmt")
	}
	if !strings.Contains(result, "encoding/json") {
		t.Error("result should mention encoding/json")
	}
}

func TestAutoImportTool_Execute_Apply(t *testing.T) {
	tool := NewAutoImportTool()

	code := `package main

func main() {
	fmt.Println("hello")
	json.Marshal(nil)
}
`
	input, _ := json.Marshal(map[string]interface{}{
		"code":  code,
		"apply": true,
	})

	result, err := tool.Execute(nil, input)
	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}
	if !strings.Contains(result, `"fmt"`) {
		t.Error("applied result should contain fmt import")
	}
	if !strings.Contains(result, `"encoding/json"`) {
		t.Error("applied result should contain encoding/json import")
	}
	if !strings.Contains(result, "func main()") {
		t.Error("applied result should preserve function body")
	}
}

func TestAutoImportTool_Execute_EmptyCode(t *testing.T) {
	tool := NewAutoImportTool()

	input, _ := json.Marshal(map[string]interface{}{
		"code": "",
	})

	_, err := tool.Execute(nil, input)
	if err == nil {
		t.Error("expected error for empty code")
	}
}

func TestAutoImportTool_Execute_WithFile(t *testing.T) {
	tool := NewAutoImportTool()

	code := `package main

func main() {
	fmt.Println("hello")
}
`
	input, _ := json.Marshal(map[string]interface{}{
		"code":  code,
		"file":  "/path/to/main.go",
		"apply": false,
	})

	result, err := tool.Execute(nil, input)
	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}
	if !strings.Contains(result, "fmt") {
		t.Error("result should mention fmt")
	}
}

func TestDetectMissing_ComplexCode(t *testing.T) {
	ai := NewAutoImporter()

	code := `package main

import (
	"fmt"
)

func handler(w http.ResponseWriter, r *http.Request) {
	data, err := json.Marshal(map[string]string{"hello": "world"})
	if err != nil {
		log.Fatal(err)
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(data)
	fmt.Println("served request")
}

func main() {
	ctx := context.Background()
	_ = ctx
	http.HandleFunc("/", handler)
	http.ListenAndServe(":8080", nil)
}
`
	missing := ai.DetectMissing(code)

	// "fmt" is imported, so it shouldn't be in missing.
	for _, sym := range missing {
		if strings.HasPrefix(sym, "fmt.") {
			t.Errorf("fmt should not be in missing (it's imported), got %q", sym)
		}
	}

	// http, json, log, context should be missing.
	expectedPkgs := map[string]bool{
		"http":    false,
		"json":    false,
		"log":     false,
		"context": false,
	}
	for _, sym := range missing {
		parts := strings.SplitN(sym, ".", 2)
		if _, ok := expectedPkgs[parts[0]]; ok {
			expectedPkgs[parts[0]] = true
		}
	}
	for pkg, found := range expectedPkgs {
		if !found {
			t.Errorf("expected %q to be detected as missing", pkg)
		}
	}
}

func TestApplyFixes_ExistingSingleImport(t *testing.T) {
	ai := NewAutoImporter()

	code := `package main

import "os"

func main() {
	os.Exit(1)
	fmt.Println("bye")
}
`
	fixes := []ImportFix{
		{Package: "fmt", Path: "fmt", Symbol: "fmt.Println"},
	}

	result := ai.ApplyFixes(code, fixes)
	if !strings.Contains(result, `"fmt"`) {
		t.Errorf("result should contain fmt import, got:\n%s", result)
	}
	if !strings.Contains(result, `"os"`) {
		t.Error("result should preserve existing os import")
	}
}

func TestApplyFixes_AlreadyImported(t *testing.T) {
	ai := NewAutoImporter()

	code := `package main

import "fmt"

func main() {
	fmt.Println("hello")
}
`
	fixes := []ImportFix{
		{Package: "fmt", Path: "fmt", Symbol: "fmt.Println"},
	}

	result := ai.ApplyFixes(code, fixes)
	// Should not duplicate the import.
	count := strings.Count(result, `"fmt"`)
	if count > 1 {
		t.Errorf("fmt import duplicated (%d times)", count)
	}
}

func TestResolve_LineNumbers(t *testing.T) {
	ai := NewAutoImporter()

	code := `package main

func main() {
	fmt.Println("hello")
	json.Marshal(nil)
}
`
	fixes := ai.Resolve(code)
	for _, fix := range fixes {
		if fix.Line <= 0 {
			t.Errorf("fix for %s has invalid line number: %d", fix.Symbol, fix.Line)
		}
	}

	// fmt.Println should be on line 4.
	for _, fix := range fixes {
		if fix.Package == "fmt" && fix.Line != 4 {
			t.Errorf("fmt.Println expected on line 4, got %d", fix.Line)
		}
	}
}

func TestAutoImporter_ConcurrentAccess(t *testing.T) {
	ai := NewAutoImporter()
	done := make(chan struct{})

	// Concurrent reads.
	for i := 0; i < 10; i++ {
		go func() {
			defer func() { done <- struct{}{} }()
			_ = ai.SuggestImport("fmt")
		}()
	}

	// Concurrent writes.
	for i := 0; i < 10; i++ {
		go func(n int) {
			defer func() { done <- struct{}{} }()
			ai.RegisterPackage(
				strings.Repeat("x", n+1),
				"github.com/test/pkg"+strings.Repeat("x", n+1),
			)
		}(i)
	}

	// Wait for all goroutines.
	for i := 0; i < 20; i++ {
		<-done
	}
}

func TestBuildImportBlock_SingleImport(t *testing.T) {
	result := buildImportBlock([]string{"fmt"}, nil, nil)
	expected := "import \"fmt\"\n"
	if result != expected {
		t.Errorf("buildImportBlock single = %q, want %q", result, expected)
	}
}

func TestBuildImportBlock_MultipleGroups(t *testing.T) {
	result := buildImportBlock(
		[]string{"fmt", "os"},
		[]string{"github.com/foo/bar"},
		nil,
	)
	if !strings.Contains(result, "import (") {
		t.Error("expected grouped import block")
	}
	if !strings.Contains(result, `"fmt"`) {
		t.Error("missing fmt")
	}
	if !strings.Contains(result, `"os"`) {
		t.Error("missing os")
	}
	if !strings.Contains(result, `"github.com/foo/bar"`) {
		t.Error("missing github.com/foo/bar")
	}
}

func TestExtractExistingImports(t *testing.T) {
	code := `package main

import (
	"fmt"
	"os"

	"github.com/foo/bar"
)
`
	imports := extractExistingImports(code)
	if !imports["fmt"] {
		t.Error("missing fmt")
	}
	if !imports["os"] {
		t.Error("missing os")
	}
	if !imports["github.com/foo/bar"] {
		t.Error("missing github.com/foo/bar")
	}
}

func TestExtractImportedPackageNames(t *testing.T) {
	code := `package main

import (
	"fmt"
	"net/http"
	myjson "encoding/json"
)
`
	names := extractImportedPackageNames(code)
	if !names["fmt"] {
		t.Error("missing fmt")
	}
	if !names["http"] {
		t.Error("missing http (from net/http)")
	}
	if !names["myjson"] {
		t.Error("missing myjson alias")
	}
}

func TestIsLikelyFieldAccess(t *testing.T) {
	code := `package main

func main() {
	myStruct := MyStruct{Field: "value"}
	_ = myStruct.Field
}
`
	if !isLikelyFieldAccess(code, "myStruct") {
		t.Error("expected myStruct to be detected as field access")
	}
	if isLikelyFieldAccess(code, "fmt") {
		t.Error("fmt should not be detected as field access")
	}
}

func TestEndToEnd_FullResolveAndApply(t *testing.T) {
	ai := NewAutoImporter()

	code := `package main

func main() {
	ctx := context.Background()
	r := chi.NewRouter()
	r.Get("/", func(w http.ResponseWriter, req *http.Request) {
		data, _ := json.Marshal(map[string]string{"status": "ok"})
		w.Write(data)
	})
	fmt.Printf("starting server on %s\n", ":8080")
	http.ListenAndServe(":8080", r)
	_ = ctx
}
`
	fixes := ai.Resolve(code)
	if len(fixes) == 0 {
		t.Fatal("expected fixes")
	}

	result := ai.ApplyFixes(code, fixes)

	// Verify all needed imports are present.
	requiredImports := []string{`"fmt"`, `"encoding/json"`, `"net/http"`, `"context"`, `"github.com/go-chi/chi/v5"`}
	for _, imp := range requiredImports {
		if !strings.Contains(result, imp) {
			t.Errorf("result missing import %s\nGot:\n%s", imp, result)
		}
	}

	// Verify function body is preserved.
	if !strings.Contains(result, "context.Background()") {
		t.Error("lost context.Background() call")
	}
	if !strings.Contains(result, "chi.NewRouter()") {
		t.Error("lost chi.NewRouter() call")
	}
}
