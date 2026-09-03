package jsonc_test

import (
	"reflect"
	"testing"

	"github.com/GrayCodeAI/graycode-cli/internal/jsonc"
)

func TestUnmarshal_PlainJSON(t *testing.T) {
	var v map[string]interface{}
	if err := jsonc.Unmarshal([]byte(`{"a": 1, "b": "x"}`), &v); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if v["a"].(float64) != 1 || v["b"].(string) != "x" {
		t.Errorf("unexpected: %+v", v)
	}
}

func TestUnmarshal_LineComment(t *testing.T) {
	src := []byte(`{
  "a": 1, // this is a comment
  "b": 2
}`)
	var v map[string]interface{}
	if err := jsonc.Unmarshal(src, &v); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if v["a"].(float64) != 1 || v["b"].(float64) != 2 {
		t.Errorf("unexpected: %+v", v)
	}
}

func TestUnmarshal_BlockComment(t *testing.T) {
	src := []byte(`{
  /* multi
     line
     comment */
  "a": 1
}`)
	var v map[string]interface{}
	if err := jsonc.Unmarshal(src, &v); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if v["a"].(float64) != 1 {
		t.Errorf("unexpected: %+v", v)
	}
}

func TestUnmarshal_TrailingCommaInObject(t *testing.T) {
	src := []byte(`{"a": 1, "b": 2,}`)
	var v map[string]interface{}
	if err := jsonc.Unmarshal(src, &v); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if v["a"].(float64) != 1 || v["b"].(float64) != 2 {
		t.Errorf("unexpected: %+v", v)
	}
}

func TestUnmarshal_TrailingCommaInArray(t *testing.T) {
	src := []byte(`[1, 2, 3,]`)
	var v []interface{}
	if err := jsonc.Unmarshal(src, &v); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if len(v) != 3 {
		t.Errorf("expected 3 elements, got %d", len(v))
	}
}

func TestUnmarshal_CommentInString(t *testing.T) {
	// Comments inside strings must NOT be stripped.
	src := []byte(`{"a": "this // is not a comment", "b": "/* also not */"}`)
	var v map[string]interface{}
	if err := jsonc.Unmarshal(src, &v); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if v["a"].(string) != "this // is not a comment" {
		t.Errorf("a was stripped: %q", v["a"])
	}
	if v["b"].(string) != "/* also not */" {
		t.Errorf("b was stripped: %q", v["b"])
	}
}

func TestUnmarshal_EscapedQuoteInString(t *testing.T) {
	src := []byte(`{"a": "she said \"hi\" // and left", "b": 1}`)
	var v map[string]interface{}
	if err := jsonc.Unmarshal(src, &v); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	// "she said \"hi\" // and left" - the // is INSIDE the string
	if v["a"].(string) != `she said "hi" // and left` {
		t.Errorf("a: %q", v["a"])
	}
	if v["b"].(float64) != 1 {
		t.Errorf("b: %v", v["b"])
	}
}

func TestUnmarshal_UnterminatedBlockComment(t *testing.T) {
	src := []byte(`{"a": 1, /* unterminated`)
	var v map[string]interface{}
	if err := jsonc.Unmarshal(src, &v); err == nil {
		t.Error("expected error for unterminated block comment")
	}
}

func TestUnmarshal_RealisticClaudeSettings(t *testing.T) {
	// A realistic Claude Code settings.json with comments and trailing
	// commas, like validated settings files may include.
	src := []byte(`{
  // Permissions configuration
  "permissions": {
    "allow": ["Read", "Grep", "Glob",], // safe tools
    "deny": ["Bash(rm -rf:*)"], // never allow destructive rm
  },
  /* Hooks setup: pre-tool-use and post-tool-use */
  "hooks": {
    "PreToolUse": [{
      "matcher": "Bash",
      "hooks": [{"type": "command", "command": "/usr/local/bin/pre-bash"}],
    },],
  },
  "model": "claude-sonnet-4-6", // default model
}`)
	var v map[string]interface{}
	if err := jsonc.Unmarshal(src, &v); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if v["model"].(string) != "claude-sonnet-4-6" {
		t.Errorf("model: %v", v["model"])
	}
	perms := v["permissions"].(map[string]interface{})
	allow := perms["allow"].([]interface{})
	if len(allow) != 3 {
		t.Errorf("allow: expected 3 entries, got %d", len(allow))
	}
	deny := perms["deny"].([]interface{})
	if len(deny) != 1 || deny[0].(string) != "Bash(rm -rf:*)" {
		t.Errorf("deny: %v", deny)
	}
}

func TestUnmarshal_NestedStruct(t *testing.T) {
	type Inner struct {
		Value string `json:"value"`
	}
	type Outer struct {
		Name  string `json:"name"`
		Inner Inner  `json:"inner"`
	}
	src := []byte(`{
		"name": "test", // comment
		"inner": {
			"value": "hello", /* block */
		},
	}`)
	var o Outer
	if err := jsonc.Unmarshal(src, &o); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if o.Name != "test" || o.Inner.Value != "hello" {
		t.Errorf("unexpected: %+v", o)
	}
}

func TestValid_Plain(t *testing.T) {
	if !jsonc.Valid([]byte(`{"a": 1}`)) {
		t.Error("expected valid")
	}
}

func TestValid_CommentsAndTrailing(t *testing.T) {
	src := []byte(`{
		// comment
		"a": 1,
		"b": [1, 2,], /* trailing */
	}`)
	if !jsonc.Valid(src) {
		t.Error("expected valid with comments + trailing")
	}
}

func TestValid_InvalidJSON(t *testing.T) {
	if jsonc.Valid([]byte(`{"a": }`)) {
		t.Error("expected invalid")
	}
}

func TestValid_UnterminatedComment(t *testing.T) {
	if jsonc.Valid([]byte(`{"a": 1, /* unterminated`)) {
		t.Error("expected invalid")
	}
}

func TestStrip_PreservesStructure(t *testing.T) {
	src := []byte(`{"a": 1 /* c */, "b": 2,}`)
	out, err := jsonc.Strip(src)
	if err != nil {
		t.Fatal(err)
	}
	expected := `{"a": 1 , "b": 2}`
	if !reflect.DeepEqual([]byte(expected), out) {
		t.Errorf("strip mismatch:\n got: %q\nwant: %q", out, expected)
	}
}

func TestMarshalIndent_RoundTrip(t *testing.T) {
	v := map[string]interface{}{"a": 1, "b": []interface{}{1.0, 2.0, 3.0}}
	out, err := jsonc.MarshalIndent(v, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if len(out) == 0 {
		t.Error("expected non-empty output")
	}
}
