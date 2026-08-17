package eventlog

import "testing"

func TestBoundaryAndPermissionFactsRoundTrip(t *testing.T) {
	log := New(nil)
	log.AppendTurnStart(1)
	log.AppendStepStart(1, 1)
	log.AppendAssistantChunk(1, 1, "hello")
	log.AppendStepEnd(1, 1)
	log.AppendTurnEnd(1, "completed")
	log.AppendPermission(PermissionFact{
		Tool:     "Bash",
		Category: "file_deletion",
		Allowed:  false,
		Message:  "denied",
	})

	if log.OfType(TurnStart)[0].Data.(BoundaryFact).Turn != 1 {
		t.Fatal("turn start missing")
	}
	if log.OfType(PermissionChange)[0].Data.(PermissionFact).Allowed {
		t.Fatal("permission should record deny")
	}

	wire, err := MarshalWire(log.Snapshot())
	if err != nil {
		t.Fatal(err)
	}
	events, err := DecodeWire(wire)
	if err != nil {
		t.Fatal(err)
	}
	for _, ev := range events {
		switch ev.Type {
		case TurnStart, StepStart, StepEnd:
			if _, ok := ev.Data.(BoundaryFact); !ok {
				t.Fatalf("%s payload = %T", ev.Type, ev.Data)
			}
		case TurnEnd:
			if _, ok := ev.Data.(TurnEndFact); !ok {
				t.Fatalf("turn/end payload = %T", ev.Data)
			}
		case AssistantChunk:
			if _, ok := ev.Data.(ChunkFact); !ok {
				t.Fatalf("chunk payload = %T", ev.Data)
			}
		case PermissionChange:
			if _, ok := ev.Data.(PermissionFact); !ok {
				t.Fatalf("permission payload = %T", ev.Data)
			}
		}
	}
}
