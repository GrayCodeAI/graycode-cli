package cmd

import "testing"

func TestParseFeatures_Numbered(t *testing.T) {
	input := `1. Add user authentication
2. Implement rate limiting
3. Add logging middleware`

	features := parseFeatures(input)
	if len(features) != 3 {
		t.Fatalf("expected 3 features, got %d", len(features))
	}
	if features[0].Description != "Add user authentication" {
		t.Errorf("unexpected first feature: %q", features[0].Description)
	}
	if features[2].Description != "Add logging middleware" {
		t.Errorf("unexpected third feature: %q", features[2].Description)
	}
}

func TestParseFeatures_Bulleted(t *testing.T) {
	input := `- Create database schema
- Build REST endpoints
- Write integration tests`

	features := parseFeatures(input)
	if len(features) != 3 {
		t.Fatalf("expected 3 features, got %d", len(features))
	}
	if features[0].Description != "Create database schema" {
		t.Errorf("unexpected: %q", features[0].Description)
	}
}

func TestParseFeatures_Parenthesis(t *testing.T) {
	input := `1) Setup project structure
2) Add core logic
3) Write tests`

	features := parseFeatures(input)
	if len(features) != 3 {
		t.Fatalf("expected 3 features, got %d", len(features))
	}
	if features[0].Description != "Setup project structure" {
		t.Errorf("unexpected: %q", features[0].Description)
	}
}

func TestParseFeatures_EmptyLines(t *testing.T) {
	input := `
1. Feature A

2. Feature B

`
	features := parseFeatures(input)
	if len(features) != 2 {
		t.Fatalf("expected 2 features, got %d", len(features))
	}
}

func TestParseFeatures_Plain(t *testing.T) {
	input := `Add auth
Add tests`

	features := parseFeatures(input)
	if len(features) != 2 {
		t.Fatalf("expected 2 features, got %d", len(features))
	}
}

func TestGetCurrentBranch_NoGit(t *testing.T) {
	branch := getCurrentBranch("/tmp")
	if branch != "main" {
		t.Errorf("expected 'main' fallback, got %q", branch)
	}
}
