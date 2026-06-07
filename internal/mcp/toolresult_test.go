package mcp

import (
	"encoding/json"
	"testing"
)

func TestParseToolCallResult(t *testing.T) {
	tests := []struct {
		name     string
		raw      string
		wantText string
		wantErr  bool
	}{
		{
			name:     "success flattens text content",
			raw:      `{"content":[{"type":"text","text":"hello "},{"type":"text","text":"world"}]}`,
			wantText: "hello world",
			wantErr:  false,
		},
		{
			name:     "isError surfaces content as error",
			raw:      `{"content":[{"type":"text","text":"file not found"}],"isError":true}`,
			wantText: "file not found",
			wantErr:  true,
		},
		{
			name:     "isError with empty content gets a default message",
			raw:      `{"content":[],"isError":true}`,
			wantText: "remote MCP tool reported an error",
			wantErr:  true,
		},
		{
			name:     "isError false is a normal result",
			raw:      `{"content":[{"type":"text","text":"ok"}],"isError":false}`,
			wantText: "ok",
			wantErr:  false,
		},
		{
			name:     "non-text content is skipped",
			raw:      `{"content":[{"type":"image","text":""},{"type":"text","text":"caption"}]}`,
			wantText: "caption",
			wantErr:  false,
		},
		{
			name:     "undecodable result falls back to raw bytes",
			raw:      `not json`,
			wantText: "not json",
			wantErr:  false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			text, err := parseToolCallResult(json.RawMessage(tc.raw))
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr = %v", err, tc.wantErr)
			}
			if text != tc.wantText {
				t.Errorf("text = %q, want %q", text, tc.wantText)
			}
		})
	}
}
