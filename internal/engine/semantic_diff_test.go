package engine

import (
	"go/ast"
	"strings"
	"testing"
)

func TestNewSemanticAnalyzer(t *testing.T) {
	sa := NewSemanticAnalyzer()
	if sa == nil {
		t.Fatal("NewSemanticAnalyzer returned nil")
	}
	if len(sa.routePatterns) == 0 {
		t.Error("expected route patterns to be initialized")
	}
}

func TestAnalyzeDiffEmpty(t *testing.T) {
	sa := NewSemanticAnalyzer()
	result, err := sa.AnalyzeDiff("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.RiskLevel != "low" {
		t.Errorf("expected risk level low for empty diff, got %s", result.RiskLevel)
	}
	if len(result.Changes) != 0 {
		t.Errorf("expected no changes for empty diff, got %d", len(result.Changes))
	}
}

func TestAnalyzeDiffFunctionAdded(t *testing.T) {
	diff := `--- a/handler.go
+++ b/handler.go
@@ -1,5 +1,12 @@
 package main

 import "fmt"

 func Existing() {}
+
+func ValidateJWT(token string) (*Claims, error) {
+	if token == "" {
+		return nil, fmt.Errorf("empty token")
+	}
+	return parseClaims(token)
+}
`
	sa := NewSemanticAnalyzer()
	result, err := sa.AnalyzeDiff(diff)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	found := false
	for _, c := range result.Changes {
		if c.Type == "function_added" && c.Name == "ValidateJWT" {
			found = true
			if c.Breaking {
				t.Error("added function should not be breaking")
			}
			break
		}
	}
	if !found {
		t.Error("expected to find function_added change for ValidateJWT")
	}
}

func TestAnalyzeDiffFunctionRemoved(t *testing.T) {
	diff := `--- a/handler.go
+++ b/handler.go
@@ -1,8 +1,5 @@
 package main

 import "fmt"

-func OldHandler(w http.ResponseWriter, r *http.Request) {
-	fmt.Fprintf(w, "hello")
-}
 func Remaining() {}
`
	sa := NewSemanticAnalyzer()
	result, err := sa.AnalyzeDiff(diff)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	found := false
	for _, c := range result.Changes {
		if c.Type == "function_removed" && c.Name == "OldHandler" {
			found = true
			if !c.Breaking {
				t.Error("removed exported function should be breaking")
			}
			break
		}
	}
	if !found {
		t.Error("expected to find function_removed change for OldHandler")
	}

	if result.RiskLevel != "high" {
		t.Errorf("expected high risk for breaking change, got %s", result.RiskLevel)
	}
}

func TestAnalyzeDiffSignatureChanged(t *testing.T) {
	diff := `--- a/auth.go
+++ b/auth.go
@@ -1,7 +1,7 @@
 package auth

 import "context"

-func Authenticate(username, password string) (bool, error) {
+func Authenticate(ctx context.Context, username, password string) (bool, error) {
 	return true, nil
 }
`
	sa := NewSemanticAnalyzer()
	result, err := sa.AnalyzeDiff(diff)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	found := false
	for _, c := range result.Changes {
		if c.Type == "signature_changed" && strings.Contains(c.Name, "Authenticate") {
			found = true
			if !c.Breaking {
				t.Error("signature change on exported function should be breaking")
			}
			break
		}
	}
	if !found {
		t.Error("expected to find signature_changed for Authenticate")
	}
}

