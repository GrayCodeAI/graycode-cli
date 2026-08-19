package eventlog

import "testing"

func TestTitleFromMessages_PrefersFirstUserTurn(t *testing.T) {
	got := TitleFromMessages([]Message{
		{Role: "assistant", Content: "Here is a fix"},
		{Role: "user", Content: "Fix the broken login flow"},
	})
	want := "Fix the broken login flow"
	if got != want {
		t.Fatalf("TitleFromMessages() = %q, want %q", got, want)
	}
}

func TestTitleFromMessages_IgnoresAssistantSystemStyleTurns(t *testing.T) {
	got := TitleFromMessages([]Message{
		{Role: "assistant", Content: "Context: current branch is main"},
	})
	if got != "Context: current branch is main" {
		t.Fatalf("TitleFromMessages() = %q", got)
	}
}

func TestTitleFromMessages_AssistFallback(t *testing.T) {
	got := TitleFromMessages([]Message{
		{Role: "assistant", Content: "No plan now"},
	})
	if got == "Untitled Session" {
		t.Fatal("assistant content should be usable if no user turn exists")
	}
}

func TestTitleFromMessages_TruncatesLongPrompt(t *testing.T) {
	long := make([]rune, 0, 250)
	for i := 0; i < 250; i++ {
		long = append(long, 'a')
	}
	got := TitleFromMessages([]Message{{Role: "user", Content: string(long)}})
	if len([]rune(got)) > 75 {
		t.Fatalf("title %q is longer than 75 runes", got)
	}
	if got == "" {
		t.Fatal("empty title for long prompt")
	}
}

func TestTitleFromMessages_EmptyReturnsUntitled(t *testing.T) {
	if got := TitleFromMessages(nil); got != "Untitled Session" {
		t.Fatalf("TitleFromMessages(nil) = %q", got)
	}
}
