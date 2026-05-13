package permissions

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParsRuleLine_Valid(t *testing.T) {
	tests := []struct {
		line    string
		want    Rule
	}{
		{
			line: `allow Read *`,
			want: Rule{Tool: "Read", Pattern: "*", Action: ActionAllow},
		},
		{
			line: `deny Bash "rm -rf /*"`,
			want: Rule{Tool: "Bash", Pattern: "rm -rf /*", Action: ActionDeny},
		},
		{
			line: `ask Bash "git push*"`,
			want: Rule{Tool: "Bash", Pattern: "git push*", Action: ActionAsk},
		},
		{
			line: `allow Edit *.go`,
			want: Rule{Tool: "Edit", Pattern: "*.go", Action: ActionAllow},
		},
		{
			line: `ALLOW Bash "go test*"`,
			want: Rule{Tool: "Bash", Pattern: "go test*", Action: ActionAllow},
		},
		{
			line: `deny Write /etc/*`,
			want: Rule{Tool: "Write", Pattern: "/etc/*", Action: ActionDeny},
		},
	}

	for _, tt := range tests {
		t.Run(tt.line, func(t *testing.T) {
			got, err := ParseRuleLine(tt.line)
			if err != nil {
				t.Fatalf("ParseRuleLine(%q) error: %v", tt.line, err)
			}
			if got.Tool != tt.want.Tool {
				t.Errorf("Tool = %q, want %q", got.Tool, tt.want.Tool)
			}
			if got.Pattern != tt.want.Pattern {
				t.Errorf("Pattern = %q, want %q", got.Pattern, tt.want.Pattern)
			}
			if got.Action != tt.want.Action {
				t.Errorf("Action = %q, want %q", got.Action, tt.want.Action)
			}
		})
	}
}

func TestParseRuleLine_Invalid(t *testing.T) {
	invalid := []string{
		"",
		"allow",
		"allow Bash",
		"invalid Bash *",
		"maybe Read *",
	}

	for _, line := range invalid {
		t.Run(line, func(t *testing.T) {
			_, err := ParseRuleLine(line)
			if err == nil {
				t.Errorf("ParseRuleLine(%q) expected error, got nil", line)
			}
		})
	}
}

func TestLoadFromFile(t *testing.T) {
	content := `# Permission rules for hawk
# Read-only tools are always allowed
allow Read *
allow Grep *
allow Glob *
allow LS *

# Safe bash commands
allow Bash "go test*"
allow Bash "go build*"
allow Bash "git status*"

# Dangerous commands
deny Bash "rm -rf /*"
deny Bash "sudo *"

# File edits
allow Edit *.go
ask   Edit *
`

	dir := t.TempDir()
	path := filepath.Join(dir, "rules")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	rs := NewRuleSet()
	if err := rs.LoadFromFile(path); err != nil {
		t.Fatalf("LoadFromFile error: %v", err)
	}

	if len(rs.Rules) != 11 {
		t.Fatalf("expected 11 rules, got %d", len(rs.Rules))
	}

	// Verify first rule.
	if rs.Rules[0].Action != ActionAllow || rs.Rules[0].Tool != "Read" || rs.Rules[0].Pattern != "*" {
		t.Errorf("first rule = %+v, want allow Read *", rs.Rules[0])
	}

	// Verify a quoted rule.
	if rs.Rules[4].Pattern != "go test*" {
		t.Errorf("rule 4 pattern = %q, want %q", rs.Rules[4].Pattern, "go test*")
	}

	// Verify deny rule.
	if rs.Rules[7].Action != ActionDeny || rs.Rules[7].Pattern != "rm -rf /*" {
		t.Errorf("rule 7 = %+v, want deny Bash rm -rf /*", rs.Rules[7])
	}
}

func TestLoadFromFile_InvalidLine(t *testing.T) {
	content := `allow Read *
badaction Tool *
`
	dir := t.TempDir()
	path := filepath.Join(dir, "rules")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	rs := NewRuleSet()
	err := rs.LoadFromFile(path)
	if err == nil {
		t.Fatal("expected error for invalid line, got nil")
	}
}

func TestEvaluate_BashCommands(t *testing.T) {
	rs := NewRuleSet()
	rs.Rules = []Rule{
		{Tool: "Bash", Pattern: "go test*", Action: ActionAllow},
		{Tool: "Bash", Pattern: "go build*", Action: ActionAllow},
		{Tool: "Bash", Pattern: "rm -rf /*", Action: ActionDeny},
		{Tool: "Bash", Pattern: "sudo *", Action: ActionDeny},
	}

	tests := []struct {
		command string
		want    Action
	}{
		{"go test ./...", ActionAllow},
		{"go test -race ./pkg/...", ActionAllow},
		{"go build -o bin/hawk .", ActionAllow},
		{"rm -rf /etc", ActionDeny},
		{"sudo apt install vim", ActionDeny},
		{"curl http://example.com", ActionAsk}, // no rule matches
	}

	for _, tt := range tests {
		t.Run(tt.command, func(t *testing.T) {
			args := map[string]interface{}{"command": tt.command}
			got := rs.Evaluate("Bash", args)
			if got != tt.want {
				t.Errorf("Evaluate(Bash, %q) = %q, want %q", tt.command, got, tt.want)
			}
		})
	}
}

