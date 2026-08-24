// Package replaycache provides a deterministic, disk-persisted replay cache
// for LLM requests. Successful responses (complete and streamed) are stored
// under a SHA-256 key computed from a canonicalized request plus a config
// fingerprint; a later identical request replays the stored bytes instead of
// calling the provider. Built for reproducible agent runs and offline
// regression tests. Adopted from herm's request_cache.
//
// Secrets are never part of the key material directly: API keys are folded
// into the fingerprint via SHA-256 so the stored filenames cannot leak them,
// and responses are written 0600.
package replaycache

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/GrayCodeAI/hawk/internal/types"
)

// Cache is a directory-backed replay cache. Safe for concurrent use within one
// process; cross-process safety relies on atomic renames of whole files.
type Cache struct {
	dir string
}

// New returns a cache rooted at dir (created lazily on first write).
func New(dir string) *Cache {
	return &Cache{dir: dir}
}

// Dir returns the cache root.
func (c *Cache) Dir() string { return c.dir }

// Fingerprint folds configuration (including secrets) into a non-reversible
// digest so changing credentials invalidates entries without leaking them.
func Fingerprint(secrets ...string) string {
	h := sha256.Sum256([]byte(strings.Join(secrets, "\x00")))
	return hex.EncodeToString(h[:])[:16]
}

// Key computes the cache key for a chat request. Messages are canonicalized
// (role/content/tool fields, sorted map keys) so semantically identical
// requests produce identical keys regardless of struct field order.
func Key(fingerprint, provider, model string, messages []types.EyrieMessage, maxTokens int) string {
	canonical := canonicalMessages(messages)
	payload := fmt.Sprintf("%s|%s|%s|%s|%d", fingerprint, provider, model, canonical, maxTokens)
	sum := sha256.Sum256([]byte(payload))
	return hex.EncodeToString(sum[:])
}

func canonicalMessages(messages []types.EyrieMessage) string {
	var b strings.Builder
	for _, m := range messages {
		fmt.Fprintf(&b, "<%s>%s", m.Role, m.Content)
		if m.Thinking != "" {
			fmt.Fprintf(&b, "<think>%s", m.Thinking)
		}
		for _, tc := range m.ToolUse {
			args, _ := json.Marshal(tc.Arguments) // map keys marshal in sorted order
			fmt.Fprintf(&b, "<tooluse>%s%s", tc.Name, args)
		}
		for _, tr := range m.ToolResults {
			fmt.Fprintf(&b, "<toolresult>%s:%d", tr.Content, b2i(tr.IsError))
		}
		b.WriteString("</msg>")
	}
	return b.String()
}

func b2i(v bool) int {
	if v {
		return 1
	}
	return 0
}

// Get returns the cached complete response for key, or ok=false.
func (c *Cache) Get(key string) (*types.EyrieResponse, bool) {
	data, err := os.ReadFile(c.path("resp", key))
	if err != nil {
		return nil, false
	}
	var resp types.EyrieResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, false
	}
	return &resp, true
}

// Put stores a complete response under key.
func (c *Cache) Put(key string, resp *types.EyrieResponse) error {
	if resp == nil {
		return fmt.Errorf("replaycache: nil response")
	}
	data, err := json.Marshal(resp)
	if err != nil {
		return err
	}
	return c.writeAtomic(c.path("resp", key), data)
}

// GetStream returns the cached stream events for key, or ok=false.
func (c *Cache) GetStream(key string) ([]types.EyrieStreamEvent, bool) {
	data, err := os.ReadFile(c.path("stream", key))
	if err != nil {
		return nil, false
	}
	var events []types.EyrieStreamEvent
	if err := json.Unmarshal(data, &events); err != nil {
		return nil, false
	}
	return events, true
}

// PutStream stores stream events under key so a later identical request can
// replay the exact same sequence.
func (c *Cache) PutStream(key string, events []types.EyrieStreamEvent) error {
	if len(events) == 0 {
		return fmt.Errorf("replaycache: no events")
	}
	data, err := json.Marshal(events)
	if err != nil {
		return err
	}
	return c.writeAtomic(c.path("stream", key), data)
}

func (c *Cache) path(kind, key string) string {
	// Shard by the first two hex chars to keep directories small.
	return filepath.Join(c.dir, kind, key[:2], key+".json")
}

func (c *Cache) writeAtomic(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".replay-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer func() { _ = os.Remove(tmpPath) }()
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
}

// Entries returns the number of cached entries across both kinds.
func (c *Cache) Entries() int {
	total := 0
	for _, kind := range []string{"resp", "stream"} {
		root := filepath.Join(c.dir, kind)
		subdirs, err := os.ReadDir(root)
		if err != nil {
			continue
		}
		for _, sd := range subdirs {
			files, err := os.ReadDir(filepath.Join(root, sd.Name()))
			if err != nil {
				continue
			}
			total += len(files)
		}
	}
	return total
}
