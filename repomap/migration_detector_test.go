package repomap

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestNewMigrationDetector(t *testing.T) {
	md := NewMigrationDetector()
	if md == nil {
		t.Fatal("NewMigrationDetector returned nil")
	}
	if len(md.Rules) < 30 {
		t.Errorf("expected at least 30 built-in rules, got %d", len(md.Rules))
	}
}

func TestScanFile_GoIoutil(t *testing.T) {
	md := NewMigrationDetector()
	content := `package main

import "io/ioutil"

func main() {
	data, _ := ioutil.ReadFile("config.yaml")
	_ = ioutil.WriteFile("out.txt", data, 0644)
	dir, _ := ioutil.TempDir("", "test")
	_ = dir
}
`
	opps := md.ScanFile("src/main.go", content)
	if len(opps) < 3 {
		t.Errorf("expected at least 3 opportunities, got %d", len(opps))
	}

	// Check that ioutil.ReadFile was detected
	found := false
	for _, opp := range opps {
		if opp.NewPattern == "os.ReadFile" {
			found = true
			if opp.Line != 6 {
				t.Errorf("expected ioutil.ReadFile on line 6, got line %d", opp.Line)
			}
			if opp.Priority != "high" {
				t.Errorf("expected high priority, got %s", opp.Priority)
			}
			if !opp.AutoFixable {
				t.Error("expected AutoFixable to be true")
			}
			if opp.Category != "deprecated" {
				t.Errorf("expected category 'deprecated', got %s", opp.Category)
			}
			break
		}
	}
	if !found {
		t.Error("did not detect ioutil.ReadFile -> os.ReadFile migration")
	}
}

func TestScanFile_GoInterfaceAny(t *testing.T) {
	md := NewMigrationDetector()
	content := `package main

func process(data interface{}) interface{} {
	m := map[string]interface{}{}
	return m
}
`
	opps := md.ScanFile("handler.go", content)
	count := 0
	for _, opp := range opps {
		if opp.NewPattern == "any" {
			count++
		}
	}
	if count < 2 {
		t.Errorf("expected at least 2 interface{} -> any opportunities, got %d", count)
	}
}

func TestScanFile_PythonRules(t *testing.T) {
	md := NewMigrationDetector()
	content := `import os

path = os.path.join("/tmp", "data", "file.txt")
msg = "Hello {}".format(name)
if d.has_key("foo"):
    pass
`
	opps := md.ScanFile("script.py", content)
	if len(opps) < 3 {
		t.Errorf("expected at least 3 Python opportunities, got %d", len(opps))
	}

	categories := map[string]bool{}
	for _, opp := range opps {
		categories[opp.Category] = true
	}
	if !categories["idiom"] {
		t.Error("expected to find 'idiom' category in Python results")
	}
	if !categories["deprecated"] {
		t.Error("expected to find 'deprecated' category in Python results")
	}
}

func TestScanFile_JavaScriptRules(t *testing.T) {
	md := NewMigrationDetector()
	content := `var express = require('express');
var moment = require('moment');

var app = express();

function getData() {
    return fetch('/api').then(function(r) { return r.json(); }).catch(function(e) { console.error(e); });
}
`
	opps := md.ScanFile("app.js", content)
	if len(opps) < 3 {
		t.Errorf("expected at least 3 JS opportunities, got %d", len(opps))
	}

	foundVar := false
	foundMoment := false
	for _, opp := range opps {
		if strings.Contains(opp.NewPattern, "const") {
			foundVar = true
		}
		if strings.Contains(opp.NewPattern, "dayjs") {
			foundMoment = true
		}
	}
	if !foundVar {
		t.Error("expected to detect var -> const/let")
	}
	if !foundMoment {
		t.Error("expected to detect moment -> dayjs")
	}
}

func TestScanFile_TypeScriptInheritsJSRules(t *testing.T) {
	md := NewMigrationDetector()
	content := `const old = "hello".substr(0, 3);
`
	opps := md.ScanFile("util.ts", content)
	foundSubstr := false
	for _, opp := range opps {
		if strings.Contains(opp.NewPattern, ".slice(") {
			foundSubstr = true
		}
	}
	if !foundSubstr {
		t.Error("expected TypeScript file to match JS substr rule")
	}
}

