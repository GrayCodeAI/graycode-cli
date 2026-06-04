package project

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestClassifyUpdate_Major(t *testing.T) {
	tests := []struct {
		current  string
		latest   string
		expected string
	}{
		{"v1.0.0", "v2.0.0", "major"},
		{"v0.9.0", "v1.0.0", "major"},
		{"v3.2.1", "v4.0.0", "major"},
		{"1.0.0", "2.0.0", "major"},
	}

	for _, tt := range tests {
		result := ClassifyUpdate(tt.current, tt.latest)
		if result != tt.expected {
			t.Errorf("ClassifyUpdate(%s, %s) = %s, want %s", tt.current, tt.latest, result, tt.expected)
		}
	}
}

func TestClassifyUpdate_Minor(t *testing.T) {
	tests := []struct {
		current  string
		latest   string
		expected string
	}{
		{"v1.0.0", "v1.1.0", "minor"},
		{"v2.1.0", "v2.3.0", "minor"},
		{"v1.0.0", "v1.5.3", "minor"},
		{"0.1.0", "0.1.0", "minor"},
	}

	for _, tt := range tests {
		result := ClassifyUpdate(tt.current, tt.latest)
		if result != tt.expected {
			t.Errorf("ClassifyUpdate(%s, %s) = %s, want %s", tt.current, tt.latest, result, tt.expected)
		}
	}
}

func TestClassifyUpdate_Patch(t *testing.T) {
	tests := []struct {
		current  string
		latest   string
		expected string
	}{
		{"v1.0.0", "v1.0.1", "patch"},
		{"v2.3.4", "v2.3.5", "patch"},
		{"v1.0.0", "v1.0.10", "patch"},
		{"1.2.3", "1.2.4", "patch"},
	}

	for _, tt := range tests {
		result := ClassifyUpdate(tt.current, tt.latest)
		if result != tt.expected {
			t.Errorf("ClassifyUpdate(%s, %s) = %s, want %s", tt.current, tt.latest, result, tt.expected)
		}
	}
}

func TestClassifyUpdate_InvalidVersionFallsToPatch(t *testing.T) {
	result := ClassifyUpdate("invalid", "alsobad")
	if result != "patch" {
		t.Errorf("ClassifyUpdate with invalid versions = %s, want patch", result)
	}
}

func TestParseSemver_Valid(t *testing.T) {
	tests := []struct {
		version             string
		major, minor, patch int
	}{
		{"v1.2.3", 1, 2, 3},
		{"1.2.3", 1, 2, 3},
		{"v0.0.1", 0, 0, 1},
		{"v10.20.30", 10, 20, 30},
		{"2.0.0", 2, 0, 0},
	}

	for _, tt := range tests {
		major, minor, patch, err := ParseSemver(tt.version)
		if err != nil {
			t.Errorf("ParseSemver(%s) unexpected error: %v", tt.version, err)
			continue
		}
		if major != tt.major || minor != tt.minor || patch != tt.patch {
			t.Errorf("ParseSemver(%s) = %d.%d.%d, want %d.%d.%d",
				tt.version, major, minor, patch, tt.major, tt.minor, tt.patch)
		}
	}
}

func TestParseSemver_WithPreRelease(t *testing.T) {
	major, minor, patch, err := ParseSemver("v1.2.3-beta.1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if major != 1 || minor != 2 || patch != 3 {
		t.Errorf("ParseSemver(v1.2.3-beta.1) = %d.%d.%d, want 1.2.3", major, minor, patch)
	}
}

func TestParseSemver_WithBuildMetadata(t *testing.T) {
	major, minor, patch, err := ParseSemver("v2.0.0+build.123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if major != 2 || minor != 0 || patch != 0 {
		t.Errorf("ParseSemver(v2.0.0+build.123) = %d.%d.%d, want 2.0.0", major, minor, patch)
	}
}

func TestParseSemver_PartialVersion(t *testing.T) {
	major, minor, patch, err := ParseSemver("v1.2")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if major != 1 || minor != 2 || patch != 0 {
		t.Errorf("ParseSemver(v1.2) = %d.%d.%d, want 1.2.0", major, minor, patch)
	}
}

func TestParseSemver_MajorOnly(t *testing.T) {
	major, minor, patch, err := ParseSemver("3")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if major != 3 || minor != 0 || patch != 0 {
		t.Errorf("ParseSemver(3) = %d.%d.%d, want 3.0.0", major, minor, patch)
	}
}

func TestParseSemver_Invalid(t *testing.T) {
	_, _, _, err := ParseSemver("vABC.1.2")
	if err == nil {
		t.Error("ParseSemver(vABC.1.2) expected error, got nil")
	}
}

