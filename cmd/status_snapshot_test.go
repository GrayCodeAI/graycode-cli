package cmd

import (
	"strings"
	"testing"
)

func TestFormatStatusSnapshot(t *testing.T) {
	snapshot := buildStatusSnapshot()
	formatted := formatStatusSnapshot(snapshot)
	for _, expected := range []string{"Hawk status", "Schema: 1", "Secrets redacted: true"} {
		if !strings.Contains(formatted, expected) {
			t.Errorf("status output missing %q: %s", expected, formatted)
		}
	}
}
