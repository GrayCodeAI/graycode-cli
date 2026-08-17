package eventlog

import (
	"encoding/json"
	"time"
)

// WireEvent is the on-disk and over-the-wire shape of one durable event. It is
// distinct from the in-memory Event so the storage schema and the live record can
// stay byte-compatible and the log can decode its own replay without importing any
// product storage package.
type WireEvent struct {
	Type Type            `json:"type"`
	Seq  uint64          `json:"seq"`
	At   time.Time       `json:"at"`
	Data json.RawMessage `json:"data,omitempty"`
}

// MarshalWire converts in-memory events to their persisted shape.
func MarshalWire(events []Event) ([]WireEvent, error) {
	out := make([]WireEvent, len(events))
	for i, ev := range events {
		var raw json.RawMessage
		if ev.Data != nil {
			b, err := json.Marshal(ev.Data)
			if err != nil {
				return nil, err
			}
			raw = b
		}
		out[i] = WireEvent{Type: ev.Type, Seq: ev.Seq, At: ev.At, Data: raw}
	}
	return out, nil
}

// DecodeWire validates and loads a wire record. Unknown kinds fail loud via
// Validate; the Data fields stay raw and are decoded by the owning consumer.
func DecodeWire(wire []WireEvent) ([]Event, error) {
	events := make([]Event, len(wire))
	for i, w := range wire {
		events[i] = Event{Type: w.Type, Seq: w.Seq, At: w.At, Data: nil}
	}
	return events, Validate(events)
}
