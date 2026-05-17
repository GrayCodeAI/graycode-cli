package tool

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNewCompatChecker(t *testing.T) {
	cc := NewCompatChecker()
	if cc == nil {
		t.Fatal("NewCompatChecker returned nil")
	}
}

func createTestPackage(t *testing.T, dir string, source string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "api.go"), []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
}

const testPkgV1 = `package testpkg

import "context"

// Exported constants
const (
	Version = "1.0.0"
	MaxRetries int = 3
)

// Exported variable
var DefaultTimeout int = 30

// Handler is a function type
type Handler func(ctx context.Context) error

// Options holds configuration
type Options struct {
	Name    string
	Retries int
	Verbose bool
}

// Processor is an interface
type Processor interface {
	Process(ctx context.Context) error
	Reset()
}

// NewOptions creates default options
func NewOptions(name string) *Options {
	return &Options{Name: name}
}

// Run executes the handler
func Run(ctx context.Context, h Handler) error {
	return h(ctx)
}

// helper is unexported
func helper() {}
`

const testPkgV2Breaking = `package testpkg

import "context"

// Exported constants
const (
	Version = "2.0.0"
	MaxRetries int = 3
)

// Exported variable
var DefaultTimeout int = 30

// Handler is a function type - CHANGED
type Handler func(ctx context.Context, opts *Options) error

// Options holds configuration - field removed
type Options struct {
	Name    string
	Retries int
}

// Processor is an interface - method added
type Processor interface {
	Process(ctx context.Context) error
	Reset()
	Validate(ctx context.Context) error
}

// NewOptions creates default options - signature changed
func NewOptions(name string, retries int) *Options {
	return &Options{Name: name, Retries: retries}
}

// Run was REMOVED

// NewHelper is a new compatible addition
func NewHelper(ctx context.Context) error {
	return nil
}

// helper is unexported
func helper() {}
`

const testPkgV2Compatible = `package testpkg

import "context"

// Exported constants
const (
	Version = "1.0.0"
	MaxRetries int = 3
)

// Exported variable
var DefaultTimeout int = 30

// Handler is a function type
type Handler func(ctx context.Context) error

// Options holds configuration
type Options struct {
	Name    string
	Retries int
	Verbose bool
}

// Processor is an interface
type Processor interface {
	Process(ctx context.Context) error
	Reset()
}

// NewOptions creates default options
func NewOptions(name string) *Options {
	return &Options{Name: name}
}

// Run executes the handler
func Run(ctx context.Context, h Handler) error {
	return h(ctx)
}

// NewHelper is a compatible addition
func NewHelper(ctx context.Context) error {
	return nil
}

// helper is unexported
func helper() {}
`

func TestSnapshot(t *testing.T) {
	dir := t.TempDir()
	createTestPackage(t, dir, testPkgV1)

	cc := NewCompatChecker()
	snap, err := cc.Snapshot(dir)
	if err != nil {
		t.Fatalf("Snapshot failed: %v", err)
	}

	if snap.Package != "testpkg" {
		t.Errorf("expected package testpkg, got %s", snap.Package)
	}

	// Check functions
	funcNames := make(map[string]bool)
	for _, f := range snap.Functions {
		funcNames[f.Name] = true
	}
	if !funcNames["NewOptions"] {
		t.Error("expected exported function NewOptions")
	}
	if !funcNames["Run"] {
		t.Error("expected exported function Run")
	}
	if funcNames["helper"] {
		t.Error("unexported function helper should not be in snapshot")
	}

	// Check types
	typeNames := make(map[string]bool)
	for _, ts := range snap.Types {
		typeNames[ts.Name] = true
	}
	if !typeNames["Options"] {
		t.Error("expected exported type Options")
	}
	if !typeNames["Handler"] {
		t.Error("expected exported type Handler")
	}

	// Check interfaces
	ifaceNames := make(map[string]bool)
	for _, i := range snap.Interfaces {
		ifaceNames[i.Name] = true
	}
	if !ifaceNames["Processor"] {
		t.Error("expected exported interface Processor")
	}

	// Check constants
	constNames := make(map[string]bool)
	for _, c := range snap.Constants {
		constNames[c.Name] = true
	}
	if !constNames["Version"] {
		t.Error("expected exported constant Version")
	}
	if !constNames["MaxRetries"] {
		t.Error("expected exported constant MaxRetries")
	}

	// Check variables
	varNames := make(map[string]bool)
	for _, v := range snap.Variables {
		varNames[v.Name] = true
	}
	if !varNames["DefaultTimeout"] {
		t.Error("expected exported variable DefaultTimeout")
	}
}

