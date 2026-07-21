package engine

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/GrayCodeAI/hawk-core-contracts/llm"
	"github.com/GrayCodeAI/hawk/internal/resilience/retry"
	"github.com/GrayCodeAI/hawk/internal/types"
)

// TestChatService_BuildOptions checks that BuildOptions correctly
// translates the service config into a types.ChatOptions.
func TestChatService_BuildOptions(t *testing.T) {
	svc := NewChatService(NewMockClientForTest(), ChatServiceConfig{
		Provider: "anthropic",
		Model:    "claude-opus-4",
	})
	opts := svc.BuildOptions("you are hawk", "claude-opus-4", 4096, nil)
	if opts.Provider != "anthropic" {
		t.Errorf("expected provider=anthropic, got %q", opts.Provider)
	}
	if opts.Model != "claude-opus-4" {
		t.Errorf("expected model=claude-opus-4, got %q", opts.Model)
	}
	if opts.MaxTokens != 4096 {
		t.Errorf("expected MaxTokens=4096, got %d", opts.MaxTokens)
	}
	if !opts.EnableCaching {
		t.Error("expected EnableCaching=true for anthropic")
	}
	if opts.System != "you are hawk" {
		t.Errorf("expected system prompt to be set, got %q", opts.System)
	}
}

func TestChatService_BuildOptions_NonAnthropicCaching(t *testing.T) {
	svc := NewChatService(NewMockClientForTest(), ChatServiceConfig{Provider: "openai", Model: "gpt-4o"})
	opts := svc.BuildOptions("system", "gpt-4o", 1024, nil)
	if opts.EnableCaching {
		t.Error("EnableCaching should be false for non-anthropic provider")
	}
}

func TestChatService_BuildOptions_GLMThinking(t *testing.T) {
	enabled := true
	svc := NewChatService(NewMockClientForTest(), ChatServiceConfig{
		Provider:           "zai_payg",
		Model:              "glm-4",
		GLMThinkingEnabled: &enabled,
	})
	opts := svc.BuildOptions("sys", "glm-4", 1024, nil)
	if opts.GLMThinkingEnabled == nil || !*opts.GLMThinkingEnabled {
		t.Error("expected GLMThinkingEnabled=true for zai_payg")
	}
	// Sanity: setting GLMThinkingEnabled on a non-zai provider is ignored.
	svc2 := NewChatService(NewMockClientForTest(), ChatServiceConfig{Provider: "openai", GLMThinkingEnabled: &enabled})
	opts2 := svc2.BuildOptions("sys", "gpt-4o", 1024, nil)
	if opts2.GLMThinkingEnabled != nil {
		t.Error("GLMThinkingEnabled should be nil for non-zai provider")
	}
}

func TestChatService_BuildOptions_OutputSchema(t *testing.T) {
	svc := NewChatService(NewMockClientForTest(), ChatServiceConfig{
		Provider:     "anthropic",
		Model:        "claude-opus-4",
		OutputSchema: `{"type":"object"}`,
	})
	opts := svc.BuildOptions("sys", "claude-opus-4", 1024, nil)
	if opts.ResponseFormat == nil || opts.ResponseFormat.Type != "json_schema" {
		t.Errorf("expected json_schema response format, got %+v", opts.ResponseFormat)
	}
}

func TestChatService_Reattach(t *testing.T) {
	oldClient := NewMockClientForTest()
	newClient := NewMockClientForTest()
	svc := NewChatService(oldClient, ChatServiceConfig{Provider: "anthropic", Model: "claude-opus-4"})
	// Reattach with a nil client should be a no-op (preserve current).
	svc.Reattach(nil, "")
	if svc.Client() != oldClient {
		t.Error("Reattach(nil, \"\") should be a no-op")
	}
	// Reattach with a real client should swap and update provider.
	svc.Reattach(newClient, "openai")
	if svc.Provider() != "openai" {
		t.Errorf("expected provider=openai, got %q", svc.Provider())
	}
}

func TestChatService_DefaultsApplied(t *testing.T) {
	// Zero config — only client is required.
	svc := NewChatService(NewMockClientForTest(), ChatServiceConfig{})
	if svc.retryCfg.MaxRetries == 0 {
		t.Error("expected default retry config to be set")
	}
	if svc.contCfg.MaxContinuations == 0 {
		t.Error("expected default continuation config to be set")
	}
	if svc.metrics == nil {
		t.Error("expected default metrics registry")
	}
}

