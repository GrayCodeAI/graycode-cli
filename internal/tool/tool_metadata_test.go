package tool

import (
	"testing"
)

func TestToolMetadata_BashTool(t *testing.T) {
	t.Parallel()
	bash := &BashTool{}

	if bash.Name() == "" {
		t.Error("Name() should not be empty")
	}
	if bash.Description() == "" {
		t.Error("Description() should not be empty")
	}
	params := bash.Parameters()
	if params == nil {
		t.Error("Parameters() should not be nil")
	}
	if rp, ok := interface{}(bash).(RiskLevelProvider); ok {
		risk := rp.RiskLevel()
		if risk == "" {
			t.Error("RiskLevel() should not be empty")
		}
	}
}

func TestToolMetadata_ReadTool(t *testing.T) {
	t.Parallel()
	read := &FileReadTool{}

	if read.Name() == "" {
		t.Error("Name() should not be empty")
	}
	if read.Description() == "" {
		t.Error("Description() should not be empty")
	}
	if read.Parameters() == nil {
		t.Error("Parameters() should not be nil")
	}
}

func TestToolMetadata_WriteTool(t *testing.T) {
	t.Parallel()
	write := &FileWriteTool{}

	if write.Name() == "" {
		t.Error("Name() should not be empty")
	}
	if write.Description() == "" {
		t.Error("Description() should not be empty")
	}
	if write.Parameters() == nil {
		t.Error("Parameters() should not be nil")
	}
}

func TestToolMetadata_EditTool(t *testing.T) {
	t.Parallel()
	edit := &FileEditTool{}

	if edit.Name() == "" {
		t.Error("Name() should not be empty")
	}
	if edit.Description() == "" {
		t.Error("Description() should not be empty")
	}
	if edit.Parameters() == nil {
		t.Error("Parameters() should not be nil")
	}
}

func TestToolMetadata_GrepTool(t *testing.T) {
	t.Parallel()
	grep := &GrepTool{}

	if grep.Name() == "" {
		t.Error("Name() should not be empty")
	}
	if grep.Description() == "" {
		t.Error("Description() should not be empty")
	}
	if grep.Parameters() == nil {
		t.Error("Parameters() should not be nil")
	}
}

func TestToolMetadata_GlobTool(t *testing.T) {
	t.Parallel()
	glob := &GlobTool{}

	if glob.Name() == "" {
		t.Error("Name() should not be empty")
	}
	if glob.Description() == "" {
		t.Error("Description() should not be empty")
	}
	if glob.Parameters() == nil {
		t.Error("Parameters() should not be nil")
	}
}

func TestRegistry_WithTools(t *testing.T) {
	t.Parallel()
	registry := NewRegistry(&BashTool{}, &FileReadTool{}, &FileWriteTool{}, &FileEditTool{}, &GrepTool{}, &GlobTool{})

	tools := registry.PrimaryTools()
	if len(tools) != 6 {
		t.Errorf("PrimaryTools() returned %d, want 6", len(tools))
	}

	if _, found := registry.Get("Bash"); !found {
		t.Error("should find Bash tool")
	}
	if _, found := registry.Get("NonExistent"); found {
		t.Error("should not find NonExistent tool")
	}
}

func TestRegistry_GraycodeRouterTools_WithTools(t *testing.T) {
	t.Parallel()
	registry := NewRegistry(&BashTool{}, &FileReadTool{})
	graycodeRouterTools := registry.GraycodeRouterTools()

	if len(graycodeRouterTools) != 2 {
		t.Errorf("GraycodeRouterTools() returned %d, want 2", len(graycodeRouterTools))
	}
	for _, et := range graycodeRouterTools {
		if et.Name == "" {
			t.Error("GraycodeRouterTool.Name should not be empty")
		}
	}
}
