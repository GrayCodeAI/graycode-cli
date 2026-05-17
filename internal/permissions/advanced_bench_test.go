package permissions

import "testing"

func BenchmarkClassifier(b *testing.B) {
	c := NewClassifier()
	commands := []string{
		"git status",
		"ls -la",
		"rm -rf /",
		"echo hello",
		"go test ./...",
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, cmd := range commands {
			c.Classify(cmd)
		}
	}
}

func BenchmarkAutoModeState(b *testing.B) {
	a := NewAutoModeState()
	a.Record("Bash", "git status", true)
	a.Record("Bash", "rm -rf /", false)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		a.ShouldAutoAllow("Bash", "git status")
	}
}
