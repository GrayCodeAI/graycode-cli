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
// Validate. Each type's Data blob is decoded back into its typed in-memory payload
// (Meta, Message, ToolCallPayload, ToolResultPayload) so a loaded log can be
// projected without a second decode hop in the consumer.
func DecodeWire(wire []WireEvent) ([]Event, error) {
	events := make([]Event, len(wire))
	for i, w := range wire {
		data, err := decodePayload(w)
		if err != nil {
			return nil, err
		}
		events[i] = Event{Type: w.Type, Seq: w.Seq, At: w.At, Data: data}
	}
	return events, Validate(events)
}

// decodePayload converts a wire Data blob into the payload struct for w.Type.
// A nil or empty blob decodes to nil; a non-empty blob must unmarshal cleanly.
func decodePayload(w WireEvent) (any, error) {
	if len(w.Data) == 0 {
		return nil, nil
	}
	switch w.Type {
	case SessionMeta:
		var p Meta
		if err := json.Unmarshal(w.Data, &p); err != nil {
			return nil, err
		}
		return p, nil
	case UserMessage, AssistantMsg:
		var p Message
		if err := json.Unmarshal(w.Data, &p); err != nil {
			return nil, err
		}
		return p, nil
	case ToolCall:
		var p ToolCallPayload
		if err := json.Unmarshal(w.Data, &p); err != nil {
			return nil, err
		}
		return p, nil
	case ToolResult:
		var p ToolResultPayload
		if err := json.Unmarshal(w.Data, &p); err != nil {
			return nil, err
		}
		return p, nil
	default:
		return nil, nil
	}
}
