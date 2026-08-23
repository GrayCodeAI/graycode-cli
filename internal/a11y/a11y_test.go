package a11y

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

// fixture builds a flat node list the way CDP returns it.
type fn struct {
	id, role, name, value string
	ignored               bool
	children              []string
	backend               int64
}

func build(fs []fn) ([]Node, string) {
	nodes := make([]Node, 0, len(fs))
	for _, f := range fs {
		n := Node{
			ID: f.id, Role: f.role, Name: f.name, Value: f.value,
			Ignored: f.ignored, ChildIDs: f.children, BackendDOMID: f.backend,
		}
		nodes = append(nodes, n)
	}
	raw, _ := json.Marshal(nodes)
	return nodes, string(raw)
}

func TestCompressAssignsUIDsToActionables(t *testing.T) {
	nodes, raw := build([]fn{
		{"1", "WebArea", "My App", "", false, []string{"2", "3", "4"}, 0},
		{"2", "generic", "", "", false, []string{"5"}, 0},
		{"5", "button", "Submit order", "", false, nil, 101},
		{"3", "textbox", "Email", "you@example.com", false, nil, 102},
		{"4", "link", "Docs", "", false, nil, 103},
	})
	snap, err := Compress(nodes, raw, "")
	if err != nil {
		t.Fatalf("Compress: %v", err)
	}
	if len(snap.Refs) != 3 {
		t.Fatalf("refs = %d, want 3", len(snap.Refs))
	}
	if snap.Refs["u1"].Role != "button" || snap.Refs["u1"].BackendDOMID != 101 {
		t.Fatalf("u1 = %+v", snap.Refs["u1"])
	}
	if !strings.Contains(snap.Text, `button u1 "Submit order"`) {
		t.Fatalf("text = %q", snap.Text)
	}
	if strings.Contains(snap.Text, "generic") {
		t.Fatal("layout containers should not render")
	}
}

func TestCompressQueryKeepsTopMatchesAndAncestors(t *testing.T) {
	nodes, raw := build([]fn{
		{"1", "WebArea", "Shop", "", false, []string{"2"}, 0},
		{"2", "generic", "", "", false, []string{"3", "4", "5", "6"}, 0},
		{"3", "link", "red shoes size 9", "", false, nil, 11},
		{"4", "link", "blue hats", "", false, nil, 12},
		{"5", "link", "green socks", "", false, nil, 13},
		{"6", "button", "checkout cart", "", false, nil, 14},
	})
	snap, err := Compress(nodes, raw, "shoes")
	if err != nil {
		t.Fatal(err)
	}
	if !snap.Truncated {
		t.Fatal("query mode with dropped matches should mark Truncated")
	}
	if _, ok := snap.Refs["u1"]; !ok {
		t.Fatalf("top match missing from refs: %+v", snap.Refs)
	}
	if strings.Contains(snap.Text, "hats") || strings.Contains(snap.Text, "socks") {
		t.Fatalf("non-matching items should be pruned: %q", snap.Text)
	}
}

func TestCompressFailClosedWhenNotSmaller(t *testing.T) {
	// A tiny tree compresses to roughly its own length; force the condition by
	// using a padded raw payload smaller than any render.
	nodes, _ := build([]fn{
		{"1", "button", "ok", "", false, nil, 7},
	})
	raw := `[{"nodeId":"1"}]` // deliberately tiny vs rendered text
	if _, err := Compress(nodes, raw, ""); !errors.Is(err, ErrNotSmaller) {
		t.Fatalf("err = %v, want ErrNotSmaller", err)
	}
}

func TestCompressIframeLeafRule(t *testing.T) {
	// Node 2 references childId 99 which is absent from this payload (another
	// frame's document under site isolation): render as a leaf, not an error.
	nodes, raw := build([]fn{
		{"1", "WebArea", "", "", false, []string{"2"}, 0},
		{"2", "iframe", "payment frame", "", false, []string{"99"}, 21},
	})
	snap, err := Compress(nodes, raw, "")
	if err != nil {
		t.Fatalf("unresolvable childId must be a leaf: %v", err)
	}
	if !strings.Contains(snap.Text, "frame") {
		t.Fatalf("frame leaf missing: %q", snap.Text)
	}
}

func TestCompressIgnoredSkipped(t *testing.T) {
	nodes, raw := build([]fn{
		{"1", "WebArea", "", "", false, []string{"2", "3"}, 0},
		{"2", "button", "hidden control", "", true, nil, 31}, // ignored
		{"3", "button", "visible", "", false, nil, 32},
	})
	snap, err := Compress(nodes, raw, "")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(snap.Text, "hidden control") {
		t.Fatal("ignored node leaked into snapshot")
	}
	if len(snap.Refs) != 1 {
		t.Fatalf("refs = %d, want 1", len(snap.Refs))
	}
}

func TestRawJSONRetainedByteExact(t *testing.T) {
	nodes, raw := build([]fn{{"1", "button", "b", "", false, nil, 1}})
	snap, err := Compress(nodes, raw, "")
	if err != nil {
		t.Fatal(err)
	}
	if snap.RawJSON != raw {
		t.Fatal("raw payload altered")
	}
}
