package types_test

import (
	"sort"
	"testing"

	"github.com/GrayCodeAI/hawk/shared/types"
)

func TestFindingSlice_SortBySeverityThenConfidence(t *testing.T) {
	t.Parallel()
	findings := types.FindingSlice{
		{ID: "a", Severity: types.SeverityLow, Confidence: 0.9},
		{ID: "b", Severity: types.SeverityCritical, Confidence: 0.7},
		{ID: "c", Severity: types.SeverityCritical, Confidence: 0.95},
		{ID: "d", Severity: types.SeverityMedium, Confidence: 0.8},
	}
	sort.Sort(findings)

	if findings[0].ID != "c" {
		t.Fatalf("expected first finding to be c (critical, 0.95), got %s", findings[0].ID)
	}
	if findings[1].ID != "b" {
		t.Fatalf("expected second finding to be b (critical, 0.7), got %s", findings[1].ID)
	}
	if findings[2].ID != "d" {
		t.Fatalf("expected third finding to be d (medium), got %s", findings[2].ID)
	}
	if findings[3].ID != "a" {
		t.Fatalf("expected fourth finding to be a (low), got %s", findings[3].ID)
	}
}

func TestFilterBySource(t *testing.T) {
	t.Parallel()
	findings := types.FindingSlice{
		{ID: "1", Source: "sight"},
		{ID: "2", Source: "inspect"},
		{ID: "3", Source: "sight"},
		{ID: "4", Source: "manual"},
	}

	got := findings.FilterBySource("sight")
	if len(got) != 2 {
		t.Fatalf("expected 2 sight findings, got %d", len(got))
	}
	for _, f := range got {
		if f.Source != "sight" {
			t.Fatalf("expected source sight, got %s", f.Source)
		}
	}
}

func TestFilterBySeverity(t *testing.T) {
	t.Parallel()
	findings := types.FindingSlice{
		{ID: "1", Severity: types.SeverityInfo},
		{ID: "2", Severity: types.SeverityHigh},
		{ID: "3", Severity: types.SeverityCritical},
		{ID: "4", Severity: types.SeverityLow},
		{ID: "5", Severity: types.SeverityMedium},
	}

	got := findings.FilterBySeverity(types.SeverityHigh)
	if len(got) != 2 {
		t.Fatalf("expected 2 findings >= high, got %d", len(got))
	}
}

func TestFilterByConfidence(t *testing.T) {
	t.Parallel()
	findings := types.FindingSlice{
		{ID: "1", Confidence: 0.9},
		{ID: "2", Confidence: 0.5},
		{ID: "3", Confidence: 0.7},
		{ID: "4", Confidence: 0.3},
	}

	got := findings.FilterByConfidence(0.7)
	if len(got) != 2 {
		t.Fatalf("expected 2 findings with confidence >= 0.7, got %d", len(got))
	}
	for _, f := range got {
		if f.Confidence < 0.7 {
			t.Fatalf("expected confidence >= 0.7, got %f", f.Confidence)
		}
	}
}

func TestByFile(t *testing.T) {
	t.Parallel()
	findings := types.FindingSlice{
		{ID: "1", File: "a.go"},
		{ID: "2", File: "b.go"},
		{ID: "3", File: "a.go"},
		{ID: "4", File: ""},
	}

	grouped := findings.ByFile()
	if len(grouped["a.go"]) != 2 {
		t.Fatalf("expected 2 findings in a.go, got %d", len(grouped["a.go"]))
	}
	if len(grouped["b.go"]) != 1 {
		t.Fatalf("expected 1 finding in b.go, got %d", len(grouped["b.go"]))
	}
	if len(grouped[""]) != 1 {
		t.Fatalf("expected 1 finding with empty file, got %d", len(grouped[""]))
	}
}

