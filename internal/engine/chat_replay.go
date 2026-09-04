package engine

import (
	"context"
	"os"

	"github.com/GrayCodeAI/graycode-cli/internal/replaycache"
	"github.com/GrayCodeAI/graycode-cli/internal/types"
)

// replayCacheDirEnv opts a run into the disk-persisted replay cache: when set,
// every non-streaming completion is looked up by its canonicalized request and
// replayed from disk on a hit, giving deterministic, offline regression runs.
// Unset (the default) leaves the chat path untouched.
const replayCacheDirEnv = "GRAYCODE_REPLAY_CACHE_DIR"

// replayFingerprintEnv optionally folds an extra string (e.g. a fixture
// version) into replay cache keys so whole suites can be invalidated at once.
const replayFingerprintEnv = "GRAYCODE_REPLAY_FINGERPRINT"

// replayKey builds the cache key for one completion request.
func replayKey(opts types.ChatOptions, messages []types.GraycodeRouterMessage) string {
	return replaycache.Key(replaycache.Fingerprint(os.Getenv(replayFingerprintEnv)),
		opts.Provider, opts.Model, messages, opts.MaxTokens)
}

// chatWithReplay wraps client.Chat with the replay cache when
// GRAYCODE_REPLAY_CACHE_DIR is set; otherwise it calls straight through.
func chatWithReplay(ctx context.Context, client ChatClient, messages []types.GraycodeRouterMessage, opts types.ChatOptions) (*types.GraycodeRouterResponse, error) {
	dir := os.Getenv(replayCacheDirEnv)
	if dir == "" {
		return client.Chat(ctx, messages, opts)
	}
	cache := replaycache.New(dir)
	key := replayKey(opts, messages)
	if resp, ok := cache.Get(key); ok {
		return resp, nil
	}
	resp, err := client.Chat(ctx, messages, opts)
	if err != nil {
		return resp, err
	}
	_ = cache.Put(key, resp) // best-effort: a failed write must not fail the turn
	return resp, nil
}
