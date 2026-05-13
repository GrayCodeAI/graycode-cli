package engine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNewDocUpdater(t *testing.T) {
	du := NewDocUpdater()
	if du == nil {
		t.Fatal("NewDocUpdater returned nil")
	}
}

func TestDetectStaleDocumentation_SignatureChanged(t *testing.T) {
	du := NewDocUpdater()

	oldContent := `package main

// ValidateToken validates a JWT token
func ValidateToken(token string) bool {
	return true
}
`

	newContent := `package main

// ValidateToken validates a JWT token
func ValidateToken(ctx context.Context, token string) bool {
	return true
}
`

	updates := du.DetectStaleDocumentation("src/auth.go", oldContent, newContent)
	if len(updates) == 0 {
		t.Fatal("expected at least one update for signature change")
	}

	found := false
	for _, u := range updates {
		if u.Symbol == "ValidateToken" {
			found = true
			if !strings.Contains(u.Reason, "signature_changed") {
				t.Errorf("expected reason to contain 'signature_changed', got %q", u.Reason)
			}
			if u.File != "src/auth.go" {
				t.Errorf("expected file 'src/auth.go', got %q", u.File)
			}
			if u.OldDoc == "" {
				t.Error("expected OldDoc to be set")
			}
			if u.NewDoc == "" {
				t.Error("expected NewDoc to be set")
			}
		}
	}
	if !found {
		t.Error("did not find update for ValidateToken")
	}
}

func TestDetectStaleDocumentation_NewParams(t *testing.T) {
	du := NewDocUpdater()

	oldContent := `package main

// ProcessData handles data processing
func ProcessData(data []byte) error {
	return nil
}
`

	newContent := `package main

// ProcessData handles data processing
func ProcessData(data []byte, timeout int, retries int) error {
	return nil
}
`

	updates := du.DetectStaleDocumentation("src/process.go", oldContent, newContent)
	if len(updates) == 0 {
		t.Fatal("expected at least one update for new params")
	}

	found := false
	for _, u := range updates {
		if u.Symbol == "ProcessData" {
			found = true
			if !strings.Contains(u.Reason, "signature_changed") && !strings.Contains(u.Reason, "new_params") {
				t.Errorf("expected reason to be signature_changed or new_params, got %q", u.Reason)
			}
		}
	}
	if !found {
		t.Error("did not find update for ProcessData")
	}
}

func TestDetectStaleDocumentation_OutdatedReference(t *testing.T) {
	du := NewDocUpdater()

	oldContent := `package main

// ProcessRequest uses oldHelper to process incoming requests
func ProcessRequest(r *http.Request) error {
	return nil
}

func oldHelper() {}
`

	newContent := `package main

// ProcessRequest uses oldHelper to process incoming requests
func ProcessRequest(r *http.Request) error {
	return nil
}
`

	updates := du.DetectStaleDocumentation("src/handler.go", oldContent, newContent)
	if len(updates) == 0 {
		t.Fatal("expected at least one update for outdated reference")
	}

	found := false
	for _, u := range updates {
		if u.Symbol == "ProcessRequest" && u.Reason == "outdated_reference" {
			found = true
			if !strings.Contains(u.NewDoc, "[removed:oldHelper]") {
				t.Errorf("expected NewDoc to contain removed marker, got %q", u.NewDoc)
			}
		}
	}
	if !found {
		t.Error("did not find outdated_reference update for ProcessRequest")
	}
}

func TestDetectStaleDocumentation_NoChanges(t *testing.T) {
	du := NewDocUpdater()

	content := `package main

// Hello says hello
func Hello(name string) string {
	return "hello " + name
}
`

	updates := du.DetectStaleDocumentation("src/hello.go", content, content)
	if len(updates) != 0 {
		t.Errorf("expected no updates when content is unchanged, got %d", len(updates))
	}
}

