package repomap

import (
	"go/ast"
	"testing"
)

func TestSplitByEmptyLine(t *testing.T) {
	tests := []struct {
		input    string
		expected int // number of chunks
	}{
		{"line1\n\nline2", 2},
		{"line1\nline2", 1},
		{"", 0},
		{"\n\n\n", 0},
		{"a\n\nb\n\nc", 3},
		{"  \n\n  ", 0},
	}

	for _, tt := range tests {
		result := splitByEmptyLine(tt.input)
		if len(result) != tt.expected {
			t.Errorf("splitByEmptyLine(%q) = %d chunks, want %d", tt.input, len(result), tt.expected)
		}
	}
}

func TestNonEmptyLines(t *testing.T) {
	tests := []struct {
		input    string
		expected int
	}{
		{"line1\nline2", 2},
		{"line1\n\nline2", 2},
		{"", 0},
		{"\n\n\n", 0},
		{"  line1  \n\n  line2  ", 2},
	}

	for _, tt := range tests {
		result := nonEmptyLines(tt.input)
		if len(result) != tt.expected {
			t.Errorf("nonEmptyLines(%q) = %d lines, want %d", tt.input, len(result), tt.expected)
		}
	}
}

func TestNonEmptyLines_TrimsWhitespace(t *testing.T) {
	result := nonEmptyLines("  hello  \n  world  ")
	if len(result) != 2 {
		t.Fatalf("expected 2 lines, got %d", len(result))
	}
	if result[0] != "hello" {
		t.Errorf("result[0] = %q, want %q", result[0], "hello")
	}
	if result[1] != "world" {
		t.Errorf("result[1] = %q, want %q", result[1], "world")
	}
}

func TestItoa(t *testing.T) {
	tests := []struct {
		input    int
		expected string
	}{
		{0, "100"},
		{-1, "100"},
		{1, "1"},
		{10, "10"},
		{100, "100"},
		{123, "123"},
		{999, "999"},
	}

	for _, tt := range tests {
		result := itoa(tt.input)
		if result != tt.expected {
			t.Errorf("itoa(%d) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

func TestExprName_Ident(t *testing.T) {
	expr := &ast.Ident{Name: "myFunc"}
	result := exprName(expr)
	if result != "myFunc" {
		t.Errorf("exprName(Ident) = %q, want %q", result, "myFunc")
	}
}

func TestExprName_StarExpr(t *testing.T) {
	expr := &ast.StarExpr{
		X: &ast.Ident{Name: "myType"},
	}
	result := exprName(expr)
	if result != "myType" {
		t.Errorf("exprName(StarExpr) = %q, want %q", result, "myType")
	}
}

func TestExprName_Unknown(t *testing.T) {
	// Test with nil or unknown expression type
	result := exprName(nil)
	if result != "" {
		t.Errorf("exprName(nil) = %q, want empty", result)
	}
}

func TestBuildCoChangeAnalysis_NonExistentDir(t *testing.T) {
	// Should return empty analysis for non-existent directory
	analysis, err := BuildCoChangeAnalysis("/nonexistent-directory-xyz", 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if analysis == nil {
		t.Fatal("expected non-nil analysis")
	}
}

func TestBuildCoChangeAnalysis_ZeroCommitLimit(t *testing.T) {
	// commitLimit <= 0 should default to 100
	tmpDir := t.TempDir()
	analysis, err := BuildCoChangeAnalysis(tmpDir, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if analysis == nil {
		t.Fatal("expected non-nil analysis")
	}
}