func TestEvaluate_FileTools(t *testing.T) {
	rs := NewRuleSet()
	rs.Rules = []Rule{
		{Tool: "Edit", Pattern: "*.go", Action: ActionAllow},
		{Tool: "Edit", Pattern: "*.ts", Action: ActionAllow},
		{Tool: "Edit", Pattern: "*", Action: ActionAsk},
		{Tool: "Write", Pattern: "*.env", Action: ActionDeny},
		{Tool: "Write", Pattern: "/etc/*", Action: ActionDeny},
		{Tool: "Read", Pattern: "*", Action: ActionAllow},
	}

	tests := []struct {
		tool string
		path string
		want Action
	}{
		{"Edit", "main.go", ActionAllow},
		{"Edit", "handler.ts", ActionAllow},
		{"Edit", "config.yaml", ActionAsk},
		{"Write", ".env", ActionDeny},
		{"Write", "/etc/passwd", ActionDeny},
		{"Write", "output.txt", ActionAsk}, // no Write rule matches
		{"Read", "anything.txt", ActionAllow},
	}

	for _, tt := range tests {
		t.Run(tt.tool+"_"+tt.path, func(t *testing.T) {
			args := map[string]interface{}{"file_path": tt.path}
			got := rs.Evaluate(tt.tool, args)
			if got != tt.want {
				t.Errorf("Evaluate(%s, %q) = %q, want %q", tt.tool, tt.path, got, tt.want)
			}
		})
	}
}

func TestEvaluate_FirstMatchWins(t *testing.T) {
	rs := NewRuleSet()
	rs.Rules = []Rule{
		{Tool: "Bash", Pattern: "git push*", Action: ActionAllow},  // more specific first
		{Tool: "Bash", Pattern: "git *", Action: ActionDeny},       // broader second
	}

	args := map[string]interface{}{"command": "git push origin main"}
	got := rs.Evaluate("Bash", args)
	if got != ActionAllow {
		t.Errorf("first-match-wins: got %q, want %q", got, ActionAllow)
	}

	// "git status" should match the second (deny) rule.
	args = map[string]interface{}{"command": "git status"}
	got = rs.Evaluate("Bash", args)
	if got != ActionDeny {
		t.Errorf("second rule match: got %q, want %q", got, ActionDeny)
	}
}

func TestEvaluate_DefaultAsk(t *testing.T) {
	rs := NewRuleSet()
	rs.Rules = []Rule{
		{Tool: "Bash", Pattern: "go test*", Action: ActionAllow},
	}

	// Completely unmatched tool.
	args := map[string]interface{}{"command": "docker build ."}
	got := rs.Evaluate("Bash", args)
	if got != ActionAsk {
		t.Errorf("default action: got %q, want %q", got, ActionAsk)
	}

	// Unmatched tool name.
	got = rs.Evaluate("UnknownTool", map[string]interface{}{"something": "value"})
	if got != ActionAsk {
		t.Errorf("unknown tool default: got %q, want %q", got, ActionAsk)
	}
}

func TestEvaluate_WildcardTool(t *testing.T) {
	rs := NewRuleSet()
	rs.Rules = []Rule{
		{Tool: "*", Pattern: "*", Action: ActionAllow},
	}

	// Any tool should match.
	tests := []struct {
		tool string
		args map[string]interface{}
	}{
		{"Bash", map[string]interface{}{"command": "echo hello"}},
		{"Read", map[string]interface{}{"file_path": "/tmp/test.txt"}},
		{"Edit", map[string]interface{}{"file_path": "main.go"}},
		{"UnknownTool", map[string]interface{}{}},
	}

	for _, tt := range tests {
		t.Run(tt.tool, func(t *testing.T) {
			got := rs.Evaluate(tt.tool, tt.args)
			if got != ActionAllow {
				t.Errorf("wildcard tool: Evaluate(%s) = %q, want %q", tt.tool, got, ActionAllow)
			}
		})
	}
}

