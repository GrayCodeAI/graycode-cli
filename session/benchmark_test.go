package session

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// ──────────────────────────────────────────────────────────────────────────────
// Helpers
// ──────────────────────────────────────────────────────────────────────────────

// makeSession builds a session with n user/assistant message pairs.
func makeSession(id string, n int) *Session {
	msgs := make([]Message, 0, n)
	for i := 0; i < n; i++ {
		if i%2 == 0 {
			msgs = append(msgs, Message{
				Role:    "user",
				Content: fmt.Sprintf("Message %d: tell me about topic %d in detail", i, i),
			})
		} else {
			msgs = append(msgs, Message{
				Role:    "assistant",
				Content: fmt.Sprintf("Response %d: here is a detailed explanation of topic %d covering several aspects", i, i),
			})
		}
	}
	return &Session{
		ID:        id,
		Model:     "claude-sonnet-4-20250514",
		Provider:  "anthropic",
		CWD:       "/tmp/bench",
		Messages:  msgs,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
}

// setupBenchHome points HOME at a temp directory so session IO goes to tmpfs.
func setupBenchHome(b *testing.B) {
	b.Helper()
	tmp := b.TempDir()
	b.Setenv("HOME", tmp)
}

// ──────────────────────────────────────────────────────────────────────────────
// 1. BenchmarkSessionSave_100Messages
// ──────────────────────────────────────────────────────────────────────────────

func BenchmarkSessionSave_100Messages(b *testing.B) {
	setupBenchHome(b)
	sess := makeSession("bench-save-100", 100)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sess.ID = fmt.Sprintf("bench-save-100-%d", i)
		if err := Save(sess); err != nil {
			b.Fatal(err)
		}
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// 2. BenchmarkSessionSave_1000Messages
// ──────────────────────────────────────────────────────────────────────────────

func BenchmarkSessionSave_1000Messages(b *testing.B) {
	setupBenchHome(b)
	sess := makeSession("bench-save-1000", 1000)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sess.ID = fmt.Sprintf("bench-save-1000-%d", i)
		if err := Save(sess); err != nil {
			b.Fatal(err)
		}
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// 3. BenchmarkSessionLoad_100Messages
// ──────────────────────────────────────────────────────────────────────────────

func BenchmarkSessionLoad_100Messages(b *testing.B) {
	setupBenchHome(b)
	sess := makeSession("bench-load-100", 100)
	if err := Save(sess); err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := Load("bench-load-100"); err != nil {
			b.Fatal(err)
		}
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// 4. BenchmarkSessionLoad_1000Messages
// ──────────────────────────────────────────────────────────────────────────────

func BenchmarkSessionLoad_1000Messages(b *testing.B) {
	setupBenchHome(b)
	sess := makeSession("bench-load-1000", 1000)
	if err := Save(sess); err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := Load("bench-load-1000"); err != nil {
			b.Fatal(err)
		}
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// 5. BenchmarkWALAppend
// ──────────────────────────────────────────────────────────────────────────────

func BenchmarkWALAppend(b *testing.B) {
	setupBenchHome(b)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		walID := fmt.Sprintf("bench-wal-%d", i)
		wal, err := NewWAL(walID)
		if err != nil {
			b.Fatal(err)
		}
		b.StartTimer()

		for j := 0; j < 1000; j++ {
			msg := Message{
				Role:    "user",
				Content: fmt.Sprintf("WAL message %d for benchmark iteration %d", j, i),
			}
			if err := wal.Append(msg); err != nil {
				b.Fatal(err)
			}
		}

		b.StopTimer()
		wal.Close()
		b.StartTimer()
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// 6. BenchmarkSessionList_50Sessions
// ──────────────────────────────────────────────────────────────────────────────

func BenchmarkSessionList_50Sessions(b *testing.B) {
	setupBenchHome(b)

	// Pre-create 50 session files with a short gap so mod times differ.
	for i := 0; i < 50; i++ {
		sess := &Session{
			ID:       fmt.Sprintf("bench-list-%02d", i),
			Model:    "claude-sonnet-4-20250514",
			Provider: "anthropic",
			CWD:      "/tmp/bench",
			Messages: []Message{
				{Role: "user", Content: fmt.Sprintf("Session %d prompt", i)},
				{Role: "assistant", Content: fmt.Sprintf("Session %d response", i)},
			},
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}
		if err := Save(sess); err != nil {
			b.Fatal(err)
		}
	}

	// Verify files exist.
	home := os.Getenv("HOME")
	dir := filepath.Join(home, ".hawk", "sessions")
	entries, _ := os.ReadDir(dir)
	if len(entries) < 50 {
		b.Fatalf("expected at least 50 session files, got %d", len(entries))
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		list, err := List()
		if err != nil {
			b.Fatal(err)
		}
		if len(list) < 50 {
			b.Fatalf("expected at least 50 entries, got %d", len(list))
		}
	}
}
