package cmd

import "testing"

func TestWelcomeDockerSegment(t *testing.T) {
	green, red, rst := "\033[32m", "\033[31m", "\033[0m"

	seg, vis := welcomeDockerSegment(nil, green, red, rst)
	if seg != "" || vis != 0 {
		t.Fatalf("expected skip when docker disabled, got %q vis=%d", seg, vis)
	}

	running := true
	seg, vis = welcomeDockerSegment(&running, green, red, rst)
	if seg == "" || vis != len("  Docker x") {
		t.Fatalf("running segment = %q vis=%d", seg, vis)
	}
	if !containsSubstring(seg, green) {
		t.Fatalf("expected green checkmark in %q", seg)
	}

	stopped := false
	seg, _ = welcomeDockerSegment(&stopped, green, red, rst)
	if !containsSubstring(seg, red) {
		t.Fatalf("expected red cross in %q", seg)
	}
}

func containsSubstring(s, sub string) bool {
	return len(sub) == 0 || (len(s) >= len(sub) && indexSubstring(s, sub) >= 0)
}

func indexSubstring(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
