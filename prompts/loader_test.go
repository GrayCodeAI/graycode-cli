package prompts

import (
	"strings"
	"testing"
)

func TestDefaultContextHasNonEmptyFields(t *testing.T) {
	ctx := DefaultContext()
	if ctx.Date == "" {
		t.Error("DefaultContext Date is empty")
	}
	if ctx.OS == "" {
		t.Error("DefaultContext OS is empty")
	}
	// WorkDir may be empty in unusual test environments but should normally be set
	if ctx.WorkDir == "" {
		t.Error("DefaultContext WorkDir is empty")
	}
}

func TestBuildSystemPromptContainsSections(t *testing.T) {
	ctx := DefaultContext()
	result, err := BuildSystemPrompt(ctx)
	if err != nil {
		t.Fatalf("BuildSystemPrompt failed: %v", err)
	}

	// Verify it contains content from each section
	for _, want := range []string{
		"Hawk",             // from role.md
		"Tool Usage",       // from tools.md
		"Coding Practices", // from practices.md
		"Communication",    // from communication.md
	} {
		if !strings.Contains(result, want) {
			t.Errorf("system prompt missing expected section text: %q", want)
		}
	}
}

func TestBuildSystemPromptTemplateSubstitution(t *testing.T) {
	ctx := PromptContext{
		Date:    "Monday, 2026-05-01",
		WorkDir: "/test/dir",
		OS:      "testOS",
		Shell:   "/bin/testsh",
	}
	result, err := BuildSystemPrompt(ctx)
	if err != nil {
		t.Fatalf("BuildSystemPrompt failed: %v", err)
	}
	if !strings.Contains(result, "Monday, 2026-05-01") {
		t.Error("template did not substitute Date")
	}
	if !strings.Contains(result, "/test/dir") {
		t.Error("template did not substitute WorkDir")
	}
	if !strings.Contains(result, "testOS") {
		t.Error("template did not substitute OS")
	}
	if !strings.Contains(result, "/bin/testsh") {
		t.Error("template did not substitute Shell")
	}
}

func TestBuildSystemPromptHasSeparators(t *testing.T) {
	ctx := DefaultContext()
	result, err := BuildSystemPrompt(ctx)
	if err != nil {
		t.Fatalf("BuildSystemPrompt failed: %v", err)
	}
	// Should have 3 separators for 4 sections
	count := strings.Count(result, "---")
	if count != 3 {
		t.Errorf("expected 3 section separators, got %d", count)
	}
}

func TestBuildSubAgentPrompt(t *testing.T) {
	ctx := PromptContext{
		MaxTurns: 10,
		Task:     "find all TODO comments",
	}
	result, err := BuildSubAgentPrompt(ctx)
	if err != nil {
		t.Fatalf("BuildSubAgentPrompt failed: %v", err)
	}
	if !strings.Contains(result, "10") {
		t.Error("sub-agent prompt missing MaxTurns value")
	}
	if !strings.Contains(result, "find all TODO comments") {
		t.Error("sub-agent prompt missing Task value")
	}
	if !strings.Contains(result, "sub-agent") {
		t.Error("sub-agent prompt missing sub-agent identifier")
	}
}

func TestListTemplates(t *testing.T) {
	names := ListTemplates()
	if len(names) == 0 {
		t.Fatal("ListTemplates returned empty")
	}
	want := map[string]bool{
		"role.md":          false,
		"tools.md":         false,
		"practices.md":     false,
		"communication.md": false,
		"subagent.md":      false,
	}
	for _, name := range names {
		if _, ok := want[name]; ok {
			want[name] = true
		}
	}
	for name, found := range want {
		if !found {
			t.Errorf("ListTemplates missing %q", name)
		}
	}
}

func TestLoadTemplate(t *testing.T) {
	content, err := LoadTemplate("role.md")
	if err != nil {
		t.Fatalf("LoadTemplate(role.md) failed: %v", err)
	}
	if !strings.Contains(content, "Hawk") {
		t.Error("role.md template missing 'Hawk'")
	}
}