func TestSaveAndLoadRoundTrip(t *testing.T) {
	original := NewRuleSet()
	original.Rules = []Rule{
		{Tool: "Read", Pattern: "*", Action: ActionAllow},
		{Tool: "Bash", Pattern: "go test*", Action: ActionAllow},
		{Tool: "Bash", Pattern: "rm -rf /*", Action: ActionDeny},
		{Tool: "Edit", Pattern: "*.go", Action: ActionAllow},
		{Tool: "Edit", Pattern: "*", Action: ActionAsk},
	}

	dir := t.TempDir()
	path := filepath.Join(dir, ".hawk", "rules")

	if err := original.SaveToFile(path); err != nil {
		t.Fatalf("SaveToFile error: %v", err)
	}

	// Verify file was created.
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("file not created: %v", err)
	}

	// Load it back.
	loaded := NewRuleSet()
	if err := loaded.LoadFromFile(path); err != nil {
		t.Fatalf("LoadFromFile error: %v", err)
	}

	if len(loaded.Rules) != len(original.Rules) {
		t.Fatalf("rule count: got %d, want %d", len(loaded.Rules), len(original.Rules))
	}

	for i, want := range original.Rules {
		got := loaded.Rules[i]
		if got.Action != want.Action || got.Tool != want.Tool || got.Pattern != want.Pattern {
			t.Errorf("rule %d: got %+v, want %+v", i, got, want)
		}
	}
}

func TestAddRule(t *testing.T) {
	rs := NewRuleSet()
	if len(rs.Rules) != 0 {
		t.Fatalf("new ruleset should be empty, got %d rules", len(rs.Rules))
	}

	rs.AddRule(Rule{Tool: "Bash", Pattern: "echo *", Action: ActionAllow})
	rs.AddRule(Rule{Tool: "Write", Pattern: "*.tmp", Action: ActionDeny})

	if len(rs.Rules) != 2 {
		t.Fatalf("expected 2 rules, got %d", len(rs.Rules))
	}
	if rs.Rules[0].Tool != "Bash" {
		t.Errorf("rule 0 tool = %q, want Bash", rs.Rules[0].Tool)
	}
	if rs.Rules[1].Tool != "Write" {
		t.Errorf("rule 1 tool = %q, want Write", rs.Rules[1].Tool)
	}
}

func TestRemoveRule(t *testing.T) {
	rs := NewRuleSet()
	rs.Rules = []Rule{
		{Tool: "Read", Pattern: "*", Action: ActionAllow},
		{Tool: "Bash", Pattern: "go test*", Action: ActionAllow},
		{Tool: "Write", Pattern: "*.env", Action: ActionDeny},
	}

	// Remove middle rule.
	if err := rs.RemoveRule(1); err != nil {
		t.Fatalf("RemoveRule(1) error: %v", err)
	}
	if len(rs.Rules) != 2 {
		t.Fatalf("expected 2 rules after removal, got %d", len(rs.Rules))
	}
	if rs.Rules[0].Tool != "Read" || rs.Rules[1].Tool != "Write" {
		t.Errorf("unexpected rules after removal: %+v", rs.Rules)
	}

	// Invalid index.
	if err := rs.RemoveRule(5); err == nil {
		t.Error("RemoveRule(5) expected error for out-of-range index")
	}
	if err := rs.RemoveRule(-1); err == nil {
		t.Error("RemoveRule(-1) expected error for negative index")
	}
}

func TestEvaluate_CaseInsensitiveTool(t *testing.T) {
	rs := NewRuleSet()
	rs.Rules = []Rule{
		{Tool: "Bash", Pattern: "*", Action: ActionAllow},
	}

	// Tool matching should be case-insensitive.
	args := map[string]interface{}{"command": "echo hi"}
	got := rs.Evaluate("bash", args)
	if got != ActionAllow {
		t.Errorf("case insensitive tool: got %q, want %q", got, ActionAllow)
	}

	got = rs.Evaluate("BASH", args)
	if got != ActionAllow {
		t.Errorf("case insensitive tool uppercase: got %q, want %q", got, ActionAllow)
	}
}

func TestMatchPattern_GlobPaths(t *testing.T) {
	tests := []struct {
		pattern string
		subject string
		want    bool
	}{
		{"*", "anything", true},
		{"*.go", "main.go", true},
		{"*.go", "main.ts", false},
		{"/tmp/*", "/tmp/file.txt", true},
		{"/tmp/*", "/etc/file.txt", false},
		{"go test*", "go test ./...", true},
		{"go test*", "go build .", false},
		{"*.env", ".env", true},
		{"*.env", "production.env", true},
		{"/etc/*", "/etc/passwd", true},
		{"/etc/*", "/home/user", false},
	}

	for _, tt := range tests {
		t.Run(tt.pattern+"_"+tt.subject, func(t *testing.T) {
			got := matchPattern(tt.pattern, tt.subject)
			if got != tt.want {
				t.Errorf("matchPattern(%q, %q) = %v, want %v", tt.pattern, tt.subject, got, tt.want)
			}
		})
	}
}
