package eventlog

import (
	"encoding/json"
	"testing"
	"time"
)

// TestClassifyChunk verifies that only assistant.chunk events with the
// canonical three-field ChunkFact are packable.
func TestClassifyChunk(t *testing.T) {
	// Valid chunk.
	data, _ := json.Marshal(ChunkFact{Turn: 1, Step: 1, Chunk: "hello"})
	valid := WireEvent{Type: AssistantChunk, Seq: 1, At: time.Now(), Data: data}
	if f := classifyChunk(valid); f == nil || f.Chunk != "hello" {
		t.Fatal("valid chunk should classify")
	}

	// Non-chunk type.
	msg := WireEvent{Type: UserMessage, Seq: 1, At: time.Now(), Data: json.RawMessage("{}")}
	if f := classifyChunk(msg); f != nil {
		t.Fatal("non-chunk should not classify")
	}

	// Empty chunk.
	empty := WireEvent{Type: AssistantChunk, Seq: 2, At: time.Now(), Data: json.RawMessage(`{"turn":1,"step":1,"chunk":""}`)}
	if f := classifyChunk(empty); f != nil {
		t.Fatal("empty chunk should not classify")
	}

	// Extra key in ChunkFact.
	extra := WireEvent{Type: AssistantChunk, Seq: 3, At: time.Now(), Data: json.RawMessage(`{"turn":1,"step":1,"chunk":"hi","extra":"x"}`)}
	if f := classifyChunk(extra); f != nil {
		t.Fatal("canonical-shape check should reject extra keys (must store verbatim)")
	}

	// Nil data.
	nilData := WireEvent{Type: AssistantChunk, Seq: 4, At: time.Now(), Data: nil}
	if f := classifyChunk(nilData); f != nil {
		t.Fatal("nil data should not classify")
	}
}

// TestPackChunkRunsRoundTrip verifies that packing and then decoding produces
// the exact original event slice.
func TestPackChunkRunsRoundTrip(t *testing.T) {
	base := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)

	// 10 consecutive same-turn/step chunks → should pack into one row.
	var events []WireEvent
	for i := 0; i < 10; i++ {
		data, _ := json.Marshal(ChunkFact{Turn: 1, Step: 1, Chunk: chunkText(i)})
		events = append(events, WireEvent{
			Type: AssistantChunk,
			Seq:  uint64(i + 1),
			At:   base.Add(time.Duration(i) * time.Millisecond),
			Data: data,
		})
	}

	packed := PackChunkRuns(events)
	if len(packed) != 1 {
		t.Fatalf("expected 1 packed record, got %d", len(packed))
	}
	if packed[0].ChunkRow == nil {
		t.Fatal("expected a chunk row")
	}
	row := packed[0].ChunkRow
	if row.Seq0 != 1 {
		t.Errorf("Seq0 = %d, want 1", row.Seq0)
	}
	if len(row.Texts) != 10 {
		t.Errorf("Texts length = %d, want 10", len(row.Texts))
	}
	if row.Texts[0] != chunkText(0) || row.Texts[9] != chunkText(9) {
		t.Errorf("Texts mismatch: got %q, %q", row.Texts[0], row.Texts[9])
	}

	// Decode the row back to events.
	raw, err := json.Marshal(row)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeStorageRecord(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(decoded) != 10 {
		t.Fatalf("decoded %d events, want 10", len(decoded))
	}
	for i, ev := range decoded {
		var f ChunkFact
		if err := json.Unmarshal(ev.Data, &f); err != nil {
			t.Fatal(err)
		}
		if f.Chunk != chunkText(i) {
			t.Errorf("chunk %d: got %q, want %q", i, f.Chunk, chunkText(i))
		}
		if ev.Seq != uint64(i+1) {
			t.Errorf("chunk %d: seq = %d, want %d", i, ev.Seq, i+1)
		}
		expectedTime := base.Add(time.Duration(i) * time.Millisecond)
		if !ev.At.Equal(expectedTime) {
			t.Errorf("chunk %d: time = %v, want %v", i, ev.At, expectedTime)
		}
	}
}

// TestPackChunkRunsNoPackShortRun verifies runs shorter than minPackRun pass
// through verbatim.
func TestPackChunkRunsNoPackShortRun(t *testing.T) {
	base := time.Now()
	var events []WireEvent
	for i := 0; i < 2; i++ { // only 2, below MIN_RUN=3
		data, _ := json.Marshal(ChunkFact{Turn: 1, Step: 1, Chunk: chunkText(i)})
		events = append(events, WireEvent{
			Type: AssistantChunk, Seq: uint64(i + 1), At: base.Add(time.Duration(i) * time.Millisecond), Data: data,
		})
	}
	packed := PackChunkRuns(events)
	if len(packed) != 2 {
		t.Fatalf("expected 2 passthrough records, got %d", len(packed))
	}
	for _, r := range packed {
		if r.ChunkRow != nil {
			t.Fatal("short run should not pack")
		}
	}
}

