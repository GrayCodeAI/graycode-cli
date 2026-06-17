package engine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestL2PipelineStatePathsAreHomeRelative is a regression guard for L2 —
// the three learning-pipeline stores (ExperienceStore, KnowledgeBase,
// FeedbackCollector) created by NewIntegrationPipeline must write to
// ~/.hawk/{experience,knowledge,feedback}/, not to <cwd>/.hawk/...
//
// Pre-fix, NewIntegrationPipeline passed the literal strings
// ".hawk/experience", ".hawk/knowledge", ".hawk/feedback" to those
// constructors, which leaked into <cwd>/cmd/.hawk/ when hawk was run
// from its own source tree.
func TestL2PipelineStatePathsAreHomeRelative(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("os.UserHomeDir: %v", err)
	}
	if home == "" {
		t.Fatal("os.UserHomeDir returned empty string")
	}
	wantPrefix := filepath.Clean(home) + string(filepath.Separator)

	check := func(name, got string) {
		t.Helper()
		if !filepath.IsAbs(got) {
			t.Errorf("%s: path %q is not absolute", name, got)
			return
		}
		if !strings.HasPrefix(got, wantPrefix) && !strings.HasPrefix(got, filepath.Clean(home)) {
			t.Errorf("%s: path %q does not start with home dir %q", name, got, home)
		}
	}

	p := NewIntegrationPipeline()
	if p == nil {
		t.Fatal("NewIntegrationPipeline returned nil")
	}
	if p.ExperienceStore == nil || p.KnowledgeBase == nil || p.FeedbackCollector == nil {
		t.Fatal("NewIntegrationPipeline left a learning-pipeline store nil")
	}

	check("ExperienceStore.Dir", p.ExperienceStore.Dir)
	check("KnowledgeBase.Dir", p.KnowledgeBase.Dir)
	check("FeedbackCollector.Dir", p.FeedbackCollector.Dir)
}
