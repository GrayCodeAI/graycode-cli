// Package engine — headless JSONL event output for CI.
//
// The daemon already streams SSE events over its HTTP API. This file exposes
// the same shaped event on stdout as newline-delimited JSON so that `graycode
// print`/`-p` (and any future `--output-format jsonl` invocation) can feed
// machine-readable transcript lines into CI pipelines, mirroring
// `codex exec --json` / `claude -p --output-format json`.
//
// Each emitted line is a JSON object with a stable envelope:
//
//	{"type":"content","content":"...","turn":3}
//	{"type":"tool_use","tool":"Read","input":{...},"turn":3}
//	{"type":"tool_result","tool":"Read","content":"...","turn":3}
//	{"type":"usage","prompt_tokens":100,"completion_tokens":32,"total_tokens":132,"turn":3}
//	{"type":"done","stop_reason":"end_turn","turn":3}
//	{"type":"error","error":"...","turn":3}
package engine

import (
	"encoding/json"
	"fmt"
	"io"
	"sync"
)

// JSONLEventWriter writes typed engine events to an io.Writer as newline-delimited
// JSON. It is concurrency-safe so the agent loop can emit from multiple
// goroutines (e.g. tool-result fan-out) without interleaving lines.
type JSONLEventWriter struct {
	mu  *sync.Mutex
	out io.Writer
	// turn tracks which agent turn the current event belongs to; callers bump
	// it via WithTurn so emitted lines carry a stable turn index.
	turn int
}

// NewJSONLEventWriter wraps w so every WriteEvent call produces one JSON line.
func NewJSONLEventWriter(w io.Writer) *JSONLEventWriter {
	return &JSONLEventWriter{mu: &sync.Mutex{}, out: w}
}

// WithTurn returns a derived writer that stamps emitted lines with the given
// turn index. The receiver is not mutated; the underlying mutex and writer are
// shared so concurrent calls (across derived writers) stay synchronized.
func (j *JSONLEventWriter) WithTurn(turn int) *JSONLEventWriter {
	return &JSONLEventWriter{mu: j.mu, out: j.out, turn: turn}
}

// Event is the typed payload emitted per line. The Type field discriminates the
// shape of Data.
type JSONLEvent struct {
	Type string `json:"type"`
	Turn int    `json:"turn,omitempty"`
	Data any    `json:"data"`
}

// WriteEvent emits one JSON line for evt. It is the single hot path; keep it
// allocation-light (no struct copy of Data — it's an interface value).
func (j *JSONLEventWriter) WriteEvent(evt JSONLEvent) error {
	evt.Turn = j.turn
	line, err := json.Marshal(evt)
	if err != nil {
		return err
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	// One write per line; io.Writer.Write is called once so a concurrent
	// reader of the pipe never sees a partial JSON object.
	_, werr := j.out.Write(append(line, '\n'))
	return werr
}

// Emit convenience helpers for the common streams.

func (j *JSONLEventWriter) Content(turn int, s string) error {
	return j.WithTurn(turn).WriteEvent(JSONLEvent{Type: "content", Data: StringPayload{Value: s}})
}

func (j *JSONLEventWriter) ToolUse(turn int, name string, input any) error {
	return j.WithTurn(turn).WriteEvent(JSONLEvent{Type: "tool_use", Data: ToolUsePayload{Tool: name, Input: input}})
}

func (j *JSONLEventWriter) ToolResult(turn int, name, result string) error {
	return j.WithTurn(turn).WriteEvent(JSONLEvent{Type: "tool_result", Data: ToolResultPayload{Tool: name, Content: result}})
}

func (j *JSONLEventWriter) Usage(turn int, prompt, completion, cacheRead, cacheWrite int, provider, model string) error {
	return j.WithTurn(turn).WriteEvent(JSONLEvent{
		Type: "usage",
		Data: UsagePayload{
			PromptTokens:     prompt,
			CompletionTokens: completion,
			CacheReadTokens:  cacheRead,
			CacheWriteTokens: cacheWrite,
			Provider:         provider,
			Model:            model,
		},
	})
}

func (j *JSONLEventWriter) Done(turn int, stopReason string) error {
	return j.WithTurn(turn).WriteEvent(JSONLEvent{Type: "done", Data: StringPayload{Value: stopReason}})
}

func (j *JSONLEventWriter) Error(turn int, err string) error {
	return j.WithTurn(turn).WriteEvent(JSONLEvent{Type: "error", Data: StringPayload{Value: err}})
}

// Payloads mirror the daemon's SSE envelopes so a single consumer shape works
// for both transport layers.

type StringPayload struct {
	Value string `json:"value"`
}

type ToolUsePayload struct {
	Tool  string `json:"tool"`
	Input any    `json:"input"`
}

type ToolResultPayload struct {
	Tool    string `json:"tool"`
	Content string `json:"content"`
}

type UsagePayload struct {
	PromptTokens     int    `json:"prompt_tokens"`
	CompletionTokens int    `json:"completion_tokens"`
	CacheReadTokens  int    `json:"cache_read_tokens,omitempty"`
	CacheWriteTokens int    `json:"cache_write_tokens,omitempty"`
	Provider         string `json:"provider,omitempty"`
	Model            string `json:"model,omitempty"`
}

// WriteEventLine writes a pre-marshalled line directly — for the rare path
// that already has a fully-formed StreamEvent from the daemon and wants to
// re-emit it as JSONL on stdout without re-marshalling.
func (j *JSONLEventWriter) WriteEventLine(turn int, line string) error {
	if len(line) == 0 {
		return nil
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	_, err := fmt.Fprintf(j.out, `{"type":"line","turn":%d,"data":%s}`+"\n", turn, line)
	return err
}
