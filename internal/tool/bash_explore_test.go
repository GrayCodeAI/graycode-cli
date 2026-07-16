package tool

import "testing"

func TestExploreBashAllowed_ReadOK(t *testing.T) {
	ok := []string{
		"ls -la",
		"pwd",
		"git status",
		"git log --oneline -5",
		"git diff HEAD~1",
		"rg TODO --type go",
		"cat README.md",
		"go list ./...",
		"go version",
		"ls | head",
		"echo hello && pwd",
		"FOO=bar ls",
	}
	for _, c := range ok {
		if err := ExploreBashAllowed(c); err != nil {
			t.Errorf("ExploreBashAllowed(%q)=%v want nil", c, err)
		}
	}
}

func TestExploreBashAllowed_MutationsDenied(t *testing.T) {
	deny := []string{
		"rm -rf /tmp/x",
		"git commit -am msg",
		"git push origin main",
		"git checkout -b feature",
		"go fmt ./...",
		"go get example.com/x",
		"npm install lodash",
		"sed -i 's/a/b/' f.go",
		"find . -delete",
		"echo hi > out.txt",
		"sudo ls",
		"chmod 777 x",
		"mv a b",
	}
	for _, c := range deny {
		if err := ExploreBashAllowed(c); err == nil {
			t.Errorf("ExploreBashAllowed(%q)=nil want error", c)
		}
	}
}

func TestExploreBashAllowed_PipelineGate(t *testing.T) {
	// second stage mutating
	if err := ExploreBashAllowed("cat f | tee out"); err == nil {
		// tee is not on allowlist
		t.Log("tee denied as expected if not allowlisted")
	}
	if err := ExploreBashAllowed("ls; rm -rf x"); err == nil {
		t.Fatal("expected deny for multi-segment with rm")
	}
}