// TestPackChunkRunsRunBoundary verifies that a turn/step change splits runs.
func TestPackChunkRunsRunBoundary(t *testing.T) {
	base := time.Now()

	// 4 chunks turn=1/step=1, then 4 chunks turn=1/step=2 → two rows.
	var events []WireEvent
	seq := uint64(1)
	for _, step := range []int{1, 2} {
		for j := 0; j < 4; j++ {
			data, _ := json.Marshal(ChunkFact{Turn: 1, Step: step, Chunk: "x"})
			events = append(events, WireEvent{
				Type: AssistantChunk, Seq: seq, At: base.Add(time.Duration(seq) * time.Millisecond), Data: data,
			})
			seq++
		}
	}

	packed := PackChunkRuns(events)
	if len(packed) != 2 {
		t.Fatalf("expected 2 packed rows (one per step), got %d", len(packed))
	}
	if packed[0].ChunkRow == nil || packed[1].ChunkRow == nil {
		t.Fatal("both records should be chunk rows")
	}

	// Non-chunk events between chunk runs should not pack.
	var mixed []WireEvent
	data, _ := json.Marshal(ChunkFact{Turn: 1, Step: 1, Chunk: "a"})
	mixed = append(mixed, WireEvent{Type: AssistantChunk, Seq: 1, At: base, Data: data})
	data2, _ := json.Marshal(ChunkFact{Turn: 1, Step: 1, Chunk: "b"})
	mixed = append(mixed, WireEvent{Type: AssistantChunk, Seq: 2, At: base, Data: data2})
	data3, _ := json.Marshal(Message{Role: "user", Content: "hi"})
	mixed = append(mixed, WireEvent{Type: UserMessage, Seq: 3, At: base, Data: data3})
	for i := 0; i < 4; i++ {
		data4, _ := json.Marshal(ChunkFact{Turn: 1, Step: 1, Chunk: "c"})
		mixed = append(mixed, WireEvent{Type: AssistantChunk, Seq: uint64(4 + i), At: base, Data: data4})
	}
	packedMixed := PackChunkRuns(mixed)
	// First 2 chunks don't pack (run < MIN_RUN). UserMessage passes through.
	// Last 4 chunks pack into 1 row.
	if len(packedMixed) != 4 {
		t.Fatalf("expected 4 records (2 passthrough + 1 message + 1 row), got %d", len(packedMixed))
	}
}

// TestPackChunkRunsEmpty verifies an empty input produces no records.
func TestPackChunkRunsEmpty(t *testing.T) {
	packed := PackChunkRuns(nil)
	if len(packed) != 0 {
		t.Fatalf("expected 0 records, got %d", len(packed))
	}
}

// TestDecodeStorageRecordMalformedRow verifies that a malformed chunk row fails
// loud (does not silently drop data).
func TestDecodeStorageRecordMalformedRow(t *testing.T) {
	// Valid-looking but with mismatched dt/texts length.
	malformed := `{"type":"text-chunks","seq0":1,"time0":"2025-01-01T12:00:00Z","turn":1,"step":1,"dt":[1,2],"texts":["a","b"]}`
	_, err := DecodeStorageRecord([]byte(malformed))
	if err == nil {
		t.Fatal("expected error on malformed row (dt length mismatch)")
	}

	// Missing keys.
	malformed2 := `{"type":"text-chunks","seq0":1}`
	_, err = DecodeStorageRecord([]byte(malformed2))
	if err == nil {
		t.Fatal("expected error on missing keys")
	}

	// Non-chunk-row tag passes through as a single event.
	normal := `{"type":"message.user","seq":1,"at":"2025-01-01T12:00:00Z","data":{"role":"user","content":"hi"}}`
	decoded, err := DecodeStorageRecord([]byte(normal))
	if err != nil {
		t.Fatalf("DecodeStorageRecord: %v", err)
	}
	if len(decoded) != 1 {
		t.Fatalf("expected 1 event, got %d", len(decoded))
	}
	if decoded[0].Type != UserMessage {
		t.Fatalf("expected UserMessage, got %s", decoded[0].Type)
	}
}