func TestScanFile_NoMatchesForCleanCode(t *testing.T) {
	md := NewMigrationDetector()
	content := `package main

import (
	"fmt"
	"os"
)

func main() {
	data, err := os.ReadFile("config.yaml")
	if err != nil {
		fmt.Println(err)
	}
	_ = data
}
`
	opps := md.ScanFile("clean.go", content)
	// Should not detect os.ReadFile as a problem
	for _, opp := range opps {
		if strings.Contains(opp.OldPattern, "ReadFile") {
			t.Errorf("should not flag os.ReadFile, but got: %+v", opp)
		}
	}
}

func TestScanFile_UnknownLanguage(t *testing.T) {
	md := NewMigrationDetector()
	opps := md.ScanFile("readme.md", "some markdown content")
	if len(opps) != 0 {
		t.Errorf("expected 0 opportunities for markdown, got %d", len(opps))
	}
}

func TestScan_Directory(t *testing.T) {
	// Create a temp directory with test files
	dir := t.TempDir()

	goFile := filepath.Join(dir, "main.go")
	goContent := `package main

import "io/ioutil"

func main() {
	data, _ := ioutil.ReadFile("test.txt")
	_ = data
}
`
	if err := os.WriteFile(goFile, []byte(goContent), 0o644); err != nil {
		t.Fatal(err)
	}

	pyFile := filepath.Join(dir, "script.py")
	pyContent := `import os
path = os.path.join("a", "b")
`
	if err := os.WriteFile(pyFile, []byte(pyContent), 0o644); err != nil {
		t.Fatal(err)
	}

	md := NewMigrationDetector()
	opps, err := md.Scan(dir)
	if err != nil {
		t.Fatalf("Scan failed: %v", err)
	}
	if len(opps) < 2 {
		t.Errorf("expected at least 2 opportunities across files, got %d", len(opps))
	}

	// Check that results are sorted by priority (high first)
	if len(opps) > 1 && priorityRank(opps[0].Priority) > priorityRank(opps[1].Priority) {
		t.Error("results should be sorted by priority (high first)")
	}
}

func TestScan_SkipsVendorAndGit(t *testing.T) {
	dir := t.TempDir()

	// Create vendor directory with a file that would match
	vendorDir := filepath.Join(dir, "vendor")
	os.MkdirAll(vendorDir, 0o755)
	os.WriteFile(filepath.Join(vendorDir, "dep.go"), []byte(`package dep
import "io/ioutil"
var _ = ioutil.ReadFile
`), 0o644)

	// Create .git directory
	gitDir := filepath.Join(dir, ".git")
	os.MkdirAll(gitDir, 0o755)
	os.WriteFile(filepath.Join(gitDir, "hooks.go"), []byte(`package git
import "io/ioutil"
var _ = ioutil.ReadFile
`), 0o644)

	// Create a normal file
	os.WriteFile(filepath.Join(dir, "main.go"), []byte(`package main
import "io/ioutil"
var _ = ioutil.ReadFile
`), 0o644)

	md := NewMigrationDetector()
	opps, err := md.Scan(dir)
	if err != nil {
		t.Fatalf("Scan failed: %v", err)
	}

	for _, opp := range opps {
		if strings.Contains(opp.File, "vendor") {
			t.Error("should not scan vendor directory")
		}
		if strings.Contains(opp.File, ".git") {
			t.Error("should not scan .git directory")
		}
	}
}

