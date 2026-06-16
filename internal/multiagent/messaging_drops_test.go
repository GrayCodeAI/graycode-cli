package mission

import (
	"bytes"
	"log/slog"
	"strings"
	"sync"
	"testing"
)

// drainChannel reads and discards all currently buffered messages from ch
// without blocking. It does not close the channel.
func drainChannel(ch <-chan AgentMessage) {
	for {
		select {
		case <-ch:
		default:
			return
		}
	}
}

// TestMessageBus_Stats_Initial verifies the zero state.
func TestMessageBus_Stats_Initial(t *testing.T) {
	mb := NewMessageBus()
	got := mb.Stats()
	if got.Dropped != 0 {
		t.Errorf("Dropped = %d, want 0", got.Dropped)
	}
	if got.Agents != 0 {
		t.Errorf("Agents = %d, want 0", got.Agents)
	}
	if got.Locks != 0 {
		t.Errorf("Locks = %d, want 0", got.Locks)
	}
	if got.HistorySz != 0 {
		t.Errorf("HistorySz = %d, want 0", got.HistorySz)
	}
}

// TestMessageBus_Stats_TracksAgents verifies the Agents counter reflects
// Register/Unregister.
func TestMessageBus_Stats_TracksAgents(t *testing.T) {
	mb := NewMessageBus()
	mb.Register("a1")
	mb.Register("a2")
	if got := mb.Stats().Agents; got != 2 {
		t.Errorf("Agents = %d, want 2", got)
	}
	mb.Unregister("a1")
	if got := mb.Stats().Agents; got != 1 {
		t.Errorf("Agents after Unregister = %d, want 1", got)
	}
}

// TestMessageBus_NoDropOnNormalSend verifies a normal send does not
// increment the dropped counter.
func TestMessageBus_NoDropOnNormalSend(t *testing.T) {
	mb := NewMessageBus()
	mb.Register("a1")
	if err := mb.Send(AgentMessage{From: "a2", To: "a1", Topic: "discovery"}); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if got := mb.Stats().Dropped; got != 0 {
		t.Errorf("Dropped = %d, want 0", got)
	}
}

// TestMessageBus_SpecificSendDrop_IncrementsCounter verifies that a
// direct send to a full channel increments the dropped counter and
// returns an error.
func TestMessageBus_SpecificSendDrop_IncrementsCounter(t *testing.T) {
	mb := NewMessageBus()
	ch := mb.Register("a1")
	_ = ch // referenced for documentation; we send via Send, not directly

	// Fill a1's channel (cap = 64 from Register).
	for i := 0; i < 64; i++ {
		if err := mb.Send(AgentMessage{From: "x", To: "a1", Topic: "discovery"}); err != nil {
			t.Fatalf("Send %d: unexpected error: %v", i, err)
		}
	}

	// Next direct send should fail with "channel full" and bump Dropped.
	err := mb.Send(AgentMessage{From: "x", To: "a1", Topic: "discovery"})
	if err == nil {
		t.Fatal("expected error on full channel, got nil")
	}
	if !strings.Contains(err.Error(), "channel full") {
		t.Errorf("err = %q, want 'channel full'", err.Error())
	}
	if got := mb.Stats().Dropped; got != 1 {
		t.Errorf("Dropped = %d, want 1", got)
	}
}

// TestMessageBus_BroadcastDrop_IncrementsCounter verifies a broadcast
// to a full channel increments the counter (the prior behavior was a
// silent drop with no log or counter).
func TestMessageBus_BroadcastDrop_IncrementsCounter(t *testing.T) {
	mb := NewMessageBus()
	mb.Register("a1")
	mb.Register("a2")
	mb.Register("sender") // not part of broadcast, but reserves a channel

	// Fill a1 and a2's channels.
	for i := 0; i < 64; i++ {
		_ = mb.Send(AgentMessage{From: "sender", To: "a1", Topic: "t"})
		_ = mb.Send(AgentMessage{From: "sender", To: "a2", Topic: "t"})
	}

	before := mb.Stats().Dropped

	// Broadcast from sender to all. a1 and a2 are full → both drops.
	if err := mb.Send(AgentMessage{From: "sender", Topic: "t"}); err != nil {
		t.Fatalf("Send (broadcast): %v", err)
	}

	if got := mb.Stats().Dropped - before; got != 2 {
		t.Errorf("Dropped delta = %d, want 2 (one per full agent)", got)
	}
}

