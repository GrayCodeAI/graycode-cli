package memory

import "testing"

func TestIsTestCommand(t *testing.T) {
	t.Parallel()
	tests := []struct {
		cmd  string
		want bool
	}{
		{"go test ./...", true},
		{"pytest tests/", true},
		{"npm test", true},
		{"make test", true},
		{"cargo test", true},
		{"echo hello", false},
		{"git commit -m 'test'", false},
		{"ls -la", false},
	}
	for _, tt := range tests {
		t.Run(tt.cmd, func(t *testing.T) {
			t.Parallel()
			got := isTestCommand(tt.cmd)
			if got != tt.want {
				t.Errorf("isTestCommand(%q) = %v, want %v", tt.cmd, got, tt.want)
			}
		})
	}
}

func TestCaptureMetrics_Inc(t *testing.T) {
	t.Parallel()
	m := &CaptureMetrics{}
	m.inc("convention")
	m.inc("convention")
	m.inc("decision")

	if m.ConventionsOut != 2 {
		t.Errorf("ConventionsOut = %d, want 2", m.ConventionsOut)
	}
	if m.DecisionsOut != 1 {
		t.Errorf("DecisionsOut = %d, want 1", m.DecisionsOut)
	}
}

func TestAutoCapture_Metrics(t *testing.T) {
	ac := NewAutoCapture(nil)
	if ac == nil {
		t.Fatal("NewAutoCapture(nil) returned nil")
	}
	defer ac.Stop()

	metrics := ac.Metrics()
	_ = metrics
}

