package smartrouting

import "testing"

func on() Config {
	return Config{Enabled: true, SimpleModel: "mini", StrongModel: "main"}
}

func TestDisabledRoutesStrong(t *testing.T) {
	d := Route(Input{UserText: "ok"}, Config{Enabled: false, StrongModel: "main"})
	if d.Complexity != Strong || d.Model != "main" {
		t.Fatalf("got %+v", d)
	}
}

func TestMissingOrEqualModelsRouteStrong(t *testing.T) {
	if Route(Input{}, Config{Enabled: true, SimpleModel: "", StrongModel: "main"}).Complexity != Strong {
		t.Fatal("missing simple should route strong")
	}
	if Route(Input{}, Config{Enabled: true, SimpleModel: "same", StrongModel: "same"}).Complexity != Strong {
		t.Fatal("equal models should route strong")
	}
}

func TestShortChatRoutesSimple(t *testing.T) {
	d := Route(Input{UserText: "ok", TurnNumber: 2}, on())
	if d.Complexity != Simple || d.Model != "mini" {
		t.Fatalf("got %+v", d)
	}
}

func TestFirstTurnRoutesStrong(t *testing.T) {
	d := Route(Input{UserText: "ok", TurnNumber: 1}, on())
	if d.Complexity != Strong {
		t.Fatalf("first turn should route strong, got %+v", d)
	}
}

func TestNonTextRoutesStrong(t *testing.T) {
	d := Route(Input{UserText: "ok", TurnNumber: 2, HasNonText: true}, on())
	if d.Complexity != Strong {
		t.Fatalf("got %+v", d)
	}
}

func TestCodeRoutesStrong(t *testing.T) {
	for _, s := range []string{"```go\nx\n```", "use `foo`"} {
		if d := Route(Input{UserText: s, TurnNumber: 2}, on()); d.Complexity != Strong {
			t.Fatalf("code %q should route strong, got %+v", s, d)
		}
	}
}

func TestStrongKeywordRoutesStrong(t *testing.T) {
	for _, s := range []string{"plan the refactor", "why does this fail", "root cause analysis"} {
		if d := Route(Input{UserText: s, TurnNumber: 2}, on()); d.Complexity != Strong {
			t.Fatalf("keyword %q should route strong, got %+v", s, d)
		}
	}
}

func TestLongInputRoutesStrong(t *testing.T) {
	long := make([]byte, 161)
	for i := range long {
		long[i] = 'a'
	}
	if d := Route(Input{UserText: string(long), TurnNumber: 2}, on()); d.Complexity != Strong {
		t.Fatalf("long input should route strong, got %+v", d)
	}
}

func TestEmptyRoutesSimple(t *testing.T) {
	d := Route(Input{TurnNumber: 2}, on())
	if d.Complexity != Simple {
		t.Fatalf("empty should route simple, got %+v", d)
	}
}
