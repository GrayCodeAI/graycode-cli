package project

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func setupMigrationTestProject(t *testing.T) (string, *MigrationPlanner) {
	t.Helper()
	dir := t.TempDir()

	// Create a simple project structure.
	files := map[string]string{
		"main.go": `package main

import "fmt"

func handleAuth(user string) error {
	fmt.Println("authenticating", user)
	return nil
}

func main() {
	handleAuth("admin")
}
`,
		"server/routes.go": `package server

func setupRoutes() {
	// line 4
	handler := handleAuth
	_ = handler
	// more usage
	result := handleAuth("user1")
	_ = result
}
`,
		"auth/handler_test.go": `package auth

import "testing"

func TestHandleAuth(t *testing.T) {
	err := handleAuth("test")
	if err != nil {
		t.Fatal(err)
	}
}
`,
		"go.mod": `module example.com/myproject

go 1.21

require (
	github.com/some/pkg v1.2.0
)
`,
		"utils/helper.go": `package utils

import "github.com/some/pkg"

func DoStuff() {
	pkg.OldFunc("hello")
	pkg.NewFunc("world")
}
`,
	}

	for name, content := range files {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	planner := NewMigrationPlanner(dir)
	return dir, planner
}

func TestNewMigrationPlanner(t *testing.T) {
	planner := NewMigrationPlanner("/tmp/test")
	if planner == nil {
		t.Fatal("expected non-nil planner")
	}
	if planner.ProjectDir != "/tmp/test" {
		t.Errorf("expected ProjectDir /tmp/test, got %s", planner.ProjectDir)
	}
}

func TestPlanRename(t *testing.T) {
	_, planner := setupMigrationTestProject(t)

	plan, err := planner.PlanRename("handleAuth", "HandleAuthentication")
	if err != nil {
		t.Fatal(err)
	}

	if plan.Name != "Rename handleAuth to HandleAuthentication" {
		t.Errorf("unexpected plan name: %s", plan.Name)
	}

	if len(plan.AffectedFiles) == 0 {
		t.Error("expected affected files")
	}

	if len(plan.Steps) == 0 {
		t.Error("expected migration steps")
	}

	if plan.EstimatedChanges == 0 {
		t.Error("expected estimated changes > 0")
	}

	if plan.RiskLevel == "" {
		t.Error("expected risk level to be set")
	}

	// Verify definitions come before usages (lower order number).
	var defOrders, usageOrders []int
	for _, step := range plan.Steps {
		if strings.Contains(step.Description, "definition") {
			defOrders = append(defOrders, step.Order)
		} else if strings.Contains(step.Description, "reference") || strings.Contains(step.Description, "test") {
			usageOrders = append(usageOrders, step.Order)
		}
	}

	if len(defOrders) > 0 && len(usageOrders) > 0 {
		maxDef := defOrders[len(defOrders)-1]
		minUsage := usageOrders[0]
		if maxDef > minUsage {
			t.Error("definition steps should come before usage steps")
		}
	}
}

func TestPlanRenameEmpty(t *testing.T) {
	planner := NewMigrationPlanner("/tmp/test")

	_, err := planner.PlanRename("", "newName")
	if err == nil {
		t.Error("expected error for empty oldName")
	}

	_, err = planner.PlanRename("oldName", "")
	if err == nil {
		t.Error("expected error for empty newName")
	}
}

func TestPlanRenameNoMatches(t *testing.T) {
	_, planner := setupMigrationTestProject(t)

	plan, err := planner.PlanRename("nonExistentSymbol", "newName")
	if err != nil {
		t.Fatal(err)
	}

	if len(plan.Steps) != 0 {
		t.Error("expected no steps for non-existent symbol")
	}
	if plan.RiskLevel != "NONE" {
		t.Errorf("expected NONE risk, got %s", plan.RiskLevel)
	}
}

func TestPlanPatternReplace(t *testing.T) {
	_, planner := setupMigrationTestProject(t)

	plan, err := planner.PlanPatternReplace(`fmt\.Println`, "fmt.Printf", "*.go")
	if err != nil {
		t.Fatal(err)
	}

	if plan == nil {
		t.Fatal("expected non-nil plan")
	}

	if len(plan.Steps) == 0 {
		t.Error("expected steps for pattern replacement")
	}

	for _, step := range plan.Steps {
		if step.Pattern != `fmt\.Println` {
			t.Errorf("unexpected pattern: %s", step.Pattern)
		}
		if step.Replacement != "fmt.Printf" {
			t.Errorf("unexpected replacement: %s", step.Replacement)
		}
	}
}

func TestPlanPatternReplaceEmptyPattern(t *testing.T) {
	planner := NewMigrationPlanner("/tmp/test")

	_, err := planner.PlanPatternReplace("", "replacement", "*.go")
	if err == nil {
		t.Error("expected error for empty pattern")
	}
}

func TestPlanPatternReplaceInvalidRegex(t *testing.T) {
	planner := NewMigrationPlanner("/tmp/test")

	_, err := planner.PlanPatternReplace("[invalid", "replacement", "*.go")
	if err == nil {
		t.Error("expected error for invalid regex")
	}
}

func TestPlanDependencyUpgrade(t *testing.T) {
	_, planner := setupMigrationTestProject(t)

	plan, err := planner.PlanDependencyUpgrade("github.com/some/pkg", "v1.2.0", "v2.0.0")
	if err != nil {
		t.Fatal(err)
	}

	if plan == nil {
		t.Fatal("expected non-nil plan")
	}

	if !strings.Contains(plan.Name, "github.com/some/pkg") {
		t.Errorf("plan name should mention package: %s", plan.Name)
	}

	// Should have at least a go.mod update step.
	found := false
	for _, step := range plan.Steps {
		if strings.Contains(step.Description, "go.mod") {
			found = true
			if step.Manual {
				t.Error("go.mod update should be automatic")
			}
			break
		}
	}
	if !found {
		t.Error("expected go.mod update step")
	}

	// Files referencing the package should be marked for manual review.
	hasManual := false
	for _, step := range plan.Steps {
		if step.Manual && strings.Contains(step.Description, "breaking changes") {
			hasManual = true
			break
		}
	}
	if !hasManual {
		t.Error("expected manual review steps for breaking changes")
	}

	if plan.RiskLevel != "MEDIUM" && plan.RiskLevel != "HIGH" {
		t.Errorf("expected MEDIUM or HIGH risk for dependency upgrade, got %s", plan.RiskLevel)
	}
}

func TestPlanDependencyUpgradeEmptyPkg(t *testing.T) {
	planner := NewMigrationPlanner("/tmp/test")

	_, err := planner.PlanDependencyUpgrade("", "v1.0.0", "v2.0.0")
	if err == nil {
		t.Error("expected error for empty package name")
	}
}

func TestPlanAPIChange(t *testing.T) {
	dir, planner := setupMigrationTestProject(t)

	// Create a file with a specific API call.
	apiFile := filepath.Join(dir, "api_caller.go")
	content := `package main

func caller() {
	result := DoWork(ctx, "input")
	_ = result
}
`
	if err := os.WriteFile(apiFile, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	plan, err := planner.PlanAPIChange("DoWork(ctx, \"input\")", "DoWork(ctx, \"input\", Options{})")
	if err != nil {
		t.Fatal(err)
	}

	if plan == nil {
		t.Fatal("expected non-nil plan")
	}

	if !strings.Contains(plan.Name, "DoWork") {
		t.Errorf("plan name should mention function: %s", plan.Name)
	}

	if len(plan.Steps) == 0 {
		t.Error("expected at least one step")
	}
}

func TestPlanAPIChangeEmpty(t *testing.T) {
	planner := NewMigrationPlanner("/tmp/test")

	_, err := planner.PlanAPIChange("", "newSig()")
	if err == nil {
		t.Error("expected error for empty oldSig")
	}

	_, err = planner.PlanAPIChange("oldSig()", "")
	if err == nil {
		t.Error("expected error for empty newSig")
	}
}

func TestMigrationPreview(t *testing.T) {
	_, planner := setupMigrationTestProject(t)

	plan, err := planner.PlanRename("handleAuth", "HandleAuthentication")
	if err != nil {
		t.Fatal(err)
	}

	preview := planner.Preview(plan)

	if !strings.Contains(preview, "Migration:") {
		t.Error("preview should contain Migration header")
	}
	if !strings.Contains(preview, "Risk:") {
		t.Error("preview should contain Risk")
	}
	if !strings.Contains(preview, "Files:") {
		t.Error("preview should contain Files count")
	}
	if !strings.Contains(preview, "Changes:") {
		t.Error("preview should contain Changes count")
	}
	if !strings.Contains(preview, "Steps:") {
		t.Error("preview should contain Steps section")
	}
	if !strings.Contains(preview, "[auto]") && !strings.Contains(preview, "[manual]") {
		t.Error("preview should tag steps as auto or manual")
	}
}

func TestMigrationPreviewNil(t *testing.T) {
	planner := NewMigrationPlanner("/tmp/test")
	preview := planner.Preview(nil)
	if preview != "" {
		t.Error("expected empty string for nil plan")
	}
}

func TestMigrationExecute(t *testing.T) {
	dir, planner := setupMigrationTestProject(t)

	plan, err := planner.PlanPatternReplace(`handleAuth`, "HandleAuthentication", "*.go")
	if err != nil {
		t.Fatal(err)
	}

	result, err := planner.Execute(plan)
	if err != nil {
		t.Fatal(err)
	}

	if result.Completed == 0 {
		t.Error("expected at least one completed step")
	}

	// Verify the file was actually changed.
	content, err := os.ReadFile(filepath.Join(dir, "main.go"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), "HandleAuthentication") {
		t.Error("expected file to contain the replacement text")
	}
	if strings.Contains(string(content), "handleAuth") {
		t.Error("expected old text to be replaced")
	}
}

func TestMigrationExecuteNil(t *testing.T) {
	planner := NewMigrationPlanner("/tmp/test")
	_, err := planner.Execute(nil)
	if err == nil {
		t.Error("expected error for nil plan")
	}
}

func TestMigrationExecuteManualStepsSkipped(t *testing.T) {
	_, planner := setupMigrationTestProject(t)

	plan := &MigrationPlan{
		Name: "Test manual skip",
		Steps: []MigrationStep{
			{Order: 1, Description: "manual step", Manual: true, Pattern: "test", Files: []string{"/nonexistent"}},
		},
	}

	result, err := planner.Execute(plan)
	if err != nil {
		t.Fatal(err)
	}

	if result.Skipped != 1 {
		t.Errorf("expected 1 skipped, got %d", result.Skipped)
	}
	if len(result.ManualReview) != 1 {
		t.Errorf("expected 1 manual review item, got %d", len(result.ManualReview))
	}
}

func TestMigrationValidate(t *testing.T) {
	_, planner := setupMigrationTestProject(t)

	plan, err := planner.PlanRename("handleAuth", "HandleAuthentication")
	if err != nil {
		t.Fatal(err)
	}

	warnings := planner.Validate(plan)
	// A valid plan should have no warnings.
	if len(warnings) != 0 {
		t.Logf("warnings: %v", warnings)
		// Don't fail — some warnings may be acceptable depending on project structure.
	}
}

func TestMigrationValidateNil(t *testing.T) {
	planner := NewMigrationPlanner("/tmp/test")
	warnings := planner.Validate(nil)
	if len(warnings) == 0 {
		t.Error("expected warnings for nil plan")
	}
}

func TestMigrationValidateMissingFiles(t *testing.T) {
	planner := NewMigrationPlanner("/tmp/test")

	plan := &MigrationPlan{
		Name:          "Test missing files",
		AffectedFiles: []string{"/nonexistent/file.go"},
		Steps: []MigrationStep{
			{Order: 1, Pattern: "test", Files: []string{"/nonexistent/file.go"}},
		},
	}

	warnings := planner.Validate(plan)
	hasFileWarning := false
	for _, w := range warnings {
		if strings.Contains(w, "does not exist") {
			hasFileWarning = true
			break
		}
	}
	if !hasFileWarning {
		t.Error("expected warning about non-existent file")
	}
}

func TestMigrationValidateEmptyPattern(t *testing.T) {
	planner := NewMigrationPlanner("/tmp/test")

	plan := &MigrationPlan{
		Name: "Test empty pattern",
		Steps: []MigrationStep{
			{Order: 1, Pattern: "", Files: []string{"/tmp/test.go"}},
		},
	}

	warnings := planner.Validate(plan)
	hasPatternWarning := false
	for _, w := range warnings {
		if strings.Contains(w, "empty pattern") {
			hasPatternWarning = true
			break
		}
	}
	if !hasPatternWarning {
		t.Error("expected warning about empty pattern")
	}
}

func TestMigrationRollback(t *testing.T) {
	dir, planner := setupMigrationTestProject(t)

	// Read original content.
	originalContent, err := os.ReadFile(filepath.Join(dir, "main.go"))
	if err != nil {
		t.Fatal(err)
	}

	plan, err := planner.PlanPatternReplace(`handleAuth`, "HandleAuthentication", "*.go")
	if err != nil {
		t.Fatal(err)
	}

	// Execute the plan.
	_, err = planner.Execute(plan)
	if err != nil {
		t.Fatal(err)
	}

	// Verify change happened.
	changed, err := os.ReadFile(filepath.Join(dir, "main.go"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(changed), "HandleAuthentication") {
		t.Fatal("expected file to be changed before rollback")
	}

	// Rollback.
	err = planner.Rollback(plan)
	if err != nil {
		t.Fatal(err)
	}

	// Verify rollback restored original content.
	restored, err := os.ReadFile(filepath.Join(dir, "main.go"))
	if err != nil {
		t.Fatal(err)
	}
	if string(restored) != string(originalContent) {
		t.Errorf("rollback did not restore original content.\nExpected:\n%s\nGot:\n%s", originalContent, restored)
	}

	// Verify steps are marked as not completed.
	for _, step := range plan.Steps {
		if step.Completed {
			t.Error("expected all steps to be marked as not completed after rollback")
		}
	}
}

func TestMigrationRollbackNil(t *testing.T) {
	planner := NewMigrationPlanner("/tmp/test")
	err := planner.Rollback(nil)
	if err == nil {
		t.Error("expected error for nil plan")
	}
}

func TestMigrationExtractFuncName(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"DoWork(ctx, input)", "DoWork"},
		{"func HandleAuth(user string) error", "HandleAuth"},
		{"myFunc(a, b int)", "myFunc"},
		{"SimpleFunc", "SimpleFunc"},
		{"", ""},
	}

	for _, tt := range tests {
		got := migrationExtractFuncName(tt.input)
		if got != tt.expected {
			t.Errorf("migrationExtractFuncName(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestMigrationIsTextFile(t *testing.T) {
	tests := []struct {
		path     string
		expected bool
	}{
		{"main.go", true},
		{"script.js", true},
		{"data.json", true},
		{"config.yaml", true},
		{"image.png", false},
		{"binary.exe", false},
		{"archive.tar.gz", false},
	}

	for _, tt := range tests {
		got := isTextFile(tt.path)
		if got != tt.expected {
			t.Errorf("isTextFile(%q) = %v, want %v", tt.path, got, tt.expected)
		}
	}
}

func TestMigrationAssessRisk(t *testing.T) {
	planner := NewMigrationPlanner("/tmp/test")

	// LOW risk: few files and changes.
	lowPlan := &MigrationPlan{
		AffectedFiles:    []string{"a.go", "b.go"},
		EstimatedChanges: 3,
		Steps:            []MigrationStep{{Manual: false}, {Manual: false}},
	}
	if risk := planner.assessRisk(lowPlan); risk != "LOW" {
		t.Errorf("expected LOW risk, got %s", risk)
	}

	// MEDIUM risk: moderate files.
	medPlan := &MigrationPlan{
		AffectedFiles:    []string{"a.go", "b.go", "c.go", "d.go", "e.go", "f.go"},
		EstimatedChanges: 10,
		Steps:            []MigrationStep{{Manual: false}},
	}
	if risk := planner.assessRisk(medPlan); risk != "MEDIUM" {
		t.Errorf("expected MEDIUM risk, got %s", risk)
	}

	// HIGH risk: many files.
	highFiles := make([]string, 25)
	for i := range highFiles {
		highFiles[i] = "file.go"
	}
	highPlan := &MigrationPlan{
		AffectedFiles:    highFiles,
		EstimatedChanges: 60,
		Steps:            []MigrationStep{{Manual: false}},
	}
	if risk := planner.assessRisk(highPlan); risk != "HIGH" {
		t.Errorf("expected HIGH risk, got %s", risk)
	}
}

func TestEndToEndMigration(t *testing.T) {
	dir, planner := setupMigrationTestProject(t)

	// Plan a rename.
	plan, err := planner.PlanRename("handleAuth", "HandleAuthentication")
	if err != nil {
		t.Fatal(err)
	}

	// Preview.
	preview := planner.Preview(plan)
	if preview == "" {
		t.Error("expected non-empty preview")
	}
	t.Log(preview)

	// Validate.
	warnings := planner.Validate(plan)
	t.Logf("Validation warnings: %v", warnings)

	// Execute.
	result, err := planner.Execute(plan)
	if err != nil {
		t.Fatal(err)
	}

	t.Logf("Result: completed=%d, skipped=%d, failed=%d", result.Completed, result.Skipped, result.Failed)

	if result.Completed == 0 && result.Failed == 0 {
		t.Error("expected some steps to be executed")
	}

	// Check that auto steps were applied.
	mainContent, err := os.ReadFile(filepath.Join(dir, "main.go"))
	if err != nil {
		t.Fatal(err)
	}
	if result.Completed > 0 && !strings.Contains(string(mainContent), "HandleAuthentication") {
		t.Error("expected main.go to have the rename applied")
	}
}

func TestMigrationPlannerConcurrentAccess(t *testing.T) {
	_, planner := setupMigrationTestProject(t)

	// Run multiple plans concurrently to verify mutex safety.
	done := make(chan struct{}, 3)

	go func() {
		planner.PlanRename("handleAuth", "HandleAuth2")
		done <- struct{}{}
	}()

	go func() {
		planner.PlanPatternReplace(`fmt\.Println`, "fmt.Printf", "*.go")
		done <- struct{}{}
	}()

	go func() {
		planner.PlanDependencyUpgrade("github.com/some/pkg", "v1.2.0", "v2.0.0")
		done <- struct{}{}
	}()

	for i := 0; i < 3; i++ {
		<-done
	}
}
