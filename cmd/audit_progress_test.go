package cmd

import "testing"

// TestAuditProgressEnabled guards the JSON-purity contract: per-session
// progress must never render when output is JSON (it would corrupt the
// payload) or when stdout is piped (it would spam the report).
func TestAuditProgressEnabled(t *testing.T) {
	cases := []struct {
		format string
		tty    bool
		want   bool
	}{
		{"json", true, false},  // JSON output must stay pure even on a TTY
		{"json", false, false}, // JSON + piped: never
		{"text", false, false}, // piped text: no per-session spam
		{"text", true, true},   // interactive text: animate
		{"", true, true},       // default format on a TTY: animate
		{"", false, false},     // default format piped: no spam
	}
	for _, c := range cases {
		if got := auditProgressEnabled(c.format, c.tty); got != c.want {
			t.Errorf("auditProgressEnabled(%q, %v) = %v, want %v", c.format, c.tty, got, c.want)
		}
	}
}

// TestAuditSeverityColor verifies severity maps to the expected semantic
// theme color so the report's severity column reads consistently.
func TestAuditSeverityColor(t *testing.T) {
	if auditSeverityColor("high") != errorCoral {
		t.Error("high should map to errorCoral")
	}
	if auditSeverityColor("critical") != errorCoral {
		t.Error("critical should map to errorCoral")
	}
	if auditSeverityColor("medium") != warnAmber {
		t.Error("medium should map to warnAmber")
	}
	if auditSeverityColor("low") != infoSky {
		t.Error("low should map to infoSky")
	}
	if auditSeverityColor("info") != infoSky {
		t.Error("info should map to infoSky")
	}
}