func TestSnapshotInvalidPath(t *testing.T) {
	cc := NewCompatChecker()
	_, err := cc.Snapshot("/nonexistent/path")
	if err == nil {
		t.Error("expected error for invalid path")
	}
}

func TestCompareBreakingChanges(t *testing.T) {
	dirV1 := t.TempDir()
	dirV2 := t.TempDir()
	createTestPackage(t, dirV1, testPkgV1)
	createTestPackage(t, dirV2, testPkgV2Breaking)

	cc := NewCompatChecker()
	snapV1, err := cc.Snapshot(dirV1)
	if err != nil {
		t.Fatalf("Snapshot V1 failed: %v", err)
	}
	snapV2, err := cc.Snapshot(dirV2)
	if err != nil {
		t.Fatalf("Snapshot V2 failed: %v", err)
	}

	changes := cc.Compare(snapV1, snapV2)
	if len(changes) == 0 {
		t.Fatal("expected breaking changes, got none")
	}

	// Check for removed function
	found := false
	for _, ch := range changes {
		if ch.Type == "removed" && strings.Contains(ch.Symbol, "Run") && ch.Severity == "breaking" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected breaking change for removed function Run")
	}

	// Check for signature change
	found = false
	for _, ch := range changes {
		if ch.Type == "signature_changed" && strings.Contains(ch.Symbol, "NewOptions") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected signature_changed for NewOptions")
	}

	// Check for removed field
	found = false
	for _, ch := range changes {
		if ch.Type == "field_removed" && strings.Contains(ch.Symbol, "Verbose") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected field_removed for Options.Verbose")
	}

	// Check for interface method added
	found = false
	for _, ch := range changes {
		if ch.Type == "method_added" && strings.Contains(ch.Symbol, "Processor") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected method_added for Processor interface")
	}

	// Check for compatible addition
	found = false
	for _, ch := range changes {
		if ch.Severity == "compatible" && strings.Contains(ch.Symbol, "NewHelper") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected compatible addition for NewHelper")
	}
}

func TestCompareCompatible(t *testing.T) {
	dirV1 := t.TempDir()
	dirV2 := t.TempDir()
	createTestPackage(t, dirV1, testPkgV1)
	createTestPackage(t, dirV2, testPkgV2Compatible)

	cc := NewCompatChecker()
	snapV1, err := cc.Snapshot(dirV1)
	if err != nil {
		t.Fatalf("Snapshot V1 failed: %v", err)
	}
	snapV2, err := cc.Snapshot(dirV2)
	if err != nil {
		t.Fatalf("Snapshot V2 failed: %v", err)
	}

	if !cc.IsBackwardCompatible(snapV1, snapV2) {
		changes := cc.Compare(snapV1, snapV2)
		for _, ch := range changes {
			if ch.Severity == "breaking" {
				t.Errorf("unexpected breaking change: %+v", ch)
			}
		}
	}
}

func TestIsBackwardCompatible(t *testing.T) {
	dirV1 := t.TempDir()
	dirV2 := t.TempDir()
	createTestPackage(t, dirV1, testPkgV1)
	createTestPackage(t, dirV2, testPkgV2Breaking)

	cc := NewCompatChecker()
	snapV1, err := cc.Snapshot(dirV1)
	if err != nil {
		t.Fatal(err)
	}
	snapV2, err := cc.Snapshot(dirV2)
	if err != nil {
		t.Fatal(err)
	}

	if cc.IsBackwardCompatible(snapV1, snapV2) {
		t.Error("expected not backward compatible")
	}
}

func TestFormatChanges(t *testing.T) {
	changes := []BreakingChange{
		{Type: "removed", Symbol: "func OldHandler", Old: "func OldHandler()", Severity: "breaking"},
		{Type: "signature_changed", Symbol: "func Process", Old: "func Process(context.Context) error", New: "func Process(context.Context, Options) error", Severity: "breaking"},
		{Type: "method_added", Symbol: "interface Validator", New: "func Validate(context.Context) error", Severity: "breaking"},
		{Type: "removed", Symbol: "func NewHelper", New: "func NewHelper(context.Context) error", Severity: "compatible"},
	}

	cc := NewCompatChecker()
	output := cc.FormatChanges(changes)

	if !strings.Contains(output, "API Compatibility Check:") {
		t.Error("expected header in output")
	}
	if !strings.Contains(output, "BREAKING") {
		t.Error("expected BREAKING in output")
	}
	if !strings.Contains(output, "CAUTION") {
		t.Error("expected CAUTION in output")
	}
	if !strings.Contains(output, "COMPATIBLE") {
		t.Error("expected COMPATIBLE in output")
	}
	if !strings.Contains(output, "2 breaking, 1 caution, 1 compatible") {
		t.Errorf("expected result summary, got:\n%s", output)
	}
}