func TestGenerateDocUpdate(t *testing.T) {
	du := NewDocUpdater()

	tests := []struct {
		name      string
		funcName  string
		signature string
		oldDoc    string
		wantSub   string
	}{
		{
			name:      "add context param",
			funcName:  "ValidateToken",
			signature: "(ctx context.Context, token string) bool",
			oldDoc:    "// ValidateToken validates a JWT token",
			wantSub:   "context",
		},
		{
			name:      "add new named param",
			funcName:  "Process",
			signature: "(data []byte, limit int) error",
			oldDoc:    "// Process processes the data",
			wantSub:   "limit",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := du.GenerateDocUpdate(tt.funcName, tt.signature, tt.oldDoc)
			if !strings.Contains(result, tt.wantSub) {
				t.Errorf("GenerateDocUpdate() = %q, want substring %q", result, tt.wantSub)
			}
			if !strings.HasPrefix(result, "//") {
				t.Errorf("GenerateDocUpdate() should start with //, got %q", result)
			}
		})
	}
}

func TestGenerateDocUpdate_PreservesPrefix(t *testing.T) {
	du := NewDocUpdater()

	result := du.GenerateDocUpdate("Foo", "(bar int) string", "// Foo does something")
	if !strings.HasPrefix(result, "// Foo") {
		t.Errorf("expected doc to preserve 'Foo' prefix, got %q", result)
	}
}

func TestScanProjectForStaleDocs(t *testing.T) {
	du := NewDocUpdater()

	// Create a temp project
	dir := t.TempDir()

	// Create a file with a reference to a non-existent symbol
	content := `package main

// HandleRequest uses NonExistentProcessor to handle requests
func HandleRequest() error {
	return nil
}

// DoWork performs work
func DoWork() {
}
`
	err := os.WriteFile(filepath.Join(dir, "main.go"), []byte(content), 0644)
	if err != nil {
		t.Fatal(err)
	}

	updates := du.ScanProjectForStaleDocs(dir)

	found := false
	for _, u := range updates {
		if u.Symbol == "HandleRequest" && u.Reason == "outdated_reference" {
			found = true
		}
	}
	if !found {
		t.Error("expected to find outdated_reference for HandleRequest referencing NonExistentProcessor")
	}
}

func TestScanProjectForStaleDocs_NoStale(t *testing.T) {
	du := NewDocUpdater()

	dir := t.TempDir()

	content := `package main

// DoWork performs work
func DoWork() {
}

// Helper assists DoWork
func Helper() {
}
`
	err := os.WriteFile(filepath.Join(dir, "main.go"), []byte(content), 0644)
	if err != nil {
		t.Fatal(err)
	}

	updates := du.ScanProjectForStaleDocs(dir)

	// Helper references DoWork which exists, so no stale docs
	for _, u := range updates {
		if u.Symbol == "Helper" {
			t.Errorf("unexpected stale doc for Helper: %+v", u)
		}
	}
}

func TestScanProjectForStaleDocs_SkipsVendor(t *testing.T) {
	du := NewDocUpdater()

	dir := t.TempDir()

	// Create vendor directory with stale docs
	vendorDir := filepath.Join(dir, "vendor")
	os.MkdirAll(vendorDir, 0755)

	vendorContent := `package vendor

// BadFunc uses MissingThing
func BadFunc() {}
`
	os.WriteFile(filepath.Join(vendorDir, "bad.go"), []byte(vendorContent), 0644)

	// Create main file without issues
	mainContent := `package main

// DoWork performs work
func DoWork() {
}
`
	os.WriteFile(filepath.Join(dir, "main.go"), []byte(mainContent), 0644)

	updates := du.ScanProjectForStaleDocs(dir)

	for _, u := range updates {
		if strings.Contains(u.File, "vendor") {
			t.Errorf("should skip vendor directory, found update: %+v", u)
		}
	}
}

func TestFormatUpdates_Empty(t *testing.T) {
	du := NewDocUpdater()

	result := du.FormatUpdates(nil)
	if result != "No stale documentation found." {
		t.Errorf("unexpected output for empty updates: %q", result)
	}
}

