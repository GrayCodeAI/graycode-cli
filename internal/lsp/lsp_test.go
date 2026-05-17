package lsp

import (
	"encoding/json"
	"testing"
)

func TestServerManagerList(t *testing.T) {
	m := NewServerManager()
	if len(m.List()) != 0 {
		t.Fatal("expected empty list")
	}
	if m.IsRunning("test") {
		t.Fatal("expected server not running")
	}
}

func TestRequestMarshal(t *testing.T) {
	req := Request{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "initialize",
		Params: map[string]interface{}{
			"rootUri": "file:///test",
		},
	}
	data, err := json.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) == 0 {
		t.Fatal("expected non-empty JSON")
	}
}
