// Chunk-packing compression for assistant.chunk events.
//
// Ported from DeepSeek Harness packages/core/session/src/chunk-rows.ts.
//
// When an LLM streams, the agent loop appends hundreds of near-identical
// assistant.chunk events whose JSON envelopes dwarf their payloads (~56×
// measured on a real DeepSeek session). This module packs each run of
// consecutive delta chunks into ONE storage row — text-chunks — and expands
// rows back to the exact original events on load.
//
// Storage rows are a durable-encoding vocabulary, NOT session events: they
// never enter the in-memory Event log, have no entry in the Type vocabulary,
// and use bare (dot-less, slash-less) type tags so a reader cannot confuse
// them with the event taxonomy. The packer whitelists exact shapes —
// anything it does not fully recognize is stored verbatim, so unknown fields
// or future chunk variants lose compression, never data. The decoder validates
// before expanding and fails loud on a malformed row-tagged value instead of
// silently dropping a whole run.
//
// DSH source: packages/core/session/src/chunk-rows.ts
package eventlog

import (
	"encoding/json"
	"fmt"
	"time"
)

// ChunkRowTag is the bare (non-event) type tag for a packed text-chunk row.
// It is NOT in the Type vocabulary, so scanJSONLLines in internal/session
// can distinguish storage rows from events and messages.
const ChunkRowTag = "text-chunks"

// chunkRowTag is the internal alias for ChunkRowTag.
const chunkRowTag = ChunkRowTag

// TextChunkRow is a packed run of consecutive assistant.chunk events that
// share the same Turn, Step, and Kind. seq0/time0 anchor the first member; the
// dt slice records epoch-millisecond gaps between consecutive members.
type TextChunkRow struct {
	// Type is always chunkRowTag.
	Type string `json:"type"`
	// Seq0 is the sequence number of the first member in the run.
	Seq0 uint64 `json:"seq0"`
	// Time0 is the timestamp of the first member.
	Time0 time.Time `json:"time0"`
	// Turn and Step are shared by all members in the run.
	Turn int `json:"turn"`
	Step int `json:"step"`
	// Kind is the chunk variant: "" (text, default) or "thinking".
	Kind string `json:"kind,omitempty"`
	// Dt is the slice of epoch-millisecond gaps between consecutive members.
	// len(Dt) == len(Texts) - 1. A gap may be negative when the clock stepped
	// backwards between events.
	Dt []int64 `json:"dt,omitempty"`
	// Texts is the list of chunk text strings, one per member.
	Texts []string `json:"texts"`
}

// StorageRecord is one durable log line: either a verbatim wire event or a
// packed chunk row. The discriminator is the "type" JSON field: bare tags
// (chunkRowTag) are rows; everything else is an event type string.
type StorageRecord struct {
	// Event is non-nil when this record is a verbatim event.
	Event *WireEvent
	// ChunkRow is non-nil when this record is a packed chunk run.
	ChunkRow *TextChunkRow
}

// minPackRun is the minimum consecutive same-turn/step chunks before a run
// packs. Below it a row's envelope rivals the event lines it replaces. A
// format constant, not a tunable: both layouts decode identically.
const minPackRun = 3

// chunkFactKeys is the exact key set a text-only packable ChunkFact must have.
// Thinking chunks add one key ("kind"), so classifyChunk checks both shapes.
var chunkFactKeys = []string{"turn", "step", "chunk"}

// classifyChunk determines whether an assistant.chunk WireEvent can enter a
// text-chunk pack. Graycode's ChunkFact is text-only (no index), so every
// non-empty ChunkFact with the canonical three keys (text) or four keys
// (text+kind) is packable. Returns nil for events that cannot pack (empty
// chunk, non-canonical keys, or non-chunk types).
func classifyChunk(w WireEvent) *ChunkFact {
	if w.Type != AssistantChunk {
		return nil
	}
	if len(w.Data) == 0 {
		return nil
	}
	var p ChunkFact
	if err := json.Unmarshal(w.Data, &p); err != nil {
		return nil
	}
	if p.Chunk == "" {
		return nil
	}
	// Structural exact-key check: the packer only packs events whose envelope
	// is exactly {turn, step, chunk} (text) or {turn, step, chunk, kind}
	// (thinking). The optional "kind" key must be the only extra key, and its
	// value must be "" or "thinking". Anything else passes through verbatim.
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(w.Data, &raw); err != nil {
		return nil
	}
	if len(raw) != len(chunkFactKeys) && len(raw) != len(chunkFactKeys)+1 {
		return nil
	}
	for _, k := range chunkFactKeys {
		if _, ok := raw[k]; !ok {
			return nil
		}
	}
	if len(raw) == len(chunkFactKeys)+1 {
		if _, ok := raw["kind"]; !ok {
			return nil // the only allowed extra key is "kind"
		}
		if p.Kind != "" && p.Kind != "thinking" {
			return nil
		}
	}
	return &p
}

