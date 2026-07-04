package cmd

import "testing"

func TestShouldFlushStreamChunkBuffer(t *testing.T) {
	if shouldFlushStreamChunkBuffer("short") {
		t.Fatal("short chunk without boundary should not flush immediately")
	}
	if !shouldFlushStreamChunkBuffer("sentence.") {
		t.Fatal("sentence boundary should flush")
	}
	if !shouldFlushStreamChunkBuffer("line\n") {
		t.Fatal("newline boundary should flush")
	}
	long := make([]byte, 512)
	for i := range long {
		long[i] = 'x'
	}
	if !shouldFlushStreamChunkBuffer(string(long)) {
		t.Fatal("large buffer should flush")
	}
}
