package eventlog

import (
	"encoding/json"
	"testing"
)

func TestAppendSpecDurableAndRoundTrips(t *testing.T) {
	log := New(nil)
	log.AppendSpec("plan", "demo")

	evs := log.OfType(SpecState)
	if len(evs) != 1 {
		t.Fatalf("spec facts = %d, want 1", len(evs))
	}
	if got := evs[0].Data.(SpecFact); got.Stage != "plan" || got.Slug != "demo" {
		t.Fatalf("payload = %#v", got)
	}

	wire, err := MarshalWire(evs)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	events, err := DecodeWire(wire)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	got, ok := events[0].Data.(SpecFact)
	if !ok || got.Stage != "plan" {
		t.Fatalf("round-trip spec fact = %#v", events[0].Data)
	}
}

func TestKnownIncludesSpecState(t *testing.T) {
	if !SpecState.Known() {
		t.Fatal("SpecState should be known")
	}
	var unknown Type = "spec.mystery"
	if unknown.Known() {
		t.Fatal("unknown type should not be known")
	}
	// Don't let the zero-value Type slip through.
	var zero Type
	if zero.Known() {
		t.Fatal("zero Type should not be known")
	}
}

// Guard the commentless shape used elsewhere: JSON only carries stage/slug.
func TestSpecFactJSONShape(t *testing.T) {
	b, _ := json.Marshal(SpecFact{Stage: "plan", Slug: "demo"})
	if string(b) != `{"stage":"plan","slug":"demo"}` {
		t.Fatalf("json = %s", b)
	}
}