func TestFormatUpdates_Multiple(t *testing.T) {
	du := NewDocUpdater()

	updates := []DocUpdate{
		{
			File:   "src/auth.go",
			Line:   15,
			OldDoc: "// ValidateToken validates a JWT token",
			NewDoc: "// ValidateToken validates a JWT token using the provided context",
			Symbol: "ValidateToken",
			Reason: "signature_changed (added ctx parameter)",
		},
		{
			File:   "src/handler.go",
			Line:   42,
			OldDoc: "// ProcessRequest uses oldHelper",
			NewDoc: "",
			Symbol: "ProcessRequest",
			Reason: "outdated_reference",
		},
		{
			File:   "src/data.go",
			Line:   8,
			OldDoc: "// Transform transforms data",
			NewDoc: "// Transform transforms data with limit",
			Symbol: "Transform",
			Reason: "new_params",
		},
	}

	result := du.FormatUpdates(updates)

	if !strings.Contains(result, "Stale Documentation (3 items):") {
		t.Error("expected header with count")
	}
	if !strings.Contains(result, "src/auth.go:15 — ValidateToken") {
		t.Error("expected first entry")
	}
	if !strings.Contains(result, "signature_changed (added ctx parameter)") {
		t.Error("expected reason for first entry")
	}
	if !strings.Contains(result, "src/handler.go:42 — ProcessRequest") {
		t.Error("expected second entry")
	}
	if !strings.Contains(result, "outdated_reference") {
		t.Error("expected reason for second entry")
	}
	if !strings.Contains(result, "src/data.go:8 — Transform") {
		t.Error("expected third entry")
	}
	if !strings.Contains(result, `"// ValidateToken validates a JWT token"`) {
		t.Error("expected old doc quoted")
	}
	if !strings.Contains(result, `"// ValidateToken validates a JWT token using the provided context"`) {
		t.Error("expected new doc quoted")
	}
}

func TestFormatUpdates_SingleItem(t *testing.T) {
	du := NewDocUpdater()

	updates := []DocUpdate{
		{
			File:   "main.go",
			Line:   5,
			OldDoc: "// Run runs",
			NewDoc: "// Run runs with options",
			Symbol: "Run",
			Reason: "new_params",
		},
	}

	result := du.FormatUpdates(updates)
	if !strings.Contains(result, "Stale Documentation (1 items):") {
		t.Error("expected header with count 1")
	}
	if !strings.Contains(result, "main.go:5 — Run") {
		t.Error("expected entry")
	}
}

func TestApplyUpdates(t *testing.T) {
	du := NewDocUpdater()

	content := `package main

// ValidateToken validates a JWT token
func ValidateToken(ctx context.Context, token string) bool {
	return true
}

// ProcessData handles data
func ProcessData(data []byte) error {
	return nil
}
`

	updates := []DocUpdate{
		{
			File:   "main.go",
			Line:   4,
			OldDoc: "// ValidateToken validates a JWT token",
			NewDoc: "// ValidateToken validates a JWT token using the provided context",
			Symbol: "ValidateToken",
			Reason: "signature_changed",
		},
	}

	result := du.ApplyUpdates(updates, content)

	if !strings.Contains(result, "// ValidateToken validates a JWT token using the provided context") {
		t.Error("expected updated doc to be applied")
	}
	if strings.Contains(result, "// ValidateToken validates a JWT token\n") {
		t.Error("old doc should have been replaced")
	}
	// Other docs should be untouched
	if !strings.Contains(result, "// ProcessData handles data") {
		t.Error("unrelated docs should not be modified")
	}
}

func TestApplyUpdates_MultipleUpdates(t *testing.T) {
	du := NewDocUpdater()

	content := `package main

// Foo does foo
func Foo(a int) {}

// Bar does bar
func Bar(b int) {}
`

	updates := []DocUpdate{
		{
			File:   "main.go",
			Line:   4,
			OldDoc: "// Foo does foo",
			NewDoc: "// Foo does foo with options",
			Symbol: "Foo",
			Reason: "new_params",
		},
		{
			File:   "main.go",
			Line:   7,
			OldDoc: "// Bar does bar",
			NewDoc: "// Bar does bar with context",
			Symbol: "Bar",
			Reason: "new_params",
		},
	}

	result := du.ApplyUpdates(updates, content)

	if !strings.Contains(result, "// Foo does foo with options") {
		t.Error("expected Foo doc to be updated")
	}
	if !strings.Contains(result, "// Bar does bar with context") {
		t.Error("expected Bar doc to be updated")
	}
}