// TestMessageBus_BroadcastDrop_LogsWarn verifies the WARN log path.
func TestMessageBus_BroadcastDrop_LogsWarn(t *testing.T) {
	var buf bytes.Buffer
	orig := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn})))
	t.Cleanup(func() { slog.SetDefault(orig) })

	mb := NewMessageBus()
	mb.Register("a1")
	for i := 0; i < 64; i++ {
		_ = mb.Send(AgentMessage{From: "sender", To: "a1", Topic: "discovery"})
	}

	buf.Reset() // clear any pre-existing output
	_ = mb.Send(AgentMessage{From: "sender", Topic: "discovery"})

	out := buf.String()
	if !strings.Contains(out, "WARN") {
		t.Errorf("expected WARN level, got: %q", out)
	}
	if !strings.Contains(out, "message bus: dropped message") {
		t.Errorf("expected 'message bus: dropped message' in log, got: %q", out)
	}
	if !strings.Contains(out, "a1") {
		t.Errorf("expected log to include dropped-to agent, got: %q", out)
	}
	if !strings.Contains(out, "sender") {
		t.Errorf("expected log to include from agent, got: %q", out)
	}
	if !strings.Contains(out, "discovery") {
		t.Errorf("expected log to include topic, got: %q", out)
	}
}

// TestMessageBus_Sampling_OnlyLogsFirstAndHundredth verifies the
// sampling: first drop logged, drops 2..99 not logged, drop 100 logged.
func TestMessageBus_Sampling_OnlyLogsFirstAndHundredth(t *testing.T) {
	var buf bytes.Buffer
	orig := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn})))
	t.Cleanup(func() { slog.SetDefault(orig) })

	mb := NewMessageBus()
	mb.Register("a1")
	for i := 0; i < 64; i++ {
		_ = mb.Send(AgentMessage{From: "sender", To: "a1", Topic: "t"})
	}

	// Drop 1: logged.
	_ = mb.Send(AgentMessage{From: "sender", Topic: "t"})
	firstLogCount := bytes.Count(buf.Bytes(), []byte("dropped message"))

	// Drops 2..99: not logged individually (cumulative counter goes up).
	for i := 0; i < 98; i++ {
		_ = mb.Send(AgentMessage{From: "sender", Topic: "t"})
	}
	midLogCount := bytes.Count(buf.Bytes(), []byte("dropped message"))

	// Drop 100: logged (n % 100 == 0).
	_ = mb.Send(AgentMessage{From: "sender", Topic: "t"})
	hundredthLogCount := bytes.Count(buf.Bytes(), []byte("dropped message"))

	if firstLogCount != 1 {
		t.Errorf("expected 1 log line after first drop, got %d", firstLogCount)
	}
	if midLogCount != firstLogCount {
		t.Errorf("expected no new log lines between drop 1 and 100, got delta %d", midLogCount-firstLogCount)
	}
	if hundredthLogCount != firstLogCount+1 {
		t.Errorf("expected exactly one new log line at drop 100, got delta %d", hundredthLogCount-firstLogCount)
	}

	// Counter should reflect all 100 drops.
	if got := mb.Stats().Dropped; got != 100 {
		t.Errorf("Dropped = %d, want 100", got)
	}
}

// TestMessageBus_Stats_SafeConcurrent verifies Stats() is safe to call
// concurrently with Send/Register/Unregister.
func TestMessageBus_Stats_SafeConcurrent(t *testing.T) {
	mb := NewMessageBus()
	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				_ = mb.Stats()
			}
		}(i)
	}
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			id := string(rune('a' + i))
			mb.Register(id)
			_ = mb.Send(AgentMessage{From: id, Topic: "t"})
			mb.Unregister(id)
		}(i)
	}
	wg.Wait()
	// No assertion on specific values — just that the race detector is happy.
}

// TestMessageBus_DroppedCount_NotAffectedByNormalSend confirms that
// history and Stats.HistorySz don't accidentally bump Dropped.
func TestMessageBus_DroppedCount_NotAffectedByNormalSend(t *testing.T) {
	mb := NewMessageBus()
	mb.Register("a1")
	for i := 0; i < 10; i++ {
		_ = mb.Send(AgentMessage{From: "a2", To: "a1", Topic: "ok"})
	}
	if got := mb.Stats().Dropped; got != 0 {
		t.Errorf("Dropped = %d, want 0", got)
	}
	if got := mb.Stats().HistorySz; got != 10 {
		t.Errorf("HistorySz = %d, want 10", got)
	}
	// Drain so we don't leak a goroutine in this test.
	drainChannel(mb.Register("drain"))
}
