package replaycache

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/GrayCodeAI/graycode-cli/internal/types"
)

func TestKeyDeterministic(t *testing.T) {
	msgs := []types.GraycodeRouterMessage{{Role: "user", Content: "hello"}}
	k1 := Key("fp", "anthropic", "claude", msgs, 100)
	k2 := Key("fp", "anthropic", "claude", msgs, 100)
	if k1 != k2 {
		t.Fatal("identical requests must produce identical keys")
	}
	if Key("fp2", "anthropic", "claude", msgs, 100) == k1 {
		t.Fatal("different fingerprint should change the key")
	}
	if Key("fp", "openai", "claude", msgs, 100) == k1 {
		t.Fatal("different provider should change the key")
	}
}

func TestFingerprintHidesSecrets(t *testing.T) {
	fp := Fingerprint("sk-secret-key")
	if strings.Contains(fp, "sk-secret") {
		t.Fatal("fingerprint leaked the secret")
	}
	if len(fp) != 16 {
		t.Fatalf("fingerprint length = %d, want 16", len(fp))
	}
	if Fingerprint("a") == Fingerprint("b") {
		t.Fatal("different secrets should produce different fingerprints")
	}
}

func TestPutGetResponseRoundTrip(t *testing.T) {
	c := New(t.TempDir())
	key := Key("fp", "p", "m", []types.GraycodeRouterMessage{{Role: "user", Content: "hi"}}, 10)
	want := &types.GraycodeRouterResponse{Content: "cached answer", FinishReason: "end_turn"}
	if err := c.Put(key, want); err != nil {
		t.Fatal(err)
	}
	got, ok := c.Get(key)
	if !ok {
		t.Fatal("cache miss after Put")
	}
	if got.Content != "cached answer" || got.FinishReason != "end_turn" {
		t.Fatalf("got %+v", got)
	}
	if _, ok := c.Get("missing"); ok {
		t.Fatal("expected miss for unknown key")
	}
}

func TestStreamRoundTrip(t *testing.T) {
	c := New(t.TempDir())
	key := Key("fp", "p", "m", []types.GraycodeRouterMessage{{Role: "user", Content: "s"}}, 10)
	events := []types.GraycodeRouterStreamEvent{
		{Type: "content", Content: "hel"},
		{Type: "content", Content: "lo"},
		{Type: "done", StopReason: "end_turn"},
	}
	if err := c.PutStream(key, events); err != nil {
		t.Fatal(err)
	}
	got, ok := c.GetStream(key)
	if !ok {
		t.Fatal("stream cache miss after PutStream")
	}
	if len(got) != 3 || got[0].Content != "hel" || got[2].StopReason != "end_turn" {
		t.Fatalf("replayed events = %+v", got)
	}
}

func TestFilesAreNotWorldReadable(t *testing.T) {
	dir := t.TempDir()
	c := New(dir)
	key := Key("fp", "p", "m", nil, 0)
	if err := c.Put(key, &types.GraycodeRouterResponse{Content: "x"}); err != nil {
		t.Fatal(err)
	}
	matches, _ := filepath.Glob(filepath.Join(dir, "resp", "*", "*.json"))
	if len(matches) != 1 {
		t.Fatalf("expected one cached file, got %v", matches)
	}
	info, err := os.Stat(matches[0])
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("file mode = %o, want 600", perm)
	}
}

func TestPutNilResponseErrors(t *testing.T) {
	if err := New(t.TempDir()).Put(Key("f", "p", "m", nil, 0), nil); err == nil {
		t.Fatal("expected error for nil response")
	}
}

func TestPutStreamEmptyErrors(t *testing.T) {
	if err := New(t.TempDir()).PutStream(Key("f", "p", "m", nil, 0), nil); err == nil {
		t.Fatal("expected error for empty events")
	}
}

func TestEntriesCountsBothKinds(t *testing.T) {
	c := New(t.TempDir())
	k := Key("fp", "p", "m", []types.GraycodeRouterMessage{{Role: "user", Content: "e"}}, 5)
	if err := c.Put(k, &types.GraycodeRouterResponse{Content: "r"}); err != nil {
		t.Fatal(err)
	}
	if err := c.PutStream(k, []types.GraycodeRouterStreamEvent{{Type: "done"}}); err != nil {
		t.Fatal(err)
	}
	if got := c.Entries(); got != 2 {
		t.Fatalf("entries = %d, want 2", got)
	}
}

func TestJSONStableAcrossMarshalOrder(t *testing.T) {
	// The canonical key must not depend on Go struct marshaling order.
	msgs := []types.GraycodeRouterMessage{
		{Role: "user", Content: "q"},
		{Role: "assistant", Content: "", ToolUse: []types.ToolCall{{Name: "Bash", Arguments: map[string]interface{}{"command": "ls"}}}, ToolResults: []types.ToolResult{{Content: "out", IsError: true}}},
	}
	a := Key("fp", "p", "m", msgs, 7)
	b := Key("fp", "p", "m", msgs, 7)
	if a != b {
		t.Fatal("key unstable for identical requests")
	}
	// A changed tool result must change the key.
	msgs[1].ToolResults[0].IsError = false
	if Key("fp", "p", "m", msgs, 7) == a {
		t.Fatal("changed tool-result error flag should change the key")
	}
}
