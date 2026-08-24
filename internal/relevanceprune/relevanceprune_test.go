package relevanceprune

import (
	"testing"
	"time"
)

func msg(role, content string, hasTool, isErr bool, t time.Time) Message {
	return Message{Role: role, Content: content, HasToolCall: hasTool, IsError: isErr, Timestamp: t}
}

func TestNormalizeCompactTailTurns(t *testing.T) {
	if NormalizeCompactTailTurns(5) != 5 {
		t.Fatal("5 should stay 5")
	}
	if NormalizeCompactTailTurns(0) != DefaultCompactTailTurns {
		t.Fatal("0 should fall back to default")
	}
	if NormalizeCompactTailTurns(-2) != DefaultCompactTailTurns {
		t.Fatal("negative should fall back to default")
	}
}

func TestCalculateRelevanceBasics(t *testing.T) {
	now := time.Now()
	base := msg("user", "hello there", false, false, now)
	if got := CalculateRelevance(base, Options{}); got < 0.5 || got > 1 {
		t.Fatalf("base score = %v", got)
	}
	// Error preserved should score higher.
	errMsg := msg("assistant", "boom", false, true, now)
	withErr := CalculateRelevance(errMsg, Options{PreserveErrors: true})
	withoutErr := CalculateRelevance(errMsg, Options{})
	if withErr <= withoutErr {
		t.Fatalf("preserved error should boost: %v vs %v", withErr, withoutErr)
	}
	// Task-context overlap should boost.
	rel := msg("user", "refactor the payment module and update tests", false, false, now)
	if CalculateRelevance(rel, Options{TaskContext: "refactor payment module tests"}) <= CalculateRelevance(rel, Options{}) {
		t.Fatal("task overlap should boost relevance")
	}
}

func TestPruneByRelevancePreservesRecent(t *testing.T) {
	base := time.Now()
	var msgs []Message
	for i := 0; i < 8; i++ {
		msgs = append(msgs, msg("user", "unrelated chatter filler content here", false, false, base.Add(time.Duration(i)*time.Minute)))
	}
	// The last message strongly matches the task.
	msgs[len(msgs)-1] = msg("user", "refactor payment module", false, false, base.Add(7*time.Minute))

	pruned := PruneByRelevance(msgs, Options{TargetTokens: 100, TaskContext: "refactor payment module", PreserveRecent: 3, PreserveTools: true, PreserveErrors: true})
	if len(pruned) < 3 {
		t.Fatalf("pruned below preserve-recent floor: %d", len(pruned))
	}
	// The most recent message must survive verbatim.
	last := pruned[len(pruned)-1]
	if last.Content != "refactor payment module" {
		t.Fatalf("most recent message not preserved: %q", last.Content)
	}
}

func TestPruneShortListUnchanged(t *testing.T) {
	msgs := []Message{
		msg("user", "a", false, false, time.Now()),
		msg("assistant", "b", false, false, time.Now()),
	}
	out := PruneByRelevance(msgs, Options{TargetTokens: 5000, PreserveRecent: 3})
	if len(out) != 2 {
		t.Fatalf("short list should be unchanged, got %d", len(out))
	}
}

func TestGetRelevanceStats(t *testing.T) {
	now := time.Now()
	msgs := []Message{
		msg("user", "refactor payment module deeply", false, false, now),
		msg("assistant", "done", true, false, now),
		msg("assistant", "failed", false, true, now),
	}
	avg, high, tools, errs := GetRelevanceStats(msgs, Options{TaskContext: "refactor payment module", PreserveTools: true, PreserveErrors: true})
	if tools != 1 || errs != 1 {
		t.Fatalf("tools=%d errors=%d", tools, errs)
	}
	if high == 0 {
		t.Fatal("expected at least one high-relevance message")
	}
	if avg <= 0 || avg > 1 {
		t.Fatalf("avg = %v", avg)
	}
}