func TestGeneratePlan_Prioritization(t *testing.T) {
	du := &DependencyUpdater{ProjectDir: "/tmp", Language: "go"}
	deps := []Dependency{
		{Name: "pkg-major", CurrentVersion: "v1.0.0", LatestVersion: "v2.0.0", UpdateType: "major"},
		{Name: "pkg-security", CurrentVersion: "v1.0.0", LatestVersion: "v1.0.1", UpdateType: "patch", SecurityFix: true},
		{Name: "pkg-minor", CurrentVersion: "v1.0.0", LatestVersion: "v1.1.0", UpdateType: "minor"},
		{Name: "pkg-patch", CurrentVersion: "v1.0.0", LatestVersion: "v1.0.2", UpdateType: "patch"},
	}

	plan := du.GeneratePlan(deps)

	if plan.Dependencies[0].Name != "pkg-security" {
		t.Errorf("expected security fix first, got %s", plan.Dependencies[0].Name)
	}
	if plan.Dependencies[1].Name != "pkg-patch" {
		t.Errorf("expected patch second, got %s", plan.Dependencies[1].Name)
	}
	if plan.Dependencies[2].Name != "pkg-minor" {
		t.Errorf("expected minor third, got %s", plan.Dependencies[2].Name)
	}
	if plan.Dependencies[3].Name != "pkg-major" {
		t.Errorf("expected major last, got %s", plan.Dependencies[3].Name)
	}
}

func TestGeneratePlan_RiskLevelHigh(t *testing.T) {
	du := &DependencyUpdater{ProjectDir: "/tmp", Language: "go"}
	deps := []Dependency{
		{Name: "pkg1", UpdateType: "major"},
		{Name: "pkg2", UpdateType: "patch"},
	}

	plan := du.GeneratePlan(deps)
	if plan.RiskLevel != "high" {
		t.Errorf("expected risk level high, got %s", plan.RiskLevel)
	}
	if plan.EstimatedBreaking != 1 {
		t.Errorf("expected 1 estimated breaking, got %d", plan.EstimatedBreaking)
	}
}

func TestGeneratePlan_RiskLevelMedium(t *testing.T) {
	du := &DependencyUpdater{ProjectDir: "/tmp", Language: "go"}
	deps := []Dependency{
		{Name: "pkg1", UpdateType: "minor"},
		{Name: "pkg2", UpdateType: "patch"},
	}

	plan := du.GeneratePlan(deps)
	if plan.RiskLevel != "medium" {
		t.Errorf("expected risk level medium, got %s", plan.RiskLevel)
	}
}

func TestGeneratePlan_RiskLevelLow(t *testing.T) {
	du := &DependencyUpdater{ProjectDir: "/tmp", Language: "go"}
	deps := []Dependency{
		{Name: "pkg1", UpdateType: "patch"},
		{Name: "pkg2", UpdateType: "patch"},
	}

	plan := du.GeneratePlan(deps)
	if plan.RiskLevel != "low" {
		t.Errorf("expected risk level low, got %s", plan.RiskLevel)
	}
}

func TestGeneratePlan_TestCommand(t *testing.T) {
	tests := []struct {
		lang     string
		expected string
	}{
		{"go", "go test ./..."},
		{"javascript", "npm test"},
		{"python", "pytest"},
		{"rust", "cargo test"},
	}

	for _, tt := range tests {
		du := &DependencyUpdater{ProjectDir: "/tmp", Language: tt.lang}
		plan := du.GeneratePlan([]Dependency{{Name: "pkg", UpdateType: "patch"}})
		if plan.TestCommand != tt.expected {
			t.Errorf("Language %s: expected test command %q, got %q", tt.lang, tt.expected, plan.TestCommand)
		}
	}
}

func TestGeneratePlan_Empty(t *testing.T) {
	du := &DependencyUpdater{ProjectDir: "/tmp", Language: "go"}
	plan := du.GeneratePlan([]Dependency{})

	if plan.RiskLevel != "low" {
		t.Errorf("expected low risk for empty plan, got %s", plan.RiskLevel)
	}
	if len(plan.Dependencies) != 0 {
		t.Errorf("expected 0 dependencies, got %d", len(plan.Dependencies))
	}
}

