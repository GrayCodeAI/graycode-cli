package acp

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/GrayCodeAI/hawk/internal/engine"
	"github.com/GrayCodeAI/hawk/internal/tool"
)

// testFactory builds a session backed by the engine's canned mock chat client,
// so prompts complete without a real provider.
func testFactory() (*engine.Session, error) {
	registry := tool.NewRegistry(tool.FileReadTool{})
	s := engine.NewSession("test", "mock-model", "test", registry)
	s.SetTestClient(engine.NewMockClientForTest())
	return s, nil
}

// runServer drives Serve with the given input lines and returns all output
// messages it wrote.
func runServer(t *testing.T, factory SessionFactory, inputLines []string) []rpcMessage {
	t.Helper()
	in := strings.NewReader(strings.Join(inputLines, "\n") + "\n")
	pr, pw := io.Pipe()

	srv := NewServer(factory)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_ = srv.Serve(ctx, in, pw)
		_ = pw.Close()
	}()

	var msgs []rpcMessage
	scanner := bufio.NewScanner(pr)
	scanner.Buffer(make([]byte, 0, 1024*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var m rpcMessage
		if err := json.Unmarshal(line, &m); err == nil {
			msgs = append(msgs, m)
		}
	}
	wg.Wait()
	return msgs
}

func TestACP_InitializeAndPrompt(t *testing.T) {
	lines := []string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":1}}`,
		`{"jsonrpc":"2.0","id":2,"method":"session/new","params":{}}`,
		`{"jsonrpc":"2.0","id":3,"method":"session/prompt","params":{"sessionId":"sess_1","prompt":[{"type":"text","text":"hello"}]}}`,
	}
	msgs := runServer(t, testFactory, lines)

	var gotInit, gotNew, gotUpdate, gotPromptResult bool
	for _, m := range msgs {
		switch {
		case hasID(m, 1):
			gotInit = true
			var r struct {
				ProtocolVersion int `json:"protocolVersion"`
			}
			_ = json.Unmarshal(m.Result, &r)
			if r.ProtocolVersion != ProtocolVersion {
				t.Errorf("initialize protocolVersion = %d, want %d", r.ProtocolVersion, ProtocolVersion)
			}
		case hasID(m, 2):
			gotNew = true
			var r struct {
				SessionID string `json:"sessionId"`
			}
			_ = json.Unmarshal(m.Result, &r)
			if r.SessionID == "" {
				t.Error("session/new returned empty sessionId")
			}
		case m.Method == "session/update":
			gotUpdate = true
		case hasID(m, 3):
			gotPromptResult = true
			var r struct {
				StopReason string `json:"stopReason"`
			}
			_ = json.Unmarshal(m.Result, &r)
			if r.StopReason != "end_turn" {
				t.Errorf("prompt stopReason = %q, want end_turn", r.StopReason)
			}
		}
	}
	if !gotInit || !gotNew || !gotUpdate || !gotPromptResult {
		t.Errorf("missing responses: init=%v new=%v update=%v promptResult=%v", gotInit, gotNew, gotUpdate, gotPromptResult)
	}
}

func TestACP_UnknownMethod(t *testing.T) {
	msgs := runServer(t, testFactory, []string{
		`{"jsonrpc":"2.0","id":1,"method":"does/not/exist","params":{}}`,
	})
	if len(msgs) != 1 || msgs[0].Error == nil || msgs[0].Error.Code != errCodeMethodNotFound {
		t.Fatalf("expected method-not-found error, got %+v", msgs)
	}
}

func TestACP_PromptUnknownSession(t *testing.T) {
	msgs := runServer(t, testFactory, []string{
		`{"jsonrpc":"2.0","id":1,"method":"session/prompt","params":{"sessionId":"nope","prompt":[{"type":"text","text":"hi"}]}}`,
	})
	if len(msgs) != 1 || msgs[0].Error == nil || msgs[0].Error.Code != errCodeInvalidParams {
		t.Fatalf("expected invalid-params error for unknown session, got %+v", msgs)
	}
}

func TestACP_ParseError(t *testing.T) {
	msgs := runServer(t, testFactory, []string{`{not valid json`})
	if len(msgs) != 1 || msgs[0].Error == nil || msgs[0].Error.Code != errCodeParseError {
		t.Fatalf("expected parse error, got %+v", msgs)
	}
}

func hasID(m rpcMessage, id int) bool {
	if len(m.ID) == 0 {
		return false
	}
	var got int
	if err := json.Unmarshal(m.ID, &got); err != nil {
		return false
	}
	return got == id
}
