package cmd

import (
	"testing"

	"github.com/GrayCodeAI/hawk/tool"
)

func TestParseToolListFromCLI(t *testing.T) {
	got := parseToolListFromCLI([]string{"Bash(git diff:*) Edit,Read", "mcp__server__tool"})
	want := []string{"Bash(git diff:*)", "Edit", "Read", "mcp__server__tool"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}

func TestFilterAvailableTools(t *testing.T) {
	all := []tool.Tool{tool.BashTool{}, tool.FileReadTool{}, tool.FileWriteTool{}, tool.FileEditTool{}}

	filtered, err := filterAvailableTools(all, true, []string{"Bash", "Edit"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if names(filtered) != "Bash,Edit" {
		t.Fatalf("got %q", names(filtered))
	}

	filtered, err = filterAvailableTools(all, true, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(filtered) != 0 {
		t.Fatalf("expected --tools empty to disable tools, got %q", names(filtered))
	}
}

func TestDisallowedBareToolRemovesToolButPatternDoesNot(t *testing.T) {
	all := []tool.Tool{tool.BashTool{}, tool.FileReadTool{}, tool.FileWriteTool{}}

	filtered, err := filterAvailableTools(all, false, nil, []string{"Bash(git:*)", "Write"})
	if err != nil {
		t.Fatal(err)
	}
	if names(filtered) != "Bash,Read" {
		t.Fatalf("got %q", names(filtered))
	}
}

func TestFilterAvailableToolsAcceptsAliases(t *testing.T) {
	all := []tool.Tool{tool.AgentTool{}, tool.FileReadTool{}}
	filtered, err := filterAvailableTools(all, true, []string{"Task", "file_read"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if names(filtered) != "Agent,Read" {
		t.Fatalf("got %q", names(filtered))
	}
}

func TestPromptFromStreamJSON(t *testing.T) {
	input := `{"type":"user","message":{"content":"hello"}}` + "\n" +
		`{"type":"assistant","content":"ignored"}` + "\n" +
		`{"type":"user_message","content":"world"}`
	got, err := promptFromStreamJSON([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	if got != "hello\nworld" {
		t.Fatalf("got %q", got)
	}
}

func TestPromptFromStreamJSONInvalid(t *testing.T) {
	if _, err := promptFromStreamJSON([]byte(`not-json`)); err == nil {
		t.Fatal("expected invalid JSON error")
	}
}

func names(tools []tool.Tool) string {
	out := ""
	for i, t := range tools {
		if i > 0 {
			out += ","
		}
		out += t.Name()
	}
	return out
}