func TestFormatOutdated_WithAllCategories(t *testing.T) {
	deps := []Dependency{
		{Name: "github.com/foo/bar", CurrentVersion: "v1.2.3", LatestVersion: "v1.2.5", UpdateType: "patch", SecurityFix: true},
		{Name: "github.com/x/y", CurrentVersion: "v2.1.0", LatestVersion: "v2.3.0", UpdateType: "minor"},
		{Name: "github.com/a/b", CurrentVersion: "v1.0.0", LatestVersion: "v1.1.0", UpdateType: "minor"},
		{Name: "github.com/big/lib", CurrentVersion: "v3.0.0", LatestVersion: "v4.0.0", UpdateType: "major"},
	}

	output := FormatOutdated(deps)

	if !strings.Contains(output, "Outdated Dependencies:") {
		t.Error("missing header")
	}
	if !strings.Contains(output, "SECURITY:") {
		t.Error("missing SECURITY section")
	}
	if !strings.Contains(output, "MINOR:") {
		t.Error("missing MINOR section")
	}
	if !strings.Contains(output, "MAJOR:") {
		t.Error("missing MAJOR section")
	}
	if !strings.Contains(output, "github.com/foo/bar") {
		t.Error("missing security dep")
	}
	if !strings.Contains(output, "security fix") {
		t.Error("missing security fix label")
	}
	if !strings.Contains(output, "breaking changes likely") {
		t.Error("missing breaking changes label")
	}
	if !strings.Contains(output, "Total: 4 outdated") {
		t.Error("missing total count")
	}
	if !strings.Contains(output, "1 security") {
		t.Error("missing security count")
	}
	if !strings.Contains(output, "Recommendation:") {
		t.Error("missing recommendation")
	}
}

func TestFormatOutdated_Empty(t *testing.T) {
	output := FormatOutdated([]Dependency{})
	if output != "All dependencies are up to date!" {
		t.Errorf("unexpected output for empty deps: %s", output)
	}
}

func TestFormatOutdated_OnlyPatch(t *testing.T) {
	deps := []Dependency{
		{Name: "pkg1", CurrentVersion: "v1.0.0", LatestVersion: "v1.0.1", UpdateType: "patch"},
	}
	output := FormatOutdated(deps)
	if !strings.Contains(output, "PATCH:") {
		t.Error("missing PATCH section")
	}
	if strings.Contains(output, "MAJOR:") {
		t.Error("should not have MAJOR section")
	}
}

func TestFormatPlan(t *testing.T) {
	plan := &UpdatePlan{
		Dependencies: []Dependency{
			{Name: "pkg1", CurrentVersion: "v1.0.0", LatestVersion: "v1.0.1", UpdateType: "patch", SecurityFix: true},
			{Name: "pkg2", CurrentVersion: "v1.0.0", LatestVersion: "v1.1.0", UpdateType: "minor"},
		},
		RiskLevel:         "medium",
		TestCommand:       "go test ./...",
		RollbackCommand:   "git checkout go.mod go.sum && go mod download",
		EstimatedBreaking: 0,
	}

	output := FormatPlan(plan)
	if !strings.Contains(output, "Update Plan:") {
		t.Error("missing plan header")
	}
	if !strings.Contains(output, "MEDIUM") {
		t.Error("missing risk level")
	}
	if !strings.Contains(output, "go test ./...") {
		t.Error("missing test command")
	}
	if !strings.Contains(output, "pkg1") {
		t.Error("missing dependency")
	}
}

func TestFormatPlan_Nil(t *testing.T) {
	output := FormatPlan(nil)
	if output != "No updates planned." {
		t.Errorf("unexpected output for nil plan: %s", output)
	}
}

func TestFormatPlan_Empty(t *testing.T) {
	plan := &UpdatePlan{Dependencies: []Dependency{}}
	output := FormatPlan(plan)
	if output != "No updates planned." {
		t.Errorf("unexpected output for empty plan: %s", output)
	}
}

func TestDepUpdater_DetectLanguage_Go(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module test"), 0o644); err != nil {
		t.Fatal(err)
	}

	du := &DependencyUpdater{ProjectDir: dir}
	lang := du.DetectLanguage()
	if lang != "go" {
		t.Errorf("expected go, got %s", lang)
	}
}

func TestDepUpdater_DetectLanguage_JavaScript(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}

	du := &DependencyUpdater{ProjectDir: dir}
	lang := du.DetectLanguage()
	if lang != "javascript" {
		t.Errorf("expected javascript, got %s", lang)
	}
}

func TestDepUpdater_DetectLanguage_Python(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "requirements.txt"), []byte("flask==2.0"), 0o644); err != nil {
		t.Fatal(err)
	}

	du := &DependencyUpdater{ProjectDir: dir}
	lang := du.DetectLanguage()
	if lang != "python" {
		t.Errorf("expected python, got %s", lang)
	}
}

func TestDepUpdater_DetectLanguage_Rust(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "Cargo.toml"), []byte("[package]"), 0o644); err != nil {
		t.Fatal(err)
	}

	du := &DependencyUpdater{ProjectDir: dir}
	lang := du.DetectLanguage()
	if lang != "rust" {
		t.Errorf("expected rust, got %s", lang)
	}
}

