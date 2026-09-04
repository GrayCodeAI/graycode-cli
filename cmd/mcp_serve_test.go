package cmd

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestMCPConfigEmitsValidServerBlock(t *testing.T) {
	var buf bytes.Buffer
	c := &cobra.Command{}
	c.SetOut(&buf)
	c.SetErr(&buf)

	if err := runMCPConfig(c, nil); err != nil {
		t.Fatalf("runMCPConfig: %v", err)
	}

	// Output should be a single JSON object (no leading comment lines without --write).
	var cfg struct {
		MCPServers map[string]struct {
			Command string   `json:"command"`
			Args    []string `json:"args"`
		} `json:"mcpServers"`
	}
	if err := json.Unmarshal(buf.Bytes(), &cfg); err != nil {
		t.Fatalf("emitted config is not valid JSON: %v\n%s", err, buf.String())
	}

	graycode, ok := cfg.MCPServers["graycode"]
	if !ok {
		t.Fatal(`config missing "graycode" server entry`)
	}
	if graycode.Command == "" {
		t.Error("graycode server entry missing command")
	}
	if len(graycode.Args) != 2 || graycode.Args[0] != "mcp" || graycode.Args[1] != "serve" {
		t.Errorf(`args = %v, want ["mcp" "serve"]`, graycode.Args)
	}
}

func TestMCPConfigWriteAddsPathHints(t *testing.T) {
	orig := mcpConfigWrite
	defer func() { mcpConfigWrite = orig }()
	mcpConfigWrite = true

	var buf bytes.Buffer
	c := &cobra.Command{}
	c.SetOut(&buf)
	c.SetErr(&buf)

	if err := runMCPConfig(c, nil); err != nil {
		t.Fatalf("runMCPConfig: %v", err)
	}
	out := buf.String()
	for _, want := range []string{"claude_desktop_config.json", "cursor", "windsurf"} {
		if !strings.Contains(strings.ToLower(out), strings.ToLower(want)) {
			t.Errorf("--write output missing hint %q\n%s", want, out)
		}
	}
	// The JSON block must still be present and parseable after the comments.
	brace := strings.Index(out, "{")
	if brace < 0 {
		t.Fatal("no JSON block found in --write output")
	}
	var anyObj map[string]any
	if err := json.Unmarshal([]byte(out[brace:]), &anyObj); err != nil {
		t.Fatalf("JSON block after hints is invalid: %v", err)
	}
}
