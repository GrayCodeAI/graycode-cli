package toolset

import (
	"reflect"
	"sort"
	"testing"
)

func TestResolveFlat(t *testing.T) {
	r, _ := NewRegistry([]Toolset{{Name: "research", Tools: []string{"WebSearch", "Read"}}})
	tools, err := r.Resolve("research")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"Read", "WebSearch"}
	if !reflect.DeepEqual(tools, want) {
		t.Fatalf("tools = %v, want %v", tools, want)
	}
}

func TestResolveComposedAndDeduped(t *testing.T) {
	r, _ := NewRegistry(Defaults())
	tools, err := r.Resolve("full_stack")
	if err != nil {
		t.Fatal(err)
	}
	// dev+ops both include research; shared tools (Read, Bash, WebFetch) must
	// appear once.
	seen := map[string]bool{}
	dupes := 0
	for _, t := range tools {
		if seen[t] {
			dupes++
		}
		seen[t] = true
	}
	if dupes != 0 {
		t.Fatalf("duplicate tools after resolve: %d", dupes)
	}
	// dev tools must be present.
	for _, want := range []string{"Read", "Write", "Edit", "Bash", "CodeMatch", "ProjectVerify"} {
		if !seen[want] {
			t.Fatalf("dev tool %q missing from full_stack: %v", want, tools)
		}
	}
	if !sort.StringsAreSorted(tools) {
		t.Fatal("resolved tools must be sorted")
	}
}

func TestResolveCycleSafe(t *testing.T) {
	r, _ := NewRegistry([]Toolset{
		{Name: "a", Requires: []string{"b"}},
		{Name: "b", Requires: []string{"a"}, Tools: []string{"X"}},
	})
	tools, err := r.Resolve("a")
	if err != nil {
		t.Fatal(err)
	}
	if len(tools) != 1 || tools[0] != "X" {
		t.Fatalf("tools = %v", tools)
	}
}

func TestResolveUnknown(t *testing.T) {
	r, _ := NewRegistry(Defaults())
	if _, err := r.Resolve("nope"); err == nil {
		t.Fatal("expected error for unknown toolset")
	}
}

func TestDuplicateNameError(t *testing.T) {
	if _, err := NewRegistry([]Toolset{{Name: "x"}, {Name: "x"}}); err == nil {
		t.Fatal("expected duplicate name error")
	}
}

func TestNamesSorted(t *testing.T) {
	r, _ := NewRegistry(Defaults())
	names := r.Names()
	if !sort.StringsAreSorted(names) {
		t.Fatalf("names not sorted: %v", names)
	}
	for _, want := range []string{"dev", "full_stack", "ops", "research"} {
		found := false
		for _, n := range names {
			if n == want {
				found = true
			}
		}
		if !found {
			t.Fatalf("missing %q in %v", want, names)
		}
	}
}
