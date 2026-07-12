package engine

import (
	"testing"

	"github.com/GrayCodeAI/hawk/internal/types"
)

func TestProviderNativeCompactionTrigger(t *testing.T) {
	strategy := &ProviderNativeCompactStrategy{}
	messages := make([]types.EyrieMessage, 8)
	if !strategy.ShouldTrigger(messages, 80_000, 80_000) {
		t.Fatal("expected provider-native compaction at threshold")
	}
	if strategy.ShouldTrigger(messages[:7], 80_000, 80_000) {
		t.Fatal("expected short conversations to skip provider-native compaction")
	}
}
