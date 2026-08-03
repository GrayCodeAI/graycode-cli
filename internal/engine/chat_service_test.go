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

func TestChatService_BuildOptions_ThinkingEnabled(t *testing.T) {
	enabled := true
	svc := NewChatService(NewMockClientForTest(), ChatServiceConfig{
		Provider:        "zai_payg",
		Model:           "glm-4",
		ThinkingEnabled: &enabled,
	})
	opts := svc.BuildOptions("sys", "glm-4", 1024, nil)
	if opts.ThinkingEnabled == nil || !*opts.ThinkingEnabled {
		t.Error("expected ThinkingEnabled=true for zai_payg")
	}
	if opts.GLMThinkingEnabled == nil || !*opts.GLMThinkingEnabled {
		t.Error("expected GLMThinkingEnabled alias=true for zai_payg")
	}
	// Sanity: setting ThinkingEnabled on a non-thinking provider is ignored.
	svc2 := NewChatService(NewMockClientForTest(), ChatServiceConfig{Provider: "openai", ThinkingEnabled: &enabled})
	opts2 := svc2.BuildOptions("sys", "gpt-4o", 1024, nil)
	if opts2.ThinkingEnabled != nil || opts2.GLMThinkingEnabled != nil {
		t.Error("ThinkingEnabled should be nil for non-thinking provider")
	}
	// LongCat / Agnes honor the generic toggle.
	svc3 := NewChatService(NewMockClientForTest(), ChatServiceConfig{Provider: "longcat", ThinkingEnabled: &enabled})
	opts3 := svc3.BuildOptions("sys", "LongCat-2.0", 1024, nil)
	if opts3.ThinkingEnabled == nil || !*opts3.ThinkingEnabled {
		t.Error("expected ThinkingEnabled=true for longcat")
	}
	svc4 := NewChatService(NewMockClientForTest(), ChatServiceConfig{Provider: "agnes", GLMThinkingEnabled: &enabled})
	opts4 := svc4.BuildOptions("sys", "agnes-2.0-flash", 1024, nil)
	if opts4.ThinkingEnabled == nil || !*opts4.ThinkingEnabled {
		t.Error("expected ThinkingEnabled=true for agnes via deprecated alias")
	}
	for _, provider := range []string{"kimi", "deepseek", "openrouter", "anthropic", "xiaomi_mimo_payg"} {
		svc := NewChatService(NewMockClientForTest(), ChatServiceConfig{Provider: provider, ThinkingEnabled: &enabled})
		opts := svc.BuildOptions("sys", "m", 1024, nil)
		if opts.ThinkingEnabled == nil || !*opts.ThinkingEnabled {
			t.Errorf("expected ThinkingEnabled=true for %s", provider)
		}
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

// overflowThenCaptureClient simulates a provider that rejects an oversized
// context on the first call, then succeeds — recording the transcript it
// received on the retry so tests can assert the emergency compact (H3).
type overflowThenCaptureClient struct {
	calls   int
	seen    []types.EyrieMessage
	started bool
}

func (*overflowThenCaptureClient) Chat(context.Context, []types.EyrieMessage, types.ChatOptions) (*types.EyrieResponse, error) {
	return nil, nil
}

func (c *overflowThenCaptureClient) StreamChatContinue(_ context.Context, messages []types.EyrieMessage, _ types.ChatOptions, _ types.ContinuationConfig) (*types.StreamResult, error) {
	c.calls++
	if c.calls == 1 {
		return nil, errors.New("input too long: 120000 tokens exceeds the limit of 100000")
	}
	c.started = true
	c.seen = append([]types.EyrieMessage(nil), messages...)
	return &types.StreamResult{}, nil
}

// TestChatService_EmergencyCompactTrimsBeforeRetry verifies the H3 fix: on a
// context-overflow error, the retry sends a compacted (smaller) transcript
// instead of re-sending the same overflowing messages.
func TestChatService_EmergencyCompactTrimsBeforeRetry(t *testing.T) {
	var messages []types.EyrieMessage
	messages = append(messages, types.EyrieMessage{Role: "system", Content: "sys"})
	for i := 0; i < 100; i++ {
		messages = append(messages, types.EyrieMessage{Role: "user", Content: "message-" + itoaForTest(i)})
	}

	client := &overflowThenCaptureClient{}
	svc := NewChatService(client, ChatServiceConfig{RetryConfig: retryConfigForBoundaryTest()})
	result, err := svc.Stream(context.Background(), messages, types.ChatOptions{})
	if err != nil {
		t.Fatalf("Stream() error = %v", err)
	}
	if !client.started {
		t.Fatal("expected the retry after overflow to reach the client")
	}
	if result == nil {
		t.Fatal("expected a result after successful retry")
	}
	if len(client.seen) >= len(messages) {
		t.Errorf("retry transcript length = %d, want < %d (compact must shrink it)", len(client.seen), len(messages))
	}
	foundLast := false
	for _, m := range client.seen {
		if m.Role == "user" && m.Content == "message-99" {
			foundLast = true
			break
		}
	}
	if !foundLast {
		t.Error("expected the retry transcript to preserve the most recent messages")
	}
}

func itoaForTest(i int) string {
	digits := []byte("0123456789")
	var b []byte
	for i >= 0 {
		b = append([]byte{digits[i%10]}, b...)
		i /= 10
		if i == 0 {
			break
		}
	}
	return string(b)
}

func TestIsContextOverflow(t *testing.T) {
	cases := []struct {
		err  error
		want bool
	}{
		{nil, false},
		{errors.New("input too long: 120000 tokens exceeds the limit of 100000"), true},
		{errors.New("context_length_exceeded: this model maximum context length"), true},
		{errors.New("context_length_error: exceeds the limit of 200000 tokens"), true},
		{errors.New("exceeds the limit of N tokens"), true},
		{errors.New("request timeout, too long"), false},
		{errors.New("the response was too long"), false},
		{errors.New("HTTP 503 unavailable"), false},
	}
	for _, c := range cases {
		if got := isContextOverflow(c.err); got != c.want {
			t.Errorf("isContextOverflow(%v) = %v, want %v", c.err, got, c.want)
		}
	}
}