func TestFormatOpportunities(t *testing.T) {
	opps := []MigrationOpportunity{
		{
			File:        "src/util.go",
			Line:        15,
			OldPattern:  `ioutil\.ReadFile`,
			NewPattern:  "os.ReadFile",
			Reason:      "deprecated since Go 1.16",
			Priority:    "high",
			AutoFixable: true,
			Category:    "deprecated",
		},
		{
			File:        "src/config.go",
			Line:        42,
			OldPattern:  `sort\.Slice\(`,
			NewPattern:  "slices.Sort(",
			Reason:      "consider slices.Sort for type-safe sorting (Go 1.21+)",
			Priority:    "medium",
			AutoFixable: false,
			Category:    "idiom",
		},
		{
			File:        "src/app.js",
			Line:        10,
			OldPattern:  `var\s+`,
			NewPattern:  "const or let",
			Reason:      "var has function-scope issues",
			Priority:    "low",
			AutoFixable: false,
			Category:    "idiom",
		},
	}

	output := FormatOpportunities(opps)

	if !strings.Contains(output, "Migration Opportunities (3 found)") {
		t.Errorf("expected header with count, got:\n%s", output)
	}
	if !strings.Contains(output, "HIGH (1)") {
		t.Errorf("expected HIGH section, got:\n%s", output)
	}
	if !strings.Contains(output, "MEDIUM (1)") {
		t.Errorf("expected MEDIUM section, got:\n%s", output)
	}
	if !strings.Contains(output, "LOW (1)") {
		t.Errorf("expected LOW section, got:\n%s", output)
	}
	if !strings.Contains(output, "Auto-fixable: 1/3") {
		t.Errorf("expected auto-fixable count, got:\n%s", output)
	}
	if !strings.Contains(output, "src/util.go:15") {
		t.Errorf("expected file:line reference, got:\n%s", output)
	}
}

func TestFormatOpportunities_Empty(t *testing.T) {
	output := FormatOpportunities(nil)
	if !strings.Contains(output, "0 found") {
		t.Errorf("expected '0 found' for empty input, got: %s", output)
	}
}

func TestAutoFix(t *testing.T) {
	content := `package main

import "io/ioutil"

func main() {
	data, _ := ioutil.ReadFile("config.yaml")
	_ = data
}
`
	opp := MigrationOpportunity{
		File:        "main.go",
		Line:        6,
		OldPattern:  `ioutil\.ReadFile`,
		NewPattern:  "os.ReadFile",
		Priority:    "high",
		AutoFixable: true,
		Category:    "deprecated",
	}

	result, err := AutoFix(opp, content)
	if err != nil {
		t.Fatalf("AutoFix failed: %v", err)
	}
	if !strings.Contains(result, "os.ReadFile") {
		t.Error("expected os.ReadFile in result")
	}
	if strings.Contains(result, "ioutil.ReadFile") {
		t.Error("expected ioutil.ReadFile to be replaced")
	}
}

func TestAutoFix_NotAutoFixable(t *testing.T) {
	opp := MigrationOpportunity{
		File:        "main.go",
		Line:        1,
		OldPattern:  `sort\.Slice\(`,
		NewPattern:  "slices.Sort(",
		AutoFixable: false,
	}
	_, err := AutoFix(opp, "sort.Slice(data, less)")
	if err == nil {
		t.Error("expected error for non-auto-fixable opportunity")
	}
}

func TestAutoFix_LineOutOfRange(t *testing.T) {
	opp := MigrationOpportunity{
		File:        "main.go",
		Line:        100,
		OldPattern:  `ioutil\.ReadFile`,
		NewPattern:  "os.ReadFile",
		AutoFixable: true,
	}
	_, err := AutoFix(opp, "single line")
	if err == nil {
		t.Error("expected error for out-of-range line")
	}
}

func TestAddRule(t *testing.T) {
	md := NewMigrationDetector()
	initialCount := len(md.Rules)

	md.AddRule(MigrationRule{
		ID:          "custom-rule",
		Language:    "go",
		OldPattern:  regexp.MustCompile(`log\.Fatal`),
		NewPattern:  "structured logging",
		Reason:      "prefer structured logging over log.Fatal",
		Priority:    "low",
		AutoFixable: false,
		Category:    "idiom",
	})

	if len(md.Rules) != initialCount+1 {
		t.Errorf("expected %d rules after AddRule, got %d", initialCount+1, len(md.Rules))
	}

	content := `package main
import "log"
func main() { log.Fatal("oops") }
`
	opps := md.ScanFile("main.go", content)
	found := false
	for _, opp := range opps {
		if opp.NewPattern == "structured logging" {
			found = true
			break
		}
	}
	if !found {
		t.Error("custom rule was not applied")
	}
}

