package cmd

import (
	"strings"
	"testing"
)

func TestGenerateManPage(t *testing.T) {
	version = "1.0.0"
	page := GenerateManPage()

	if !strings.Contains(page, ".TH HAWK 1") {
		t.Fatal("missing .TH header")
	}
	if !strings.Contains(page, "1.0.0") {
		t.Fatal("missing version")
	}
	if !strings.Contains(page, ".SH NAME") {
		t.Fatal("missing NAME section")
	}
	if !strings.Contains(page, ".SH SYNOPSIS") {
		t.Fatal("missing SYNOPSIS section")
	}
	if !strings.Contains(page, ".SH DESCRIPTION") {
		t.Fatal("missing DESCRIPTION section")
	}
	if !strings.Contains(page, ".SH OPTIONS") {
		t.Fatal("missing OPTIONS section")
	}
	if !strings.Contains(page, ".SH SLASH COMMANDS") {
		t.Fatal("missing SLASH COMMANDS section")
	}
	if !strings.Contains(page, ".SH FILES") {
		t.Fatal("missing FILES section")
	}
	if !strings.Contains(page, ".SH ENVIRONMENT") {
		t.Fatal("missing ENVIRONMENT section")
	}
	if !strings.Contains(page, "ANTHROPIC_API_KEY") {
		t.Fatal("missing ANTHROPIC_API_KEY in env section")
	}
	if !strings.Contains(page, "GrayCode AI") {
		t.Fatal("missing AUTHORS section")
	}
}

func TestGenerateManPage_EmptyVersion(t *testing.T) {
	version = ""
	page := GenerateManPage()
	if !strings.Contains(page, "dev") {
		t.Fatal("expected 'dev' as fallback version")
	}
}
