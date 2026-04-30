package update

import "testing"

func TestIsNewer(t *testing.T) {
	tests := []struct {
		a     string
		b     string
		newer bool
	}{
		{"1.0.1", "1.0.0", true},
		{"1.0.0", "1.0.0", false},
		{"1.1.0", "1.0.0", true},
		{"2.0.0", "1.0.0", true},
		{"1.0.0", "1.0.1", false},
		{"v1.0.1", "v1.0.0", true},
	}

	for _, tt := range tests {
		result := isNewer(tt.a, tt.b)
		if result != tt.newer {
			t.Errorf("isNewer(%q, %q) = %v, want %v", tt.a, tt.b, result, tt.newer)
		}
	}
}

func TestPlatform(t *testing.T) {
	p := Platform()
	if p == "" {
		t.Fatal("expected non-empty platform")
	}
}