// TestChunkRowRoundTripFullPipeline verifies PackChunkRuns → Marshal → DecodeStorageRecord
// produces the exact original events, including mixed content.
func TestChunkRowRoundTripFullPipeline(t *testing.T) {
	base := time.Date(2025, 6, 15, 10, 30, 0, 0, time.UTC)
	var events []WireEvent

	// A user message (non-chunk).
	data, _ := json.Marshal(Message{Role: "user", Content: "Hello"})
	events = append(events, WireEvent{Type: UserMessage, Seq: 1, At: base, Data: data})

	// 5 assistant chunks (same turn/step, should pack).
	for i := 0; i < 5; i++ {
		data, _ := json.Marshal(ChunkFact{Turn: 1, Step: 1, Chunk: chunkText(i)})
		events = append(events, WireEvent{
			Type: AssistantChunk, Seq: uint64(i + 2),
			At: base.Add(time.Duration(i+1) * time.Millisecond), Data: data,
		})
	}

	// A tool call (non-chunk).
	tcData, _ := json.Marshal(ToolCallPayload{Name: "read", Arguments: map[string]any{"path": "/etc"}})
	events = append(events, WireEvent{Type: ToolCall, Seq: 7, At: base.Add(10 * time.Millisecond), Data: tcData})

	packed := PackChunkRuns(events)
	// user(1) + chunk-row(1) + tool-call(1) = 3 records
	if len(packed) != 3 {
		t.Fatalf("expected 3 records, got %d", len(packed))
	}

	// Marshal each record to JSONL-like lines and decode back.
	var rebuilt []WireEvent
	for _, rec := range packed {
		var raw json.RawMessage
		if rec.ChunkRow != nil {
			raw, _ = json.Marshal(rec.ChunkRow)
		} else {
			raw, _ = json.Marshal(rec.Event)
		}
		decoded, err := DecodeStorageRecord(raw)
		if err != nil {
			t.Fatalf("DecodeStorageRecord: %v", err)
		}
		rebuilt = append(rebuilt, decoded...)
	}

	if len(rebuilt) != len(events) {
		t.Fatalf("rebuilt %d events, want %d", len(rebuilt), len(events))
	}
	for i, ev := range rebuilt {
		if ev.Type != events[i].Type {
			t.Errorf("event %d: type = %s, want %s", i, ev.Type, events[i].Type)
		}
		if ev.Seq != events[i].Seq {
			t.Errorf("event %d: seq = %d, want %d", i, ev.Seq, events[i].Seq)
		}
	}
}

// chunkText generates deterministic chunk text for testing.
func chunkText(i int) string {
	return string(rune('a' + i%26))
}

// TestPackChunkRunsThinkingChunks verifies that thinking (reasoning-delta)
// chunks pack into separate rows from text chunks, and round-trip correctly.
func TestPackChunkRunsThinkingChunks(t *testing.T) {
	base := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	var events []WireEvent

	// 3 text chunks (same turn/step)
	for i := 0; i < 3; i++ {
		data, _ := json.Marshal(ChunkFact{Turn: 1, Step: 1, Chunk: chunkText(i)})
		events = append(events, WireEvent{
			Type: AssistantChunk, Seq: uint64(i + 1),
			At: base.Add(time.Duration(i) * time.Millisecond), Data: data,
		})
	}
	// 3 thinking chunks (same turn/step, Kind="thinking")
	for i := 0; i < 3; i++ {
		data, _ := json.Marshal(ChunkFact{Turn: 1, Step: 1, Chunk: "thinking-" + chunkText(i), Kind: "thinking"})
		events = append(events, WireEvent{
			Type: AssistantChunk, Seq: uint64(i + 4),
			At: base.Add(time.Duration(i+3) * time.Millisecond), Data: data,
		})
	}

	packed := PackChunkRuns(events)
	// Should produce 2 rows: one for text, one for thinking.
	if len(packed) != 2 {
		t.Fatalf("expected 2 packed rows (text + thinking), got %d", len(packed))
	}
	if packed[0].ChunkRow == nil || packed[1].ChunkRow == nil {
		t.Fatal("both records should be chunk rows")
	}
	// Text row should have no kind, thinking row should have Kind="thinking".
	if packed[0].ChunkRow.Kind != "" {
		t.Errorf("text row Kind = %q, want empty", packed[0].ChunkRow.Kind)
	}
	if packed[1].ChunkRow.Kind != "thinking" {
		t.Errorf("thinking row Kind = %q, want \"thinking\"", packed[1].ChunkRow.Kind)
	}

	// Round-trip: decode both rows and verify Kind is preserved.
	var rebuilt []WireEvent
	for _, rec := range packed {
		raw, _ := json.Marshal(rec.ChunkRow)
		decoded, err := DecodeStorageRecord(raw)
		if err != nil {
			t.Fatalf("DecodeStorageRecord: %v", err)
		}
		rebuilt = append(rebuilt, decoded...)
	}
	if len(rebuilt) != len(events) {
		t.Fatalf("rebuilt %d events, want %d", len(rebuilt), len(events))
	}
	// First 3 should be text (Kind="")
	for i := 0; i < 3; i++ {
		var f ChunkFact
		json.Unmarshal(rebuilt[i].Data, &f)
		if f.Kind != "" {
			t.Errorf("text chunk %d: Kind = %q, want empty", i, f.Kind)
		}
	}
	// Next 3 should be thinking (Kind="thinking")
	for i := 3; i < 6; i++ {
		var f ChunkFact
		json.Unmarshal(rebuilt[i].Data, &f)
		if f.Kind != "thinking" {
			t.Errorf("thinking chunk %d: Kind = %q, want \"thinking\"", i, f.Kind)
		}
	}
}
