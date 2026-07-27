package sandbox

import (
	"testing"
)

func TestUserSandboxTOMLPath(t *testing.T) {
	path := UserSandboxTOMLPath()
	if path == "" {
		t.Error("expected non-empty path")
	}
}

func TestProjectSandboxTOMLPath(t *testing.T) {
	path := ProjectSandboxTOMLPath("/test/project")
	if path == "" {
		t.Error("expected non-empty path")
	}
}

func TestLoadTOML_NonExistent(t *testing.T) {
	cfg, err := LoadTOML("/nonexistent/sandbox.toml")
	if err != nil {
		t.Errorf("LoadTOML for non-existent file should not error: %v", err)
	}
	if len(cfg.Profiles) != 0 {
		t.Error("expected empty profiles for non-existent file")
	}
}

func TestResolve_EmptyProject(t *testing.T) {
	eff, err := Resolve("")
	if err != nil {
		t.Errorf("Resolve error: %v", err)
	}
	// Should return some effective config even with empty project
	_ = eff
}

func TestResolve_NonExistentProject(t *testing.T) {
	eff, err := Resolve("/nonexistent/project")
	if err != nil {
		t.Errorf("Resolve error: %v", err)
	}
	_ = eff
}

func TestCodeVerifier_VerifyGo(t *testing.T) {
	cv := &CodeVerifier{}
	violations := cv.VerifyGo(`package main
import "fmt"
func main() { fmt.Println("hello") }`)
	// Should return some violations or empty
	_ = violations
}

func TestCodeVerifier_VerifyGo_Empty(t *testing.T) {
	cv := &CodeVerifier{}
	violations := cv.VerifyGo("")
	if len(violations) != 0 {
		t.Errorf("expected 0 violations for empty code, got %d", len(violations))
	}
}

func TestCodeVerifier_VerifyPython(t *testing.T) {
	cv := &CodeVerifier{}
	violations := cv.VerifyPython("import os\nprint('hello')")
	_ = violations
}

func TestCodeVerifier_VerifyPython_Empty(t *testing.T) {
	cv := &CodeVerifier{}
	violations := cv.VerifyPython("")
	if len(violations) != 0 {
		t.Errorf("expected 0 violations for empty code, got %d", len(violations))
	}
}

func TestCodeVerifier_VerifyBash(t *testing.T) {
	cv := &CodeVerifier{}
	violations := cv.VerifyBash("echo hello")
	_ = violations
}

func TestCodeVerifier_VerifyBash_Empty(t *testing.T) {
	cv := &CodeVerifier{}
	violations := cv.VerifyBash("")
	if len(violations) != 0 {
		t.Errorf("expected 0 violations for empty code, got %d", len(violations))
	}
}
