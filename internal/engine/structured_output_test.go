package engine

import (
	"context"
	"testing"

	"github.com/GrayCodeAI/graycode-cli/internal/types"
)

const personSchema = `{
	"type": "object",
	"required": ["name", "age"],
	"properties": {
		"name": {"type": "string"},
		"age": {"type": "integer"}
	}
}`

func TestValidateAgainstSchema(t *testing.T) {
	tests := []struct {
		name    string
		content string
		schema  string
		wantErr bool
	}{
		{name: "valid object", content: `{"name":"ada","age":36}`, schema: personSchema},
		{name: "valid with fence", content: "```json\n{\"name\":\"ada\",\"age\":36}\n```", schema: personSchema},
		{name: "missing required", content: `{"name":"ada"}`, schema: personSchema, wantErr: true},
		{name: "wrong type", content: `{"name":123,"age":36}`, schema: personSchema, wantErr: true},
		{name: "non-integer age", content: `{"name":"ada","age":3.5}`, schema: personSchema, wantErr: true},
		{name: "not json", content: `hello world`, schema: personSchema, wantErr: true},
		{name: "empty", content: ``, schema: personSchema, wantErr: true},
		{name: "unparseable schema passes", content: `{"x":1}`, schema: `{not json`, wantErr: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateAgainstSchema(tt.content, tt.schema)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ValidateAgainstSchema() err=%v, wantErr=%v", err, tt.wantErr)
			}
		})
	}
}

func TestChatStructured_NoSchema(t *testing.T) {
	mc := newMockClient(mockTextResponse("anything goes"))
	s := NewSession("test", "test-model", "", nil)
	s.SetTestClient(mc)

	resp, err := s.ChatStructured(context.Background(),
		[]types.EyrieMessage{{Role: "user", Content: "hi"}}, types.ChatOptions{}, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Content != "anything goes" {
		t.Fatalf("got %q", resp.Content)
	}
	if mc.callCount() != 1 {
		t.Fatalf("expected 1 call, got %d", mc.callCount())
	}
}

func TestChatStructured_ValidFirstTry(t *testing.T) {
	mc := newMockClient(mockTextResponse(`{"name":"ada","age":36}`))
	s := NewSession("test", "test-model", "", nil)
	s.SetTestClient(mc)

	_, err := s.ChatStructured(context.Background(),
		[]types.EyrieMessage{{Role: "user", Content: "describe ada"}}, types.ChatOptions{}, personSchema)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mc.callCount() != 1 {
		t.Fatalf("expected no retry (1 call), got %d", mc.callCount())
	}
}

func TestChatStructured_RetryOnceThenSucceeds(t *testing.T) {
	// First response is invalid (missing age), second is valid.
	mc := newMockClient(
		mockTextResponse(`{"name":"ada"}`),
		mockTextResponse(`{"name":"ada","age":36}`),
	)
	s := NewSession("test", "test-model", "", nil)
	s.SetTestClient(mc)

	resp, err := s.ChatStructured(context.Background(),
		[]types.EyrieMessage{{Role: "user", Content: "describe ada"}}, types.ChatOptions{}, personSchema)
	if err != nil {
		t.Fatalf("unexpected error after retry: %v", err)
	}
	if resp.Content != `{"name":"ada","age":36}` {
		t.Fatalf("got %q", resp.Content)
	}
	if mc.callCount() != 2 {
		t.Fatalf("expected exactly one retry (2 calls), got %d", mc.callCount())
	}
}

func TestChatStructured_RetryOnceThenReturnsError(t *testing.T) {
	// Both responses invalid; should retry exactly once then return the error.
	mc := newMockClient(
		mockTextResponse(`{"name":"ada"}`),
		mockTextResponse(`{"name":"grace"}`),
	)
	s := NewSession("test", "test-model", "", nil)
	s.SetTestClient(mc)

	resp, err := s.ChatStructured(context.Background(),
		[]types.EyrieMessage{{Role: "user", Content: "describe"}}, types.ChatOptions{}, personSchema)
	if err == nil {
		t.Fatal("expected schema error after failed retry")
	}
	if resp == nil || resp.Content != `{"name":"grace"}` {
		t.Fatalf("expected last (retry) response returned, got %+v", resp)
	}
	if mc.callCount() != 2 {
		t.Fatalf("expected exactly one retry (2 calls), got %d", mc.callCount())
	}
}
