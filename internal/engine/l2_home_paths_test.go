package engine

import (
	"path/filepath"
	"strings"
	"testing"
)

// TestL2PipelineStatePathsAreHomeRelative is a regression guard for L2 —
// the three learning-pipeline stores (ExperienceStore, KnowledgeBase,
// FeedbackCollector) created by NewIntegrationPipeline must write to
// ~/.graycode/{experience,knowledge,feedback}/, not to <cwd>/.graycode/...
//
// Pre-fix, NewIntegrationPipeline passed the literal strings
// ".graycode/experience", ".graycode/knowledge", ".graycode/feedback" to those
// constructors, which leaked into <cwd>/cmd/.graycode/ when graycode was run
// from its own source tree.
func TestL2PipelineStatePathsAreHomeRelative(t *testing.T) {
	// Make the test independent of the caller's HOME/GRAYCODE_STATE_DIR. The
	// production contract is the configured per-user state root, and tests may
	// intentionally redirect that root to an isolated temporary directory.
	stateRoot := t.TempDir()
	t.Setenv("GRAYCODE_STATE_DIR", stateRoot)
	wantPrefix := filepath.Clean(stateRoot) + string(filepath.Separator)

	check := func(name, got string) {
		t.Helper()
		if !filepath.IsAbs(got) {
			t.Errorf("%s: path %q is not absolute", name, got)
			return
		}
		if !strings.HasPrefix(got, wantPrefix) {
			t.Errorf("%s: path %q does not start with state root %q", name, got, stateRoot)
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