func TestSaveAndLoadSnapshot(t *testing.T) {
	dir := t.TempDir()
	createTestPackage(t, dir, testPkgV1)

	cc := NewCompatChecker()
	snap, err := cc.Snapshot(dir)
	if err != nil {
		t.Fatal(err)
	}

	savePath := filepath.Join(t.TempDir(), "baseline.json")
	if err := cc.SaveSnapshot(snap, savePath); err != nil {
		t.Fatalf("SaveSnapshot failed: %v", err)
	}

	loaded, err := cc.LoadSnapshot(savePath)
	if err != nil {
		t.Fatalf("LoadSnapshot failed: %v", err)
	}

	if loaded.Package != snap.Package {
		t.Errorf("package mismatch: got %s, want %s", loaded.Package, snap.Package)
	}
	if len(loaded.Functions) != len(snap.Functions) {
		t.Errorf("functions count mismatch: got %d, want %d", len(loaded.Functions), len(snap.Functions))
	}
	if len(loaded.Types) != len(snap.Types) {
		t.Errorf("types count mismatch: got %d, want %d", len(loaded.Types), len(snap.Types))
	}
	if len(loaded.Interfaces) != len(snap.Interfaces) {
		t.Errorf("interfaces count mismatch: got %d, want %d", len(loaded.Interfaces), len(snap.Interfaces))
	}
	if len(loaded.Constants) != len(snap.Constants) {
		t.Errorf("constants count mismatch: got %d, want %d", len(loaded.Constants), len(snap.Constants))
	}
	if len(loaded.Variables) != len(snap.Variables) {
		t.Errorf("variables count mismatch: got %d, want %d", len(loaded.Variables), len(snap.Variables))
	}
}

func TestLoadSnapshotInvalidPath(t *testing.T) {
	cc := NewCompatChecker()
	_, err := cc.LoadSnapshot("/nonexistent/file.json")
	if err == nil {
		t.Error("expected error for nonexistent file")
	}
}

func TestLoadSnapshotInvalidJSON(t *testing.T) {
	dir := t.TempDir()
	badFile := filepath.Join(dir, "bad.json")
	if err := os.WriteFile(badFile, []byte("not json"), 0o644); err != nil {
		t.Fatal(err)
	}

	cc := NewCompatChecker()
	_, err := cc.LoadSnapshot(badFile)
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestAPICompatToolName(t *testing.T) {
	tool := NewAPICompatTool()
	if tool.Name() != "APICompat" {
		t.Errorf("expected name APICompat, got %s", tool.Name())
	}
}

func TestAPICompatToolDescription(t *testing.T) {
	tool := NewAPICompatTool()
	if tool.Description() == "" {
		t.Error("expected non-empty description")
	}
}

func TestAPICompatToolParameters(t *testing.T) {
	tool := NewAPICompatTool()
	params := tool.Parameters()
	if params == nil {
		t.Fatal("expected non-nil parameters")
	}
	props, ok := params["properties"].(map[string]interface{})
	if !ok {
		t.Fatal("expected properties map")
	}
	if _, ok := props["package_path"]; !ok {
		t.Error("expected package_path parameter")
	}
	if _, ok := props["baseline_path"]; !ok {
		t.Error("expected baseline_path parameter")
	}
	if _, ok := props["save_baseline"]; !ok {
		t.Error("expected save_baseline parameter")
	}
}

func TestAPICompatToolSaveBaseline(t *testing.T) {
	dir := t.TempDir()
	createTestPackage(t, dir, testPkgV1)
	baselinePath := filepath.Join(t.TempDir(), "baseline.json")

	tool := NewAPICompatTool()
	input, _ := json.Marshal(map[string]interface{}{
		"package_path":  dir,
		"baseline_path": baselinePath,
		"save_baseline": true,
	})

	result, err := tool.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if !strings.Contains(result, "Saved API baseline") {
		t.Errorf("expected save confirmation, got: %s", result)
	}

	// Verify file was created
	if _, err := os.Stat(baselinePath); os.IsNotExist(err) {
		t.Error("baseline file was not created")
	}
}

func TestAPICompatToolCompare(t *testing.T) {
	dirV1 := t.TempDir()
	dirV2 := t.TempDir()
	createTestPackage(t, dirV1, testPkgV1)
	createTestPackage(t, dirV2, testPkgV2Breaking)

	// First save a baseline from V1
	tool := NewAPICompatTool()
	baselinePath := filepath.Join(t.TempDir(), "baseline.json")
	input, _ := json.Marshal(map[string]interface{}{
		"package_path":  dirV1,
		"baseline_path": baselinePath,
		"save_baseline": true,
	})
	_, err := tool.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("Save baseline failed: %v", err)
	}

	// Now compare V2 against baseline
	input, _ = json.Marshal(map[string]interface{}{
		"package_path":  dirV2,
		"baseline_path": baselinePath,
	})
	result, err := tool.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("Compare failed: %v", err)
	}
	if !strings.Contains(result, "BREAKING") {
		t.Errorf("expected BREAKING in result, got: %s", result)
	}
}

