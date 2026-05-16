package session

import (
	"encoding/json"
	"testing"
)

func FuzzParseMessage(f *testing.F) {
	f.Add([]byte(`{"role":"user","content":"hello"}`))
	f.Add([]byte(`{"role":"assistant","content":"hi there"}`))
	f.Add([]byte(`{}`))
	f.Add([]byte(``))
	f.Add([]byte(`null`))
	f.Add([]byte(`{"role":"user","content":""}`))
	f.Add([]byte(`{"role":"invalid","content":"x"}`))
	f.Add([]byte(`{"role":"user","content":"` + string(make([]byte, 10000)) + `"}`))
	f.Add([]byte(`not json at all`))
	f.Add([]byte(`{"role":"user","tool_use":[{"id":"t1","name":"bash","input":{"command":"ls"}}]}`))

	f.Fuzz(func(t *testing.T, data []byte) {
		var msg Message
		err := json.Unmarshal(data, &msg)
		if err != nil {
			return
		}
		// If parsing succeeded, re-marshal should not panic
		out, err := json.Marshal(msg)
		if err != nil {
			t.Fatalf("re-marshal failed: %v", err)
		}
		// Round-trip: unmarshal the output
		var msg2 Message
		if err := json.Unmarshal(out, &msg2); err != nil {
			t.Fatalf("round-trip unmarshal failed: %v", err)
		}
		if msg.Role != msg2.Role {
			t.Errorf("round-trip role mismatch: %q vs %q", msg.Role, msg2.Role)
		}
		if msg.Content != msg2.Content {
			t.Errorf("round-trip content mismatch")
		}
	})
}

func FuzzParseSessionMeta(f *testing.F) {
	f.Add([]byte(`{"type":"session_meta","id":"abc","model":"gpt-4","provider":"openai"}`))
	f.Add([]byte(`{"type":"session_meta"}`))
	f.Add([]byte(`{}`))
	f.Add([]byte(`{"type":"not_meta"}`))

	f.Fuzz(func(t *testing.T, data []byte) {
		var raw map[string]interface{}
		if json.Unmarshal(data, &raw) != nil {
			return
		}
		// Should not panic regardless of input
		_ = raw["type"]
		_ = raw["id"]
		_ = raw["model"]
	})
}