func TestChatService_ChatDelegatesToClient(t *testing.T) {
	svc := NewChatService(NewMockClientForTest(), ChatServiceConfig{
		Provider: "anthropic",
		Model:    "claude-opus-4",
	})
	resp, err := svc.Chat(
		context.Background(),
		[]types.EyrieMessage{{Role: "user", Content: "hi"}},
		svc.BuildOptions("sys", "claude-opus-4", 1024, nil),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Content != "mock test response" {
		t.Errorf("expected mock content, got %q", resp.Content)
	}
}

// errClient is a ChatClient that always fails. Used to verify that
// ChatService.Chat surfaces the underlying error unchanged.
type errClient struct{ err error }

func (e *errClient) Chat(_ context.Context, _ []types.EyrieMessage, _ types.ChatOptions) (*types.EyrieResponse, error) {
	return nil, e.err
}

func (e *errClient) StreamChatContinue(_ context.Context, _ []types.EyrieMessage, _ types.ChatOptions, _ types.ContinuationConfig) (*types.StreamResult, error) {
	return nil, e.err
}

func TestChatService_ChatSurfacesError(t *testing.T) {
	want := errors.New("upstream kaput")
	svc := NewChatService(&errClient{err: want}, ChatServiceConfig{})
	_, err := svc.Chat(context.Background(), nil, types.ChatOptions{})
	if err == nil || err.Error() != want.Error() {
		t.Errorf("expected err %v, got %v", want, err)
	}
}

type resilienceManagingTestClient struct {
	err   error
	calls int
}

func (c *resilienceManagingTestClient) Chat(context.Context, []types.EyrieMessage, types.ChatOptions) (*types.EyrieResponse, error) {
	return nil, c.err
}

func (c *resilienceManagingTestClient) StreamChatContinue(context.Context, []types.EyrieMessage, types.ChatOptions, types.ContinuationConfig) (*types.StreamResult, error) {
	c.calls++
	return nil, c.err
}
func (c *resilienceManagingTestClient) ManagesResilience() bool { return true }

func TestChatService_DoesNotDuplicateEngineResilience(t *testing.T) {
	client := &resilienceManagingTestClient{err: errors.New("routed transport failed")}
	svc := NewChatService(client, ChatServiceConfig{})
	_, err := svc.Stream(context.Background(), []types.EyrieMessage{{Role: "user", Content: "hi"}}, types.ChatOptions{})
	if err == nil {
		t.Fatal("expected stream error")
	}
	if client.calls != 1 {
		t.Fatalf("engine-owning client called %d times, want exactly once", client.calls)
	}
}

type flakyLegacyStartClient struct {
	calls int
}

func (*flakyLegacyStartClient) Chat(context.Context, []types.EyrieMessage, types.ChatOptions) (*types.EyrieResponse, error) {
	return nil, nil
}

func (c *flakyLegacyStartClient) StreamChatContinue(context.Context, []types.EyrieMessage, types.ChatOptions, types.ContinuationConfig) (*types.StreamResult, error) {
	c.calls++
	if c.calls == 1 {
		return nil, errors.New("temporary transport failure")
	}
	events := make(chan types.EyrieStreamEvent)
	close(events)
	return llm.NewStreamResult(events, "", nil), nil
}

func TestChatService_LegacyClientRetainsStartRetry(t *testing.T) {
	client := &flakyLegacyStartClient{}
	svc := NewChatService(client, ChatServiceConfig{RetryConfig: retryConfigForBoundaryTest()})
	result, err := svc.Stream(context.Background(), nil, types.ChatOptions{})
	if err != nil {
		t.Fatalf("Stream() error = %v", err)
	}
	if result == nil {
		t.Fatal("Stream() returned nil result after compatibility retry")
	}
	if client.calls != 2 {
		t.Fatalf("legacy client calls = %d, want initial attempt plus one retry", client.calls)
	}
}

func retryConfigForBoundaryTest() retry.Config {
	return retry.Config{
		MaxRetries: 1,
		BaseDelay:  time.Nanosecond,
		MaxDelay:   time.Nanosecond,
		Multiplier: 1,
	}
}
