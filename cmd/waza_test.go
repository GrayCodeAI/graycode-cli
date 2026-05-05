package cmd

import (
	"strings"
	"testing"
)

func TestBuildThinkPrompt(t *testing.T) {
	p := buildThinkPrompt("add authentication to the API")
	if !strings.Contains(p, "THINK mode") {
		t.Error("should contain THINK mode header")
	}
	if !strings.Contains(p, "add authentication to the API") {
		t.Error("should contain the topic")
	}
	if !strings.Contains(p, "No code") {
		t.Error("should have no-code-before-approval rule")
	}
	if !strings.Contains(p, "Attack angles") {
		t.Error("should include attack angles section")
	}
	if !strings.Contains(p, "Handoff") {
		t.Error("should include handoff section")
	}
}

func TestBuildHuntPrompt(t *testing.T) {
	p := buildHuntPrompt("segfault on login")
	if !strings.Contains(p, "HUNT mode") {
		t.Error("should contain HUNT mode header")
	}
	if !strings.Contains(p, "segfault on login") {
		t.Error("should contain the symptom")
	}
	if !strings.Contains(p, "root cause") {
		t.Error("should mention root cause")
	}
	if !strings.Contains(p, "3 failed hypotheses") {
		t.Error("should have 3-hypothesis stop rule")
	}
	if !strings.Contains(p, "Bisect") {
		t.Error("should include bisect mode")
	}
}

func TestBuildCheckPrompt(t *testing.T) {
	p := buildCheckPrompt()
	if !strings.Contains(p, "CHECK mode") {
		t.Error("should contain CHECK mode header")
	}
	if !strings.Contains(p, "git diff") {
		t.Error("should mention getting the diff")
	}
	if !strings.Contains(p, "Auto-fix") {
		t.Error("should mention auto-fix for safe issues")
	}
	if !strings.Contains(p, "Verify") {
		t.Error("should mention verification")
	}
}

func TestBuildDesignPrompt(t *testing.T) {
	p := buildDesignPrompt("landing page hero section")
	if !strings.Contains(p, "DESIGN mode") {
		t.Error("should contain DESIGN mode header")
	}
	if !strings.Contains(p, "landing page hero section") {
		t.Error("should contain the topic")
	}
	if !strings.Contains(p, "Iterate") {
		t.Error("should include iteration process")
	}
	if !strings.Contains(p, "Mobile-first") {
		t.Error("should mention mobile-first")
	}
	if !strings.Contains(p, "Accessibility") {
		t.Error("should mention accessibility")
	}
}
