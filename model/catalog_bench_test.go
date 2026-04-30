package model

import "testing"

func BenchmarkFind(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Find("gpt-4o")
	}
}

func BenchmarkByProvider(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ByProvider("anthropic")
	}
}

func BenchmarkDefaultModel(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		DefaultModel("anthropic")
	}
}
