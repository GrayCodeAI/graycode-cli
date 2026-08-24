package engine

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/GrayCodeAI/hawk/internal/types"
)

func TestChatWithReplayDisabledPassesThrough(t *testing.T) {
	t.Setenv(replayCacheDirEnv, "")
	calls := 0
	client := &countingClient{n: &calls, resp: "live"}
	resp, err := chatWithReplay(context.Background(), client,
		[]types.EyrieMessage{{Role: "user", Content: "hi"}}, types.ChatOptions{Model: "m"})
	if err != nil || resp.Content != "live" {
		t.Fatalf("resp=%v err=%v", resp, err)
	}
	if calls != 1 {
		t.Fatalf("calls = %d, want 1", calls)
	}
}

func TestChatWithReplayCachesAndHits(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(replayCacheDirEnv, dir)

	calls := 0
	client := &countingClient{n: &calls, resp: "live"}
	msgs := []types.EyrieMessage{{Role: "user", Content: "deterministic"}}
	opts := types.ChatOptions{Provider: "p", Model: "m", MaxTokens: 10}

	first, err := chatWithReplay(context.Background(), client, msgs, opts)
	if err != nil || first.Content != "live" {
		t.Fatalf("first call: resp=%v err=%v", first, err)
	}
	if calls != 1 {
		t.Fatalf("calls after first = %d", calls)
	}

	// Second identical request must replay from disk without calling the client.
	second, err := chatWithReplay(context.Background(), client, msgs, opts)
	if err != nil || second.Content != "live" {
		t.Fatalf("replayed call: resp=%v err=%v", second, err)
	}
	if calls != 1 {
		t.Fatalf("client called %d times, want 1 (second served from cache)", calls)
	}
}

func TestChatWithReplayFingerprintInvalidates(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(replayCacheDirEnv, dir)
	t.Setenv(replayFingerprintEnv, "v1")

	calls := 0
	client := &countingClient{n: &calls, resp: "live"}
	msgs := []types.EyrieMessage{{Role: "user", Content: "x"}}
	opts := types.ChatOptions{Provider: "p", Model: "m"}

	if _, err := chatWithReplay(context.Background(), client, msgs, opts); err != nil {
		t.Fatal(err)
	}
	t.Setenv(replayFingerprintEnv, "v2")
	if _, err := chatWithReplay(context.Background(), client, msgs, opts); err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatalf("calls = %d, want 2 after fingerprint bump", calls)
	}
}

func TestChatWithReplayDoesNotCacheErrors(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(replayCacheDirEnv, dir)

	calls := 0
	client := &countingClient{n: &calls, err: errors.New("boom")}
	for i := 0; i < 2; i++ {
		if _, err := chatWithReplay(context.Background(), client,
			[]types.EyrieMessage{{Role: "user", Content: "e"}}, types.ChatOptions{}); err == nil {
			t.Fatal("expected error to pass through")
		}
	}
	if calls != 2 {
		t.Fatalf("errors must not be cached; calls = %d", calls)
	}
	entries, _ := filepath.Glob(filepath.Join(dir, "resp", "*", "*.json"))
	if len(entries) != 0 {
		t.Fatalf("no entries expected after failed calls, got %v", entries)
	}
}

type countingClient struct {
	n    *int
	resp string
	err  error
}

func (c *countingClient) Chat(context.Context, []types.EyrieMessage, types.ChatOptions) (*types.EyrieResponse, error) {
	*c.n++
	if c.err != nil {
		return nil, c.err
	}
	return &types.EyrieResponse{Content: c.resp}, nil
}

func (c *countingClient) StreamChatContinue(context.Context, []types.EyrieMessage, types.ChatOptions, types.ContinuationConfig) (*types.StreamResult, error) {
	*c.n++
	if c.err != nil {
		return nil, c.err
	}
	ch := make(chan types.EyrieStreamEvent, 1)
	ch <- types.EyrieStreamEvent{Type: "done"}
	return &types.StreamResult{Events: ch}, nil
}