func TestDetectBreakingChangesExportedRemoved(t *testing.T) {
	oldContent := `package main

func ProcessRequest(data []byte) error {
	return nil
}

func helperFunc() {}
`
	newContent := `package main

func helperFunc() {}
`

	changes := DetectBreakingChanges(oldContent, newContent)

	found := false
	for _, c := range changes {
		if c.Name == "ProcessRequest" && c.Breaking {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected ProcessRequest removal to be detected as breaking")
	}
}

func TestDetectBreakingChangesSignatureChanged(t *testing.T) {
	oldContent := `package main

func Validate(input string) bool {
	return input != ""
}
`
	newContent := `package main

func Validate(input string, strict bool) (bool, error) {
	return input != "", nil
}
`

	changes := DetectBreakingChanges(oldContent, newContent)

	found := false
	for _, c := range changes {
		if c.Name == "Validate" && c.Type == "signature_changed" && c.Breaking {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected Validate signature change to be detected as breaking")
	}
}

func TestDetectBreakingChangesTypeChanged(t *testing.T) {
	oldContent := `package main

type Config struct {
	Host string
	Port int
}
`
	newContent := `package main

type Config struct {
	Host    string
	Port    int
	Timeout int
}
`

	changes := DetectBreakingChanges(oldContent, newContent)

	found := false
	for _, c := range changes {
		if c.Name == "Config" && c.Type == "type_changed" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected Config type change to be detected")
	}
}

func TestDetectBreakingChangesInterfaceMethodAdded(t *testing.T) {
	oldContent := `package main

type Handler interface {
	Handle(req Request) Response
}
`
	newContent := `package main

type Handler interface {
	Handle(req Request) Response
	Close() error
}
`

	changes := DetectBreakingChanges(oldContent, newContent)

	found := false
	for _, c := range changes {
		if strings.Contains(c.Name, "Close") && c.Breaking {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected interface method addition to be detected as breaking")
	}
}

func TestDetectBehaviorChangesErrorHandling(t *testing.T) {
	oldContent := `package main

func Process(data []byte) error {
	result := transform(data)
	save(result)
	return nil
}
`
	newContent := `package main

func Process(data []byte) error {
	result := transform(data)
	if err != nil {
		return err
	}
	save(result)
	return nil
}
`

	changes := DetectBehaviorChanges(oldContent, newContent)

	found := false
	for _, c := range changes {
		if c.Type == "behavior_changed" && strings.Contains(c.Description, "Error handling added") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected error handling addition to be detected")
	}
}

func TestDetectBehaviorChangesNilChecks(t *testing.T) {
	oldContent := `package main

func GetUser(id string) *User {
	user := findUser(id)
	return user
}
`
	newContent := `package main

func GetUser(id string) *User {
	user := findUser(id)
	if user == nil {
		return nil
	}
	return user
}
`

	changes := DetectBehaviorChanges(oldContent, newContent)

	found := false
	for _, c := range changes {
		if c.Type == "behavior_changed" && strings.Contains(c.Description, "Nil checks") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected nil check addition to be detected")
	}
}

func TestDetectBehaviorChangesLoopBounds(t *testing.T) {
	oldContent := `package main

func ProcessItems(items []Item) {
	for i := 0; i < len(items); i++ {
		handle(items[i])
	}
}
`
	newContent := `package main

func ProcessItems(items []Item) {
	for i := 0; i < len(items)-1; i++ {
		handle(items[i])
	}
}
`

	changes := DetectBehaviorChanges(oldContent, newContent)

	found := false
	for _, c := range changes {
		if c.Type == "behavior_changed" && strings.Contains(c.Description, "Loop bounds") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected loop bounds change to be detected")
	}
}

func TestClassifyRiskLow(t *testing.T) {
	changes := []SemanticChange{
		{Type: "function_added", Breaking: false},
		{Type: "import_added", Breaking: false},
	}
	risk := ClassifyRisk(changes)
	if risk != "low" {
		t.Errorf("expected low risk, got %s", risk)
	}
}

func TestClassifyRiskMedium(t *testing.T) {
	changes := []SemanticChange{
		{Type: "function_modified", Breaking: false},
		{Type: "behavior_changed", Breaking: false},
	}
	risk := ClassifyRisk(changes)
	if risk != "medium" {
		t.Errorf("expected medium risk, got %s", risk)
	}
}

func TestClassifyRiskHigh(t *testing.T) {
	changes := []SemanticChange{
		{Type: "function_removed", Breaking: true},
		{Type: "function_added", Breaking: false},
	}
	risk := ClassifyRisk(changes)
	if risk != "high" {
		t.Errorf("expected high risk, got %s", risk)
	}
}

func TestGenerateSummary(t *testing.T) {
	diff := &SemanticDiff{
		Changes: []SemanticChange{
			{Type: "function_added", Name: "ValidateJWT", Description: "Function added: func ValidateJWT(token string) (*Claims, error)", Breaking: false},
			{Type: "function_removed", Name: "OldHandler", Description: "Exported function OldHandler removed", Breaking: true},
			{Type: "behavior_changed", Name: "HandleRequest", Description: "Error handling added in HandleRequest (0 -> 2 checks)", Breaking: false},
		},
		RiskLevel:    "high",
		AffectedAPIs: []string{"/api/auth", "/api/users"},
	}

	summary := GenerateSummary(diff)

	if !strings.Contains(summary, "Semantic Analysis:") {
		t.Error("summary missing header")
	}
	if !strings.Contains(summary, "Risk: HIGH") {
		t.Error("summary missing risk level")
	}
	if !strings.Contains(summary, "Added:") {
		t.Error("summary missing added entry")
	}
	if !strings.Contains(summary, "Breaking:") {
		t.Error("summary missing breaking entry")
	}
	if !strings.Contains(summary, "Modified:") {
		t.Error("summary missing modified entry")
	}
	if !strings.Contains(summary, "/api/auth") {
		t.Error("summary missing affected API")
	}
	if !strings.Contains(summary, "Impact:") {
		t.Error("summary missing impact line")
	}
}

func TestFindAffectedAPIs(t *testing.T) {
	sa := NewSemanticAnalyzer()
	changes := []SemanticChange{
		{Name: "HandleAuth"},
		{Name: "GetUser"},
	}
	content := `
http.HandleFunc("/api/auth", HandleAuth)
http.HandleFunc("/api/users", GetUser)
http.HandleFunc("/api/health", HealthCheck)
`
	apis := sa.FindAffectedAPIs(changes, content)

	foundAuth := false
	foundUsers := false
	for _, api := range apis {
		if api == "/api/auth" {
			foundAuth = true
		}
		if api == "/api/users" {
			foundUsers = true
		}
	}
	if !foundAuth {
		t.Error("expected /api/auth to be found in affected APIs")
	}
	if !foundUsers {
		t.Error("expected /api/users to be found in affected APIs")
	}
}

func TestCompareSignaturesParamsAdded(t *testing.T) {
	old := "func Process(data []byte) error"
	new := "func Process(ctx context.Context, data []byte) error"

	change := CompareSignatures(old, new)
	if change == nil {
		t.Fatal("expected non-nil SignatureChange")
	}
	if change.Name != "Process" {
		t.Errorf("expected name Process, got %s", change.Name)
	}
	if len(change.ParamsAdded) == 0 {
		t.Error("expected at least one added param")
	}
}

func TestCompareSignaturesReturnChanged(t *testing.T) {
	old := "func Validate(input string) bool"
	new := "func Validate(input string) (bool, error)"

	change := CompareSignatures(old, new)
	if change == nil {
		t.Fatal("expected non-nil SignatureChange")
	}
	if !change.ReturnChanged {
		t.Error("expected return type to be marked as changed")
	}
	if change.OldReturn != "bool" {
		t.Errorf("expected old return 'bool', got '%s'", change.OldReturn)
	}
}

func TestCompareSignaturesReceiverChanged(t *testing.T) {
	old := "func (s *Server) Handle(r Request) Response"
	new := "func (s Server) Handle(r Request) Response"

	change := CompareSignatures(old, new)
	if change == nil {
		t.Fatal("expected non-nil SignatureChange")
	}
	if !change.ReceiverChanged {
		t.Error("expected receiver to be marked as changed")
	}
}

func TestCompareSignaturesIdentical(t *testing.T) {
	sig := "func Process(data []byte) error"
	change := CompareSignatures(sig, sig)
	if change != nil {
		t.Error("expected nil for identical signatures")
	}
}

func TestIsExported(t *testing.T) {
	tests := []struct {
		name     string
		expected bool
	}{
		{"HandleRequest", true},
		{"handleRequest", false},
		{"Server.Handle", true},
		{"server.handle", false},
		{"", false},
	}

	for _, tt := range tests {
		got := isExported(tt.name)
		if got != tt.expected {
			t.Errorf("isExported(%q) = %v, want %v", tt.name, got, tt.expected)
		}
	}
}

func TestDetectImportChanges(t *testing.T) {
	oldContent := `package main

import (
	"fmt"
	"os"
)
`
	newContent := `package main

import (
	"fmt"
	"context"
	"net/http"
)
`

	changes := detectImportChanges(oldContent, newContent)

	var added, removed []string
	for _, c := range changes {
		switch c.Type {
		case "import_added":
			added = append(added, c.Name)
		case "import_removed":
			removed = append(removed, c.Name)
		}
	}

	if len(added) != 2 {
		t.Errorf("expected 2 added imports, got %d: %v", len(added), added)
	}
	if len(removed) != 1 {
		t.Errorf("expected 1 removed import, got %d: %v", len(removed), removed)
	}
}

func TestAnalyzeDiffBehaviorChange(t *testing.T) {
	diff := `--- a/service.go
+++ b/service.go
@@ -1,8 +1,11 @@
 package main

 func Process(data []byte) error {
 	result := transform(data)
+	if err != nil {
+		return err
+	}
 	save(result)
 	return nil
 }
`
	sa := NewSemanticAnalyzer()
	result, err := sa.AnalyzeDiff(diff)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	foundBehavior := false
	for _, c := range result.Changes {
		if c.Type == "behavior_changed" {
			foundBehavior = true
			break
		}
	}
	if !foundBehavior {
		t.Error("expected behavior change to be detected")
	}
}

func TestClassifyRiskEmpty(t *testing.T) {
	risk := ClassifyRisk(nil)
	if risk != "low" {
		t.Errorf("expected low risk for nil changes, got %s", risk)
	}
}

func TestDetectBreakingChangesEmptyOld(t *testing.T) {
	changes := DetectBreakingChanges("", "package main\nfunc New() {}")
	if len(changes) != 0 {
		t.Errorf("expected no breaking changes when old is empty, got %d", len(changes))
	}
}

func TestDetectBehaviorChangesEmptyContent(t *testing.T) {
	changes := DetectBehaviorChanges("", "package main\nfunc X() {}")
	if len(changes) != 0 {
		t.Errorf("expected no behavior changes when old is empty, got %d", len(changes))
	}
}

func TestAnalyzeDiffMultipleFiles(t *testing.T) {
	diff := `--- a/auth.go
+++ b/auth.go
@@ -1,5 +1,8 @@
 package main

 func Login(user string) bool {
 	return true
 }
+
+func Logout(user string) {
+}
--- a/db.go
+++ b/db.go
@@ -1,5 +1,3 @@
 package main

-func OldQuery(sql string) error {
-	return nil
-}
+func NewQuery(ctx context.Context, sql string) error { return nil }
`
	sa := NewSemanticAnalyzer()
	result, err := sa.AnalyzeDiff(diff)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Changes) == 0 {
		t.Fatal("expected changes to be detected across multiple files")
	}

	// Should detect OldQuery removal as breaking
	foundBreaking := false
	for _, c := range result.Changes {
		if c.Breaking && strings.Contains(c.Name, "OldQuery") {
			foundBreaking = true
			break
		}
	}
	if !foundBreaking {
		t.Error("expected OldQuery removal to be detected as breaking")
	}
}

func TestCompareSignaturesParamsReordered(t *testing.T) {
	old := "func Send(to string, from string) error"
	new := "func Send(from string, to string) error"

	change := CompareSignatures(old, new)
	if change == nil {
		t.Fatal("expected non-nil SignatureChange")
	}
	if !change.ParamsReordered {
		t.Error("expected params to be detected as reordered")
	}
}

func TestGenerateSummaryNoAPIs(t *testing.T) {
	diff := &SemanticDiff{
		Changes: []SemanticChange{
			{Type: "function_added", Name: "helper", Description: "added helper", Breaking: false},
		},
		RiskLevel:    "low",
		AffectedAPIs: nil,
	}

	summary := GenerateSummary(diff)
	if strings.Contains(summary, "Affected APIs:") {
		t.Error("should not contain Affected APIs when there are none")
	}
}

// TestFormatNodeNonExprRegression guards H10 — a non-ast.Expr node (e.g. *ast.Comment)
// must not panic; the comma-ok form should fall through to "unknown".
func TestFormatNodeNonExprRegression(t *testing.T) {
	// *ast.Comment is ast.Node but not ast.Expr. Pre-fix this panicked
	// with "interface conversion: *ast.Comment is not ast.Expr".
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("formatNode panicked on non-Expr node: %v", r)
		}
	}()

	got := formatNode(nil, &ast.Comment{Text: "x"})
	if got != "unknown" {
		t.Errorf("formatNode(*ast.Comment) = %q, want %q", got, "unknown")
	}
}
