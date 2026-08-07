package sandbox

import "testing"

func TestCodeVerifier_JSAndRuby(t *testing.T) {
	cv := NewCodeVerifier()

	js := `const { execSync } = require('child_process');
execSync('rm -rf /');`
	r := cv.Verify(js, "javascript")
	if r.Safe {
		t.Fatal("JS with child_process should be unsafe")
	}

	ruby := `FileUtils.rm_rf("/tmp/foo")
Kernel#system("ls")`
	r = cv.Verify(ruby, "ruby")
	if r.Safe {
		t.Fatal("Ruby with FileUtils.rm_rf should be unsafe")
	}

	// Safe JS should pass.
	safeJS := `export const add = (a, b) => a + b;`
	r = cv.Verify(safeJS, "javascript")
	if !r.Safe {
		t.Fatal("safe JS should pass")
	}
}

func TestCodeVerifier_ApplyConfig(t *testing.T) {
	cv := NewCodeVerifier()
	cv.ApplyConfig(&CodeVerifierConfig{
		BlockedModules:  []string{"dangerous-module"},
		BlockedPatterns: []string{`require\s*\(\s*["']dangerous-module["']\s*\)`},
	})
	r := cv.Verify(`require('dangerous-module')`, "javascript")
	if r.Safe {
		t.Fatal("configured blocked module should be unsafe")
	}
}