// continuesTextRun checks whether next extends a run ending in prev.
// Both must be non-nil packable chunks, consecutive in seq, same turn/step,
// and the time gap must be a safe integer (exact millisecond arithmetic).
func continuesTextRun(prev, next WireEvent, prevFact, nextFact *ChunkFact) bool {
	if nextFact == nil {
		return false
	}
	if next.Seq != prev.Seq+1 {
		return false
	}
	if nextFact.Turn != prevFact.Turn || nextFact.Step != prevFact.Step {
		return false
	}
	gap := next.At.UnixMilli() - prev.At.UnixMilli()
	if gap < -(1<<62) || gap > (1<<62) {
		return false
	}
	return true
}

// buildTextRow constructs a TextChunkRow from a completed run.
func buildTextRow(run []WireEvent, facts []*ChunkFact) *TextChunkRow {
	first := facts[0]
	dt := make([]int64, len(run)-1)
	for i := 1; i < len(run); i++ {
		dt[i-1] = run[i].At.UnixMilli() - run[i-1].At.UnixMilli()
	}
	texts := make([]string, len(run))
	for i, f := range facts {
		texts[i] = f.Chunk
	}
	return &TextChunkRow{
		Type:  chunkRowTag,
		Seq0:  run[0].Seq,
		Time0: run[0].At,
		Turn:  first.Turn,
		Step:  first.Step,
		Kind:  first.Kind,
		Dt:    dt,
		Texts: texts,
	}
}

// PackChunkRuns is the storage encoding pass: each run of at least minPackRun
// consecutive same-turn/step assistant.chunk events becomes one
// TextChunkRow; every other WireEvent passes through verbatim, in order. Pure
// and stateless — safe over any slice, including one whose runs were split by
// flush boundaries (split runs simply pack per batch).
func PackChunkRuns(events []WireEvent) []StorageRecord {
	out := make([]StorageRecord, 0, len(events))
	var kindFact *ChunkFact
	var run []WireEvent
	var runFacts []*ChunkFact

	flush := func() {
		if len(run) >= minPackRun {
			out = append(out, StorageRecord{ChunkRow: buildTextRow(run, runFacts)})
		} else {
			for i := range run {
				out = append(out, StorageRecord{Event: &run[i]})
			}
		}
		kindFact = nil
		run = nil
		runFacts = nil
	}

	for i := range events {
		ev := events[i]
		f := classifyChunk(ev)
		if f == nil {
			flush()
			out = append(out, StorageRecord{Event: &events[i]})
			continue
		}
		if kindFact != nil && continuesTextRun(run[len(run)-1], ev, kindFact, f) &&
			f.Turn == runFacts[0].Turn && f.Step == runFacts[0].Step &&
			f.Kind == runFacts[0].Kind { // text and thinking chunks pack separately
			run = append(run, ev)
			runFacts = append(runFacts, f)
			continue
		}
		flush()
		kindFact = f
		run = append(run, ev)
		runFacts = append(runFacts, f)
	}
	flush()
	return out
}

// malformedRow throws the uniform malformed-row diagnostic.
func malformedRow(why string) error {
	return fmt.Errorf("malformed %s storage row: %s", chunkRowTag, why)
}

