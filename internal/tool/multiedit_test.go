package tool

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestMultiEditApplyAll(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.go")
	os.WriteFile(path, []byte("func hello() {\n\tfmt.Println(\"hello\")\n\tfmt.Println(\"world\")\n}"), 0o644)

	input, _ := json.Marshal(map[string]interface{}{
		"file_path": path,
		"edits": []map[string]interface{}{
			{"old_string": "hello", "new_string": "greet", "replace_all": true},
			{"old_string": "world", "new_string": "earth"},
		},
	})

	out, err := (MultiEditTool{}).Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("MultiEdit: %v", err)
	}
	if out != "Applied 2/2 edit(s) to "+path+"." {
		t.Errorf("unexpected output: %s", out)
	}

	data, _ := os.ReadFile(path)
	content := string(data)
	if content != "func greet() {\n\tfmt.Println(\"greet\")\n\tfmt.Println(\"earth\")\n}" {
		t.Errorf("unexpected content: %s", content)
	}
}

func TestMultiEditPartialFailure(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")
	os.WriteFile(path, []byte("aaa bbb ccc"), 0o644)

	input, _ := json.Marshal(map[string]interface{}{
		"file_path": path,
		"edits": []map[string]interface{}{
			{"old_string": "aaa", "new_string": "xxx"},
			{"old_string": "NOTFOUND", "new_string": "yyy"},
			{"old_string": "ccc", "new_string": "zzz"},
		},
	})

	out, err := (MultiEditTool{}).Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("MultiEdit: %v", err)
	}
	if out != "Applied 2/3 edit(s) to "+path+"." {
		t.Errorf("unexpected output: %s", out)
	}
}

func TestMultiEditNoEdits(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")
	os.WriteFile(path, []byte("content"), 0o644)

	input, _ := json.Marshal(map[string]interface{}{
		"file_path": path,
		"edits":     []map[string]interface{}{},
	})

	_, err := (MultiEditTool{}).Execute(context.Background(), input)
	if err == nil {
		t.Error("expected error for empty edits")
	}
}

func TestDownloadToolMissingParams(t *testing.T) {
	input, _ := json.Marshal(map[string]interface{}{})
	_, err := (DownloadTool{}).Execute(context.Background(), input)
	if err == nil {
		t.Error("expected error for missing params")
	}
}