func TestApplyUpdates_EmptyUpdates(t *testing.T) {
	du := NewDocUpdater()

	content := `package main

// Hello says hello
func Hello() {}
`

	result := du.ApplyUpdates(nil, content)
	if result != content {
		t.Error("content should be unchanged with no updates")
	}
}

func TestApplyUpdates_SkipsEmptyDoc(t *testing.T) {
	du := NewDocUpdater()

	content := `package main

// Hello says hello
func Hello() {}
`

	updates := []DocUpdate{
		{
			File:   "main.go",
			Line:   4,
			OldDoc: "",
			NewDoc: "// Hello says hello world",
			Symbol: "Hello",
			Reason: "new_params",
		},
	}

	result := du.ApplyUpdates(updates, content)
	// Should not modify since OldDoc is empty (can't find what to replace)
	if !strings.Contains(result, "// Hello says hello") {
		t.Error("should not modify when OldDoc is empty")
	}
}

func TestDocUpdParseFunctions(t *testing.T) {
	content := `package main

// Add adds two numbers
func Add(a, b int) int {
	return a + b
}

// Greet greets the user
func (s *Server) Greet(name string) string {
	return "hello " + name
}

func noDoc() {}
`

	funcs := docUpdParseFunctions(content)

	if _, ok := funcs["Add"]; !ok {
		t.Error("expected to find Add function")
	}
	if funcs["Add"].Doc != "// Add adds two numbers" {
		t.Errorf("unexpected doc for Add: %q", funcs["Add"].Doc)
	}

	if _, ok := funcs["Greet"]; !ok {
		t.Error("expected to find Greet function (method)")
	}
	if funcs["Greet"].Doc != "// Greet greets the user" {
		t.Errorf("unexpected doc for Greet: %q", funcs["Greet"].Doc)
	}

	if _, ok := funcs["noDoc"]; !ok {
		t.Error("expected to find noDoc function")
	}
	if funcs["noDoc"].Doc != "" {
		t.Errorf("expected empty doc for noDoc, got %q", funcs["noDoc"].Doc)
	}
}

func TestDocUpdExtractParams(t *testing.T) {
	tests := []struct {
		sig    string
		want   int
		params []string
	}{
		{"(a int, b string) error", 2, []string{"a int", "b string"}},
		{"() error", 0, nil},
		{"(ctx context.Context) error", 1, []string{"ctx context.Context"}},
		{"(data []byte, opts ...Option) error", 2, []string{"data []byte", "opts ...Option"}},
	}

	for _, tt := range tests {
		params := docUpdExtractParams(tt.sig)
		if len(params) != tt.want {
			t.Errorf("docUpdExtractParams(%q): got %d params, want %d: %v", tt.sig, len(params), tt.want, params)
		}
		if tt.params != nil {
			for i, p := range tt.params {
				if i < len(params) && params[i] != p {
					t.Errorf("docUpdExtractParams(%q)[%d]: got %q, want %q", tt.sig, i, params[i], p)
				}
			}
		}
	}
}

func TestDocUpdDetectSignatureChangeDetail(t *testing.T) {
	detail := docUpdDetectSignatureChangeDetail(
		"(token string) bool",
		"(ctx context.Context, token string) bool",
	)
	if !strings.Contains(detail, "added ctx parameter") {
		t.Errorf("expected 'added ctx parameter', got %q", detail)
	}

	detail = docUpdDetectSignatureChangeDetail(
		"(ctx context.Context, token string) bool",
		"(token string) bool",
	)
	if !strings.Contains(detail, "removed ctx parameter") {
		t.Errorf("expected 'removed ctx parameter', got %q", detail)
	}
}

func TestConcurrentAccess(t *testing.T) {
	du := NewDocUpdater()

	oldContent := `package main

// Work does work
func Work(a int) {}
`
	newContent := `package main

// Work does work
func Work(a int, b int) {}
`

	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func() {
			_ = du.DetectStaleDocumentation("file.go", oldContent, newContent)
			done <- true
		}()
	}

	for i := 0; i < 10; i++ {
		<-done
	}
}