func TestDepUpdater_DetectLanguage_Unknown(t *testing.T) {
	dir := t.TempDir()
	du := &DependencyUpdater{ProjectDir: dir}
	lang := du.DetectLanguage()
	if lang != "unknown" {
		t.Errorf("expected unknown, got %s", lang)
	}
}

func TestDepUpdater_DetectLanguage_Priority(t *testing.T) {
	// When multiple files exist, go.mod takes priority
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module test"), 0o644)
	os.WriteFile(filepath.Join(dir, "package.json"), []byte("{}"), 0o644)

	du := &DependencyUpdater{ProjectDir: dir}
	lang := du.DetectLanguage()
	if lang != "go" {
		t.Errorf("expected go (priority), got %s", lang)
	}
}

func TestNewDependencyUpdater(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module test"), 0o644)

	du := NewDependencyUpdater(dir)
	if du.ProjectDir != dir {
		t.Errorf("expected ProjectDir %s, got %s", dir, du.ProjectDir)
	}
	if du.Language != "go" {
		t.Errorf("expected Language go, got %s", du.Language)
	}
}

func TestBatchUpdate_FiltersByRisk(t *testing.T) {
	// We can't actually run package manager commands in tests,
	// but we can test the filtering logic by checking with an unsupported language
	// that will fail on ApplyUpdate. This tests that filtering happens correctly.
	dir := t.TempDir()
	du := &DependencyUpdater{ProjectDir: dir, Language: "unknown"}

	deps := []Dependency{
		{Name: "pkg-patch", UpdateType: "patch", LatestVersion: "v1.0.1"},
		{Name: "pkg-minor", UpdateType: "minor", LatestVersion: "v1.1.0"},
		{Name: "pkg-major", UpdateType: "major", LatestVersion: "v2.0.0"},
	}

	// With maxRisk "patch", only patch should be attempted
	_, errors := du.BatchUpdate(deps, "patch")
	// All should error due to unsupported language, but only 1 should be attempted
	if len(errors) != 1 {
		t.Errorf("expected 1 error (only patch attempted), got %d", len(errors))
	}

	// With maxRisk "minor", patch and minor should be attempted
	_, errors = du.BatchUpdate(deps, "minor")
	if len(errors) != 2 {
		t.Errorf("expected 2 errors (patch + minor attempted), got %d", len(errors))
	}

	// With maxRisk "major", all should be attempted
	_, errors = du.BatchUpdate(deps, "major")
	if len(errors) != 3 {
		t.Errorf("expected 3 errors (all attempted), got %d", len(errors))
	}
}

func TestBatchUpdate_SecurityAlwaysIncluded(t *testing.T) {
	dir := t.TempDir()
	du := &DependencyUpdater{ProjectDir: dir, Language: "unknown"}

	deps := []Dependency{
		{Name: "pkg-security-major", UpdateType: "major", LatestVersion: "v2.0.0", SecurityFix: true},
		{Name: "pkg-major", UpdateType: "major", LatestVersion: "v2.0.0"},
	}

	// With maxRisk "patch", security fix should still be included even though it's major
	_, errors := du.BatchUpdate(deps, "patch")
	if len(errors) != 1 {
		t.Errorf("expected 1 error (security fix attempted despite being major), got %d", len(errors))
	}
	// Verify the error is for the security package
	if len(errors) > 0 && !strings.Contains(errors[0].Error(), "pkg-security-major") {
		t.Errorf("expected error for pkg-security-major, got: %s", errors[0].Error())
	}
}

func TestBatchUpdate_EmptyDeps(t *testing.T) {
	dir := t.TempDir()
	du := &DependencyUpdater{ProjectDir: dir, Language: "go"}

	updated, errors := du.BatchUpdate([]Dependency{}, "major")
	if len(updated) != 0 {
		t.Errorf("expected 0 updated, got %d", len(updated))
	}
	if len(errors) != 0 {
		t.Errorf("expected 0 errors, got %d", len(errors))
	}
}

func TestRollbackCommand(t *testing.T) {
	tests := []struct {
		lang     string
		contains string
	}{
		{"go", "go.mod"},
		{"javascript", "package.json"},
		{"python", "requirements.txt"},
		{"rust", "Cargo.toml"},
	}

	for _, tt := range tests {
		du := &DependencyUpdater{ProjectDir: "/tmp", Language: tt.lang}
		plan := du.GeneratePlan([]Dependency{{Name: "pkg", UpdateType: "patch"}})
		if !strings.Contains(plan.RollbackCommand, tt.contains) {
			t.Errorf("Language %s: rollback command %q should contain %q",
				tt.lang, plan.RollbackCommand, tt.contains)
		}
	}
}