func TestLanguageForFile(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"main.go", "go"},
		{"script.py", "python"},
		{"app.js", "javascript"},
		{"app.jsx", "javascript"},
		{"app.mjs", "javascript"},
		{"component.ts", "typescript"},
		{"component.tsx", "typescript"},
		{"readme.md", ""},
		{"data.json", ""},
	}
	for _, tt := range tests {
		got := languageForFile(tt.path)
		if got != tt.want {
			t.Errorf("languageForFile(%q) = %q, want %q", tt.path, got, tt.want)
		}
	}
}

func TestPriorityRank(t *testing.T) {
	if priorityRank("high") >= priorityRank("medium") {
		t.Error("high should rank lower than medium")
	}
	if priorityRank("medium") >= priorityRank("low") {
		t.Error("medium should rank lower than low")
	}
}

func TestScanFile_GoRandSeed(t *testing.T) {
	md := NewMigrationDetector()
	content := `package main

import "math/rand"

func init() {
	rand.Seed(42)
}
`
	opps := md.ScanFile("init.go", content)
	found := false
	for _, opp := range opps {
		if strings.Contains(opp.Reason, "rand.Seed") {
			found = true
			if opp.Priority != "high" {
				t.Errorf("expected high priority for rand.Seed, got %s", opp.Priority)
			}
			break
		}
	}
	if !found {
		t.Error("did not detect rand.Seed deprecation")
	}
}

func TestScanFile_GoStringsTitle(t *testing.T) {
	md := NewMigrationDetector()
	content := `package main

import "strings"

func title(s string) string {
	return strings.Title(s)
}
`
	opps := md.ScanFile("util.go", content)
	found := false
	for _, opp := range opps {
		if strings.Contains(opp.Reason, "strings.Title") {
			found = true
			break
		}
	}
	if !found {
		t.Error("did not detect strings.Title deprecation")
	}
}

func TestScanFile_PythonPrintStatement(t *testing.T) {
	md := NewMigrationDetector()
	content := `print "hello world"
print "another line"
`
	opps := md.ScanFile("old.py", content)
	found := false
	for _, opp := range opps {
		if opp.NewPattern == "print(...)" {
			found = true
			break
		}
	}
	if !found {
		t.Error("did not detect Python 2 print statement")
	}
}

func TestScanFile_MultipleRulesOnSameLine(t *testing.T) {
	md := NewMigrationDetector()
	// A line that matches multiple patterns
	content := `package main

func handle(data interface{}) {
	ioutil.ReadFile("test")
}
`
	opps := md.ScanFile("multi.go", content)
	// Should find both interface{} and ioutil.ReadFile
	foundInterface := false
	foundIoutil := false
	for _, opp := range opps {
		if opp.NewPattern == "any" {
			foundInterface = true
		}
		if opp.NewPattern == "os.ReadFile" {
			foundIoutil = true
		}
	}
	if !foundInterface {
		t.Error("missed interface{} detection")
	}
	if !foundIoutil {
		t.Error("missed ioutil.ReadFile detection")
	}
}

func TestMigrationOpportunityStruct(t *testing.T) {
	opp := MigrationOpportunity{
		File:        "main.go",
		Line:        10,
		OldPattern:  `ioutil\.ReadFile`,
		NewPattern:  "os.ReadFile",
		Reason:      "deprecated",
		Priority:    "high",
		AutoFixable: true,
		Category:    "deprecated",
	}
	if opp.File != "main.go" {
		t.Error("File field mismatch")
	}
	if opp.Line != 10 {
		t.Error("Line field mismatch")
	}
	if opp.Priority != "high" {
		t.Error("Priority field mismatch")
	}
	if opp.Category != "deprecated" {
		t.Error("Category field mismatch")
	}
}
