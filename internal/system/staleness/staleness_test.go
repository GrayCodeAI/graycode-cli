package staleness

import (
	"strings"
	"testing"
	"time"
)

func TestDetector_RecordRuleUsed(t *testing.T) {
	d := NewDetector()
	now := time.Now()
	d.RecordRuleUsed("rule-1", now)

	usages := d.AllUsages()
	if usages["rule-1"] == nil {
		t.Fatal("expected rule-1 to be tracked")
	}
	if usages["rule-1"].UseCount != 1 {
		t.Errorf("expected use count 1, got %d", usages["rule-1"].UseCount)
	}
	if !usages["rule-1"].LastUsed.Equal(now) {
		t.Error("expected LastUsed to match")
	}
}

func TestDetector_RecordRuleUsed_UpdatesTimestamp(t *testing.T) {
	d := NewDetector()
	t1 := time.Now().Add(-24 * time.Hour)
	t2 := time.Now()

	d.RecordRuleUsed("rule-1", t1)
	d.RecordRuleUsed("rule-1", t2)

	usages := d.AllUsages()
	if usages["rule-1"].UseCount != 2 {
		t.Errorf("expected use count 2, got %d", usages["rule-1"].UseCount)
	}
	if !usages["rule-1"].LastUsed.Equal(t2) {
		t.Error("expected LastUsed to be updated to later timestamp")
	}
}

func TestDetector_RecordContradiction(t *testing.T) {
	d := NewDetector()
	d.RecordRuleUsed("rule-1", time.Now())
	d.RecordContradiction("rule-1", "user uses snake_case instead of camelCase")

	// The rule was just used so it won't be stale by time alone,
	// but with only 1 contradiction it won't appear either.
	// Add more contradictions.
	d.RecordContradiction("rule-1", "user uses snake_case again")
	d.RecordContradiction("rule-1", "user still uses snake_case")

	stale := d.CheckStaleness(24 * time.Hour * 365) // High threshold so time isn't the factor.
	found := false
	for _, s := range stale {
		if s.ID == "rule-1" {
			found = true
			if s.ContradictionCount != 3 {
				t.Errorf("expected 3 contradictions, got %d", s.ContradictionCount)
			}
		}
	}
	if !found {
		t.Error("expected rule-1 to appear in stale rules due to contradictions")
	}
}

func TestDetector_CheckStaleness_TimeThreshold(t *testing.T) {
	d := NewDetector()

	// Record a rule used 10 days ago.
	d.RecordRuleUsed("old-rule", time.Now().Add(-10*24*time.Hour))
	d.RecordRulePath("old-rule", ".hawk/rules/old-rule.md")

	// Record a rule used just now.
	d.RecordRuleUsed("fresh-rule", time.Now())

	// Check with 7-day threshold.
	stale := d.CheckStaleness(7 * 24 * time.Hour)

	if len(stale) != 1 {
		t.Fatalf("expected 1 stale rule, got %d", len(stale))
	}
	if stale[0].ID != "old-rule" {
		t.Errorf("expected old-rule to be stale, got %q", stale[0].ID)
	}
	if stale[0].Path != ".hawk/rules/old-rule.md" {
		t.Errorf("expected path, got %q", stale[0].Path)
	}
	if stale[0].DaysSinceUsed < 9 || stale[0].DaysSinceUsed > 11 {
		t.Errorf("expected ~10 days since used, got %d", stale[0].DaysSinceUsed)
	}
}

func TestDetector_CheckStaleness_Empty(t *testing.T) {
	d := NewDetector()
	stale := d.CheckStaleness(24 * time.Hour)
	if len(stale) != 0 {
		t.Errorf("expected no stale rules for empty detector, got %d", len(stale))
	}
}

func TestFormatReport_Empty(t *testing.T) {
	report := FormatReport(nil)
	if !strings.Contains(report, "No stale rules") {
		t.Error("expected 'No stale rules' in empty report")
	}
}

func TestFormatReport_WithRules(t *testing.T) {
	stale := []StaleRule{
		{
			ID:            "naming-rule",
			Path:          ".hawk/rules/naming.md",
			LastUsed:      time.Now().Add(-15 * 24 * time.Hour),
			DaysSinceUsed: 15,
		},
		{
			ID:                 "error-rule",
			Path:               ".hawk/rules/errors.md",
			LastUsed:           time.Now().Add(-5 * 24 * time.Hour),
			DaysSinceUsed:      5,
			ContradictionCount: 4,
			Contradictions: []Contradiction{
				{RuleID: "error-rule", UserBehavior: "user wraps errors", Timestamp: time.Now()},
			},
		},
	}

	report := FormatReport(stale)
	if !strings.Contains(report, "naming-rule") {
		t.Error("expected naming-rule in report")
	}
	if !strings.Contains(report, "error-rule") {
		t.Error("expected error-rule in report")
	}
	if !strings.Contains(report, "Contradictions: 4") {
		t.Error("expected contradiction count in report")
	}
}

func TestRecommendAction(t *testing.T) {
	tests := []struct {
		rule StaleRule
		want string
	}{
		{StaleRule{ContradictionCount: 5}, "REMOVE"},
		{StaleRule{ContradictionCount: 3}, "REVIEW"},
		{StaleRule{DaysSinceUsed: 35}, "REMOVE"},
		{StaleRule{DaysSinceUsed: 20}, "REVIEW"},
		{StaleRule{DaysSinceUsed: 5}, "MONITOR"},
	}

	for _, tt := range tests {
		action := RecommendAction(tt.rule)
		if !strings.HasPrefix(action, tt.want) {
			t.Errorf("RecommendAction(%+v) = %q, want prefix %q", tt.rule, action, tt.want)
		}
	}
}
