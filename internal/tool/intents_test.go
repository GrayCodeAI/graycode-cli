package tool

import "testing"

func TestMatchIntentBundles(t *testing.T) {
	got := MatchIntentBundles("Please inspect this URL and run the tests")
	seen := make(map[string]bool, len(got))
	for _, bundle := range got {
		seen[bundle.Name] = true
	}
	if !seen["web"] || !seen["verification"] {
		t.Fatalf("matched bundles = %#v, want web and verification", seen)
	}
}

func TestPromoteForIntentOnlyPromotesRegisteredTools(t *testing.T) {
	registry := NewRegistry(FileReadTool{}, WebFetchTool{}, DiagnosticsTool{})
	registry.EnableLazyModelSurface([]string{"Read"})

	got := registry.PromoteForIntent("check this website and run tests")
	if !registry.IsModelVisible("WebFetch") || !registry.IsModelVisible("Diagnostics") {
		t.Fatalf("web/diagnostics tools were not promoted: %v", got)
	}
	if registry.IsModelVisible("Browser") {
		t.Fatal("unregistered Browser tool was promoted")
	}

	second := registry.PromoteForIntent("check this website and run tests")
	if len(second) != 0 {
		t.Fatalf("second promotion = %v, want no duplicate promotions", second)
	}
}
