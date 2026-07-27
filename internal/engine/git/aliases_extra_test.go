package git

import (
	"testing"
)

func TestNewContext(t *testing.T) {
	ctx := NewContext("/tmp")
	if ctx == nil {
		t.Fatal("expected non-nil Context")
	}
}

func TestNewContext_EmptyDir(t *testing.T) {
	ctx := NewContext("")
	if ctx == nil {
		t.Fatal("expected non-nil Context")
	}
}

func TestNewProvider(t *testing.T) {
	provider := NewProvider("github", "token", "owner", "repo")
	if provider == nil {
		t.Fatal("expected non-nil Provider")
	}
}

func TestNewProvider_EmptyArgs(t *testing.T) {
	provider := NewProvider("", "", "", "")
	if provider == nil {
		t.Fatal("expected non-nil Provider")
	}
}

func TestNewProvider_GitLab(t *testing.T) {
	provider := NewProvider("gitlab", "token", "owner", "repo")
	if provider == nil {
		t.Fatal("expected non-nil Provider")
	}
}