func TestAPICompatToolNoBreaking(t *testing.T) {
	dirV1 := t.TempDir()
	dirV2 := t.TempDir()
	createTestPackage(t, dirV1, testPkgV1)
	createTestPackage(t, dirV2, testPkgV1) // Same package

	// Save baseline
	tool := NewAPICompatTool()
	baselinePath := filepath.Join(t.TempDir(), "baseline.json")
	input, _ := json.Marshal(map[string]interface{}{
		"package_path":  dirV1,
		"baseline_path": baselinePath,
		"save_baseline": true,
	})
	_, err := tool.Execute(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}

	// Compare same package
	input, _ = json.Marshal(map[string]interface{}{
		"package_path":  dirV2,
		"baseline_path": baselinePath,
	})
	result, err := tool.Execute(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, "No changes detected") {
		t.Errorf("expected no changes, got: %s", result)
	}
}

func TestAPICompatToolMissingBaseline(t *testing.T) {
	dir := t.TempDir()
	createTestPackage(t, dir, testPkgV1)

	tool := NewAPICompatTool()
	input, _ := json.Marshal(map[string]interface{}{
		"package_path":  dir,
		"baseline_path": "/nonexistent/baseline.json",
	})
	_, err := tool.Execute(context.Background(), input)
	if err == nil {
		t.Error("expected error for missing baseline")
	}
}

func TestAPICompatToolMissingPackagePath(t *testing.T) {
	tool := NewAPICompatTool()
	input, _ := json.Marshal(map[string]interface{}{})
	_, err := tool.Execute(context.Background(), input)
	if err == nil {
		t.Error("expected error for missing package_path")
	}
}

func TestFuncSigWithReceiver(t *testing.T) {
	src := `package testpkg

type Server struct{}

func (s *Server) Start() error { return nil }
func (s *Server) Stop() {}
func (s Server) Name() string { return "" }
`
	dir := t.TempDir()
	createTestPackage(t, dir, src)

	cc := NewCompatChecker()
	snap, err := cc.Snapshot(dir)
	if err != nil {
		t.Fatal(err)
	}

	receiverFuncs := 0
	for _, f := range snap.Functions {
		if f.Receiver != "" {
			receiverFuncs++
		}
	}
	if receiverFuncs != 3 {
		t.Errorf("expected 3 receiver functions, got %d", receiverFuncs)
	}
}

func TestExprToString(t *testing.T) {
	// Test via snapshot parsing of complex types
	src := `package testpkg

type Complex struct {
	Items    []string
	Mapping  map[string]int
	Callback func(int) error
	Ptr      *Complex
	Ch       chan int
}
`
	dir := t.TempDir()
	createTestPackage(t, dir, src)

	cc := NewCompatChecker()
	snap, err := cc.Snapshot(dir)
	if err != nil {
		t.Fatal(err)
	}

	if len(snap.Types) != 1 {
		t.Fatalf("expected 1 type, got %d", len(snap.Types))
	}

	fields := make(map[string]string)
	for _, f := range snap.Types[0].Fields {
		fields[f.Name] = f.Type
	}

	if fields["Items"] != "[]string" {
		t.Errorf("expected []string, got %s", fields["Items"])
	}
	if fields["Mapping"] != "map[string]int" {
		t.Errorf("expected map[string]int, got %s", fields["Mapping"])
	}
	if !strings.Contains(fields["Callback"], "func(") {
		t.Errorf("expected func type, got %s", fields["Callback"])
	}
	if fields["Ptr"] != "*Complex" {
		t.Errorf("expected *Complex, got %s", fields["Ptr"])
	}
	if fields["Ch"] != "chan int" {
		t.Errorf("expected chan int, got %s", fields["Ch"])
	}
}

func TestToolInterface(t *testing.T) {
	// Verify APICompatTool implements Tool interface
	var _ Tool = (*APICompatTool)(nil)
}
