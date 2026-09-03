package cmd

import (
	"fmt"
	"strings"
	"testing"
)

func TestProviderCountCopyMatchesRegistry(t *testing.T) {
	count := registeredProviderCount()
	if count < 20 {
		t.Fatalf("registered providers = %d, expected a first-class provider registry", count)
	}

	want := fmt.Sprintf("%d first-class LLM providers", count)
	if !strings.Contains(rootCmd.Long, want) {
		t.Fatalf("CLI help does not contain registry-backed provider count %q", want)
	}
	if page := GenerateManPage(); !strings.Contains(page, want) {
		t.Fatalf("manpage does not contain registry-backed provider count %q", want)
	}
}

func TestGenerateManPage(t *testing.T) {
	preserveCLICompilerVersionState(t)
	version = "1.0.0"
	page := GenerateManPage()

	if !strings.Contains(page, ".TH GRAYCODE 1") {
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
	if !strings.Contains(page, ".SH CREDENTIALS") {
		t.Fatal("missing CREDENTIALS section")
	}
	if !strings.Contains(page, "/config") {
		t.Fatal("missing /config guidance in credentials section")
	}
	if !strings.Contains(page, "GrayCode AI") {
		t.Fatal("missing AUTHORS section")
	}
}

func TestGenerateManPage_EmptyVersion(t *testing.T) {
	preserveCLICompilerVersionState(t)
	version = ""
	page := GenerateManPage()
	if !strings.Contains(page, "dev") {
		t.Fatal("expected 'dev' as fallback version")
	}
}
