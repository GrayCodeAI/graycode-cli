package memory

import (
	"testing"
)

func TestProactiveContextReset(t *testing.T) {
	bridge := &YaadBridge{ready: false}
	pc := NewProactiveContext(bridge)

	pc.TrackFile("/tmp/auth.go")
	pc.TrackFile("/tmp/main.go")
	pc.injectedKeys["test"] = true

	pc.Reset()

	if len(pc.activeFiles) != 0 {
		t.Errorf("expected 0 active files after reset, got %d", len(pc.activeFiles))
	}
	if len(pc.injectedKeys) != 0 {
		t.Errorf("expected 0 injected keys after reset, got %d", len(pc.injectedKeys))
	}
}

func TestProactiveContextTrackFile(t *testing.T) {
	bridge := &YaadBridge{ready: false}
	pc := NewProactiveContext(bridge)

	pc.TrackFile("/tmp/auth.go")
	pc.TrackFile("/tmp/main.go")
	pc.TrackFile("/tmp/auth.go") // duplicate

	if len(pc.activeFiles) != 2 {
		t.Errorf("expected 2 unique files, got %d", len(pc.activeFiles))
	}
}

func TestProactiveContextForFileNotReady(t *testing.T) {
	bridge := &YaadBridge{ready: false}
	pc := NewProactiveContext(bridge)

	result := pc.ContextForFile("/tmp/test.go")
	if result != "" {
		t.Errorf("expected empty result when bridge not ready, got %q", result)
	}
}

func TestProactiveContextForToolWithPath(t *testing.T) {
	bridge := &YaadBridge{ready: false}
	pc := NewProactiveContext(bridge)

	result := pc.ContextForTool("Read", map[string]interface{}{
		"file_path": "/tmp/test.go",
	})
	if result != "" {
		t.Errorf("expected empty when bridge not ready, got %q", result)
	}
}
