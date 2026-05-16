package sarif

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestBuilder_BasicRoundtrip(t *testing.T) {
	t.Parallel()

	b := New(Tool{
		Name:           "mytool",
		Version:        "1.2.3",
		InformationURI: "https://example.com",
	})

	b.AddRule(Rule{
		ID:               "mytool/sql-injection",
		Name:             "sql-injection",
		ShortDescription: "Possible SQL injection sink",
		Severity:         SeverityError,
		Tags:             []string{"security"},
	})

	b.AddResult(Result{
		RuleID:   "mytool/sql-injection",
		Severity: SeverityError,
		Message:  "concatenated user input",
		URI:      "src/handlers.go",
		Region:   &Region{StartLine: 42},
		Taxa:     []TaxaRef{{ID: "CWE-89", Component: "CWE"}},
		Fix:      "Use parameterised queries.",
	})

	out, err := b.JSON()
	if err != nil {
		t.Fatalf("JSON: %v", err)
	}

	// Round-trip through map[string]any to assert the key fields.
	var got map[string]any
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if got["version"] != specVersion {
		t.Errorf("version = %v, want %s", got["version"], specVersion)
	}

	runs, ok := got["runs"].([]any)
	if !ok || len(runs) != 1 {
		t.Fatalf("runs malformed: %v", got["runs"])
	}

	run := runs[0].(map[string]any)
	tool := run["tool"].(map[string]any)["driver"].(map[string]any)
	if tool["name"] != "mytool" {
		t.Errorf("tool.name = %v, want mytool", tool["name"])
	}
	if tool["version"] != "1.2.3" {
		t.Errorf("tool.version = %v, want 1.2.3", tool["version"])
	}

	results := run["results"].([]any)
	if len(results) != 1 {
		t.Fatalf("results: got %d, want 1", len(results))
	}
	res := results[0].(map[string]any)
	if res["level"] != "error" {
		t.Errorf("level = %v, want error", res["level"])
	}
	if res["ruleId"] != "mytool/sql-injection" {
		t.Errorf("ruleId = %v", res["ruleId"])
	}

	// EndLine should be populated from StartLine.
	loc := res["locations"].([]any)[0].(map[string]any)
	region := loc["physicalLocation"].(map[string]any)["region"].(map[string]any)
	if region["startLine"] != float64(42) || region["endLine"] != float64(42) {
		t.Errorf("region = %v, want startLine=42 endLine=42", region)
	}

	// Taxa.
	taxa := res["taxa"].([]any)
	if len(taxa) != 1 {
		t.Fatalf("taxa: got %d", len(taxa))
	}
	if taxa[0].(map[string]any)["id"] != "CWE-89" {
		t.Errorf("taxa[0].id = %v", taxa[0])
	}

	// Fix description.
	fixes := res["fixes"].([]any)
	if len(fixes) != 1 {
		t.Fatalf("fixes: got %d", len(fixes))
	}
}

func TestBuilder_DedupRules(t *testing.T) {
	t.Parallel()

	b := New(Tool{Name: "x", Version: "1"})
	b.AddRule(Rule{ID: "r1", Severity: SeverityNote})
	b.AddRule(Rule{ID: "r1", Severity: SeverityError}) // duplicate; should be ignored
	b.AddRule(Rule{ID: "r2", Severity: SeverityWarning})

	if len(b.rules) != 2 {
		t.Errorf("rules: got %d, want 2", len(b.rules))
	}
	// First insertion wins.
	if b.rules[0].Severity != SeverityNote {
		t.Errorf("rules[0].Severity = %v, want SeverityNote", b.rules[0].Severity)
	}
}

func TestBuilder_EmptyResultURI(t *testing.T) {
	t.Parallel()

	b := New(Tool{Name: "x", Version: "1"})
	b.AddRule(Rule{ID: "r1", Severity: SeverityWarning})
	b.AddResult(Result{
		RuleID:   "r1",
		Severity: SeverityWarning,
		Message:  "no location",
	})

	out := b.String()
	if !strings.Contains(out, `"ruleId": "r1"`) {
		t.Errorf("missing ruleId in output:\n%s", out)
	}
	if strings.Contains(out, `"locations"`) {
		t.Errorf("expected no locations field for empty URI, got:\n%s", out)
	}
}

func TestSeverity_Level(t *testing.T) {
	t.Parallel()

	cases := []struct {
		sev  Severity
		want string
	}{
		{SeverityError, "error"},
		{SeverityWarning, "warning"},
		{SeverityNote, "note"},
		{SeverityNone, "none"},
	}
	for _, tc := range cases {
		if got := tc.sev.level(); got != tc.want {
			t.Errorf("Severity(%d).level() = %s, want %s", tc.sev, got, tc.want)
		}
	}
}