func TestSummary(t *testing.T) {
	t.Parallel()
	findings := types.FindingSlice{
		{ID: "1", Source: "sight", Severity: types.SeverityHigh, Confidence: 0.8},
		{ID: "2", Source: "sight", Severity: types.SeverityLow, Confidence: 0.6},
		{ID: "3", Source: "inspect", Severity: types.SeverityHigh, Confidence: 1.0},
	}

	summary := findings.Summary()
	if summary.Total != 3 {
		t.Fatalf("expected total 3, got %d", summary.Total)
	}
	if summary.BySource["sight"] != 2 {
		t.Fatalf("expected 2 sight findings, got %d", summary.BySource["sight"])
	}
	if summary.BySource["inspect"] != 1 {
		t.Fatalf("expected 1 inspect finding, got %d", summary.BySource["inspect"])
	}
	if summary.BySeverity["high"] != 2 {
		t.Fatalf("expected 2 high severity, got %d", summary.BySeverity["high"])
	}
	if summary.BySeverity["low"] != 1 {
		t.Fatalf("expected 1 low severity, got %d", summary.BySeverity["low"])
	}
	// avg confidence = (0.8 + 0.6 + 1.0) / 3 = 0.8
	if summary.AvgConfidence < 0.79 || summary.AvgConfidence > 0.81 {
		t.Fatalf("expected avg confidence ~0.8, got %f", summary.AvgConfidence)
	}
}

func TestFindingFromSight(t *testing.T) {
	t.Parallel()
	f := types.FindingFromSight("sql-injection", "db.go", 42, "unparameterized query", "CWE-89", types.SeverityCritical, 0.95)

	if f.ID != "sight:sql-injection:db.go:42" {
		t.Fatalf("unexpected ID: %s", f.ID)
	}
	if f.Source != "sight" {
		t.Fatalf("expected source sight, got %s", f.Source)
	}
	if f.Concern != "sql-injection" {
		t.Fatalf("expected concern sql-injection, got %s", f.Concern)
	}
	if f.Severity != types.SeverityCritical {
		t.Fatalf("expected severity critical, got %v", f.Severity)
	}
	if f.File != "db.go" {
		t.Fatalf("expected file db.go, got %s", f.File)
	}
	if f.Line != 42 {
		t.Fatalf("expected line 42, got %d", f.Line)
	}
	if f.CWE != "CWE-89" {
		t.Fatalf("expected CWE-89, got %s", f.CWE)
	}
	if f.Confidence != 0.95 {
		t.Fatalf("expected confidence 0.95, got %f", f.Confidence)
	}
	if f.CreatedAt.IsZero() {
		t.Fatal("expected CreatedAt to be set")
	}
}

func TestFindingFromInspect(t *testing.T) {
	t.Parallel()
	tags := []string{"security", "injection"}
	f := types.FindingFromInspect("broken-auth", "https://example.com/api", "missing auth header", types.SeverityHigh, tags)

	if f.ID != "inspect:broken-auth:https://example.com/api" {
		t.Fatalf("unexpected ID: %s", f.ID)
	}
	if f.Source != "inspect" {
		t.Fatalf("expected source inspect, got %s", f.Source)
	}
	if f.URL != "https://example.com/api" {
		t.Fatalf("expected url, got %s", f.URL)
	}
	if f.Severity != types.SeverityHigh {
		t.Fatalf("expected severity high, got %v", f.Severity)
	}
	if len(f.Tags) != 2 || f.Tags[0] != "security" {
		t.Fatalf("unexpected tags: %v", f.Tags)
	}
	if f.CreatedAt.IsZero() {
		t.Fatal("expected CreatedAt to be set")
	}
}

func TestEmptySliceHandling(t *testing.T) {
	t.Parallel()
	var empty types.FindingSlice

	if empty.Len() != 0 {
		t.Fatalf("expected len 0, got %d", empty.Len())
	}
	if len(empty.FilterBySource("x")) != 0 {
		t.Fatal("expected empty filter result")
	}
	if len(empty.FilterBySeverity(types.SeverityCritical)) != 0 {
		t.Fatal("expected empty filter result")
	}
	if len(empty.FilterByConfidence(0.5)) != 0 {
		t.Fatal("expected empty filter result")
	}
	if len(empty.ByFile()) != 0 {
		t.Fatal("expected empty ByFile map")
	}
	summary := empty.Summary()
	if summary.Total != 0 {
		t.Fatalf("expected total 0, got %d", summary.Total)
	}
	if summary.AvgConfidence != 0.0 {
		t.Fatalf("expected avg confidence 0, got %f", summary.AvgConfidence)
	}
}
