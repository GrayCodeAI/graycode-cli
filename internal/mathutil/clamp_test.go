package mathutil

import "testing"

func TestClamp(t *testing.T) {
	if got := Clamp(5, 0, 10); got != 5 {
		t.Errorf("Clamp(5,0,10) = %d, want 5", got)
	}
	if got := Clamp(-1, 0, 10); got != 0 {
		t.Errorf("Clamp(-1,0,10) = %d, want 0", got)
	}
	if got := Clamp(20, 0, 10); got != 10 {
		t.Errorf("Clamp(20,0,10) = %d, want 10", got)
	}
	if got := Clamp(1.5, 0.0, 1.0); got != 1.0 {
		t.Errorf("Clamp(1.5,0,1) = %v, want 1.0", got)
	}
}