// validateRow validates a parsed TextChunkRow, returning it or an error.
// The "kind" key is optional (backward compat with text-only rows that lack it).
func validateRow(value map[string]json.RawMessage) (*TextChunkRow, error) {
	expectedKeys := []string{"type", "seq0", "time0", "turn", "step", "dt", "texts"}
	optionalKeys := []string{"kind"}
	totalKeys := len(expectedKeys) + len(optionalKeys)
	if len(value) < len(expectedKeys) || len(value) > totalKeys {
		return nil, malformedRow(fmt.Sprintf("envelope must be exactly {%s} with optional %s", joinKeys(expectedKeys), joinKeys(optionalKeys)))
	}
	for _, k := range expectedKeys {
		if _, ok := value[k]; !ok {
			return nil, malformedRow(fmt.Sprintf("missing key %q", k))
		}
	}

	var row TextChunkRow
	if err := json.Unmarshal(value["type"], &row.Type); err != nil {
		return nil, malformedRow("type must be " + chunkRowTag)
	}
	if row.Type != chunkRowTag {
		return nil, malformedRow("type must be " + chunkRowTag)
	}
	if err := json.Unmarshal(value["seq0"], &row.Seq0); err != nil {
		return nil, malformedRow("seq0 must be a non-negative integer")
	}
	if err := json.Unmarshal(value["time0"], &row.Time0); err != nil {
		return nil, malformedRow("time0 must be a valid timestamp")
	}
	if err := json.Unmarshal(value["turn"], &row.Turn); err != nil {
		return nil, malformedRow("turn must be a number")
	}
	if err := json.Unmarshal(value["step"], &row.Step); err != nil {
		return nil, malformedRow("step must be a number")
	}
	if kindRaw, ok := value["kind"]; ok {
		if err := json.Unmarshal(kindRaw, &row.Kind); err != nil {
			return nil, malformedRow("kind must be a string")
		}
		if row.Kind != "" && row.Kind != "thinking" {
			return nil, malformedRow("kind must be empty or \"thinking\"")
		}
	}
	if err := json.Unmarshal(value["dt"], &row.Dt); err != nil {
		return nil, malformedRow("dt must be an array of integers")
	}
	if err := json.Unmarshal(value["texts"], &row.Texts); err != nil {
		return nil, malformedRow("texts must be a non-empty string array")
	}

	if len(row.Texts) == 0 {
		return nil, malformedRow("texts must be non-empty")
	}
	for _, t := range row.Texts {
		if t == "" {
			return nil, malformedRow("texts must not contain empty strings")
		}
	}
	for _, gap := range row.Dt {
		if gap < -(1<<62) || gap > (1<<62) {
			return nil, malformedRow("dt gaps must be safe integer milliseconds")
		}
	}
	if len(row.Dt) != len(row.Texts)-1 {
		return nil, malformedRow(fmt.Sprintf("dt length %d does not match %d members", len(row.Dt), len(row.Texts)))
	}
	// Reconstruction bounds: seq0 + len(texts) - 1 must stay a safe uint64.
	if row.Seq0 > ^uint64(0)-uint64(len(row.Texts)-1) {
		return nil, malformedRow("member seqs must stay within safe range")
	}
	// Time reconstruction: each successive time = time0 + sum of gaps so far.
	t := row.Time0
	for _, gap := range row.Dt {
		t = t.Add(time.Duration(gap) * time.Millisecond)
	}
	_ = t

	return &row, nil
}

// expandRow expands a validated TextChunkRow back into its exact original
// WireEvent slice, in order.
func expandRow(row *TextChunkRow) []WireEvent {
	out := make([]WireEvent, len(row.Texts))
	t := row.Time0
	for k := range row.Texts {
		if k > 0 {
			t = t.Add(time.Duration(row.Dt[k-1]) * time.Millisecond)
		}
		fact := ChunkFact{Turn: row.Turn, Step: row.Step, Chunk: row.Texts[k], Kind: row.Kind}
		data, _ := json.Marshal(fact)
		out[k] = WireEvent{
			Type: AssistantChunk,
			Seq:  row.Seq0 + uint64(k),
			At:   t,
			Data: data,
		}
	}
	return out
}

// DecodeStorageRecord decodes one parsed JSONL line value into the WireEvents
// it stores. A chunk-row-tagged value validates and expands (a malformed row
// returns an error — it is corrupt storage and treating it as a single event
// would silently drop a whole run); every other value is returned as-is when
// it is a valid WireEvent, or returned as a single WireEvent without validation
// (validation happens later in DecodeWire/Validate).
//
// In practice, the persistence layer calls DecodeWire (which calls Validate)
// after collecting all events. This function only handles the chunk-row
// expansion; unknown type tags pass through as single events.
func DecodeStorageRecord(value json.RawMessage) ([]WireEvent, error) {
	var probe struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(value, &probe); err != nil {
		return nil, fmt.Errorf("storage record: parse type: %w", err)
	}

	// Bare row tags are chunk-pack rows.
	if probe.Type == ChunkRowTag {
		var raw map[string]json.RawMessage
		if err := json.Unmarshal(value, &raw); err != nil {
			return nil, malformedRow("not a JSON object")
		}
		row, err := validateRow(raw)
		if err != nil {
			return nil, err
		}
		return expandRow(row), nil
	}

	// Regular event line: decode as a single WireEvent (payload validation
	// happens later in DecodeWire).
	var ev WireEvent
	if err := json.Unmarshal(value, &ev); err != nil {
		return nil, fmt.Errorf("storage record: parse event: %w", err)
	}
	return []WireEvent{ev}, nil
}

// IsStorageRecord reports whether a type tag identifies a chunk-packing
// storage row (as produced by PackChunkRuns). Storage rows use bare tags that
// are NOT in the Type vocabulary, so this is the discriminator the persistence
// layer uses to decide whether to expand a line or decode it as a single event.
func IsStorageRecord(tag string) bool {
	return tag == ChunkRowTag
}

// joinKeys joins a slice of strings for error messages.
func joinKeys(keys []string) string {
	out := ""
	for i, k := range keys {
		if i > 0 {
			out += ", "
		}
		out += k
	}
	return out
}
