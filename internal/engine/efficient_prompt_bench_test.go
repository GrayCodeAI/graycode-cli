package engine

import (
	"strings"
	"testing"
)

func BenchmarkCompressWhitespace(b *testing.B) {
	// Build a realistic prompt with multiple consecutive blank lines
	var sb strings.Builder
	for i := 0; i < 100; i++ {
		sb.WriteString("This is a line of text that simulates a prompt message.\n")
		if i%10 == 0 {
			sb.WriteString("\n\n\n\n\n") // 5 consecutive newlines
		}
	}
	input := sb.String()

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		compressWhitespace(input)
	}
}

func BenchmarkCompressWhitespaceLarge(b *testing.B) {
	// Large input with many triple-newline sequences
	var sb strings.Builder
	for i := 0; i < 1000; i++ {
		sb.WriteString("Line of content here with some text to make it realistic.\n")
		if i%5 == 0 {
			sb.WriteString("\n\n\n\n")
		}
	}
	input := sb.String()

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		compressWhitespace(input)
	}
}

func BenchmarkRemoveFiller(b *testing.B) {
	input := "I think that this is basically just a simple test, you know, for benchmarking purposes actually."

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		removeFiller(input)
	}
}

func BenchmarkAbbreviatePhrases(b *testing.B) {
	input := "For example, this is a test that you should be able to complete in a short amount of time."

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		abbreviatePhrases(input)
	}
}

func BenchmarkOptimizePrompt(b *testing.B) {
	ep := NewEfficientPrompter()
	input := "I think that basically, for example, this is just a simple test, you know, that should be optimized. It contains multiple sentences with filler words and    extra   whitespace.\n\n\n\nAnd also some repeated content content content content."

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		ep.Optimize(input)
	}
}
