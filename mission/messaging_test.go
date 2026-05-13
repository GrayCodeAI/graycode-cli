package mission

import (
	"strings"
	"sync"
	"testing"
	"time"
)

func TestRegisterUnregister(t *testing.T) {
	mb := NewMessageBus()

	ch := mb.Register("agent-1")
	if ch == nil {
		t.Fatal("expected non-nil channel from Register")
	}

	// Verify the agent is registered
	mb.mu.RLock()
	_, ok := mb.channels["agent-1"]
	mb.mu.RUnlock()
	if !ok {
		t.Error("agent-1 should be in channels map after Register")
	}

	mb.Unregister("agent-1")

	// Channel should be closed
	_, open := <-ch
	if open {
		t.Error("expected channel to be closed after Unregister")
	}

	// Verify the agent is removed
	mb.mu.RLock()
	_, ok = mb.channels["agent-1"]
	mb.mu.RUnlock()
	if ok {
		t.Error("agent-1 should not be in channels map after Unregister")
	}
}

func TestUnregisterRemovesSubscriptions(t *testing.T) {
	mb := NewMessageBus()
	mb.Register("agent-1")
	mb.Subscribe("agent-1", "discovery")
	mb.Subscribe("agent-1", "conflict")

	mb.Unregister("agent-1")

	mb.mu.RLock()
	for topic, agents := range mb.subscribers {
		for _, a := range agents {
			if a == "agent-1" {
				t.Errorf("agent-1 still subscribed to topic %q after Unregister", topic)
			}
		}
	}
	mb.mu.RUnlock()
}

func TestSendToSpecificAgent(t *testing.T) {
	mb := NewMessageBus()
	ch1 := mb.Register("agent-1")
	ch2 := mb.Register("agent-2")

	msg := AgentMessage{
		From:    "agent-1",
		To:      "agent-2",
		Topic:   "discovery",
		Content: "found something",
	}
	if err := mb.Send(msg); err != nil {
		t.Fatalf("Send failed: %v", err)
	}

	// agent-2 should receive the message
	select {
	case received := <-ch2:
		if received.Content != "found something" {
			t.Errorf("unexpected content: %s", received.Content)
		}
		if received.ID == "" {
			t.Error("expected message ID to be auto-generated")
		}
		if received.Timestamp.IsZero() {
			t.Error("expected timestamp to be set")
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("agent-2 did not receive message")
	}

	// agent-1 should NOT receive the message
	select {
	case <-ch1:
		t.Fatal("agent-1 should not receive a message targeted to agent-2")
	case <-time.After(50 * time.Millisecond):
		// Expected
	}
}

func TestSendToUnregisteredAgent(t *testing.T) {
	mb := NewMessageBus()
	msg := AgentMessage{
		From:    "agent-1",
		To:      "agent-99",
		Topic:   "discovery",
		Content: "hello",
	}
	err := mb.Send(msg)
	if err == nil {
		t.Fatal("expected error sending to unregistered agent")
	}
	if !strings.Contains(err.Error(), "not registered") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestBroadcastToAll(t *testing.T) {
	mb := NewMessageBus()
	ch1 := mb.Register("agent-1")
	ch2 := mb.Register("agent-2")
	ch3 := mb.Register("agent-3")

	mb.Broadcast("agent-1", "discovery", "auth uses JWT")

	// agent-2 and agent-3 should receive, agent-1 (sender) should not
	for _, tc := range []struct {
		name string
		ch   <-chan AgentMessage
		want bool
	}{
		{"agent-1 (sender)", ch1, false},
		{"agent-2", ch2, true},
		{"agent-3", ch3, true},
	} {
		select {
		case msg := <-tc.ch:
			if !tc.want {
				t.Errorf("%s should NOT have received message, got: %v", tc.name, msg)
			} else if msg.Content != "auth uses JWT" {
				t.Errorf("%s got wrong content: %s", tc.name, msg.Content)
			}
		case <-time.After(50 * time.Millisecond):
			if tc.want {
				t.Errorf("%s should have received message but didn't", tc.name)
			}
		}
	}
}

func TestTopicSubscriptionFiltering(t *testing.T) {
	mb := NewMessageBus()
	ch1 := mb.Register("agent-1")
	ch2 := mb.Register("agent-2")
	ch3 := mb.Register("agent-3")

	// Only agent-2 subscribes to "discovery"
	mb.Subscribe("agent-2", "discovery")

	// Broadcast a discovery message. Since "discovery" topic has subscribers,
	// only subscribed agents should receive it.
	mb.Broadcast("agent-1", "discovery", "found a pattern")

	// agent-2 (subscriber) should receive
	select {
	case msg := <-ch2:
		if msg.Content != "found a pattern" {
			t.Errorf("unexpected content: %s", msg.Content)
		}
	case <-time.After(50 * time.Millisecond):
		t.Error("agent-2 (subscriber) should have received the message")
	}

	// agent-3 (not subscribed) should NOT receive
	select {
	case <-ch3:
		t.Error("agent-3 (not subscribed) should NOT receive the message")
	case <-time.After(50 * time.Millisecond):
		// Expected
	}

	// agent-1 (sender) should NOT receive
	select {
	case <-ch1:
		t.Error("sender should not receive own broadcast")
	case <-time.After(50 * time.Millisecond):
		// Expected
	}
}

func TestHistoryRetrieval(t *testing.T) {
	mb := NewMessageBus()
	mb.Register("agent-1")
	mb.Register("agent-2")

	mb.Broadcast("agent-1", "discovery", "msg-1")
	mb.Broadcast("agent-2", "conflict", "msg-2")
	mb.Broadcast("agent-1", "discovery", "msg-3")
	mb.Broadcast("agent-2", "progress", "msg-4")

	// All history
	all := mb.GetHistory("", 0)
	if len(all) != 4 {
		t.Fatalf("expected 4 messages in history, got %d", len(all))
	}

	// Filter by topic
	discoveries := mb.GetHistory("discovery", 0)
	if len(discoveries) != 2 {
		t.Errorf("expected 2 discovery messages, got %d", len(discoveries))
	}

	// Limit
	limited := mb.GetHistory("", 2)
	if len(limited) != 2 {
		t.Errorf("expected 2 messages with limit, got %d", len(limited))
	}
	// Should be the most recent 2, in chronological order
	if limited[0].Content != "msg-3" {
		t.Errorf("expected msg-3, got %s", limited[0].Content)
	}
	if limited[1].Content != "msg-4" {
		t.Errorf("expected msg-4, got %s", limited[1].Content)
	}
}

func TestWaitForResponse_Success(t *testing.T) {
	mb := NewMessageBus()
	mb.Register("agent-1")
	mb.Register("agent-2")

	requestID := mb.RequestHelp("agent-1", "need help with auth")

	// Simulate agent-2 responding after a short delay
	go func() {
		time.Sleep(30 * time.Millisecond)
		response := AgentMessage{
			From:       "agent-2",
			To:         "agent-1",
			Topic:      "request",
			Content:    "use the OAuth2 module",
			ResponseTo: requestID,
		}
		_ = mb.Send(response)
	}()

	resp, err := mb.WaitForResponse(requestID, 500*time.Millisecond)
	if err != nil {
		t.Fatalf("WaitForResponse failed: %v", err)
	}
	if resp.Content != "use the OAuth2 module" {
		t.Errorf("unexpected response content: %s", resp.Content)
	}
	if resp.From != "agent-2" {
		t.Errorf("unexpected from: %s", resp.From)
	}
}

func TestWaitForResponse_Timeout(t *testing.T) {
	mb := NewMessageBus()
	mb.Register("agent-1")

	requestID := mb.RequestHelp("agent-1", "need help")

	_, err := mb.WaitForResponse(requestID, 50*time.Millisecond)
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if !strings.Contains(err.Error(), "timeout") {
		t.Errorf("expected timeout error, got: %v", err)
	}
}

func TestReportConflict(t *testing.T) {
	mb := NewMessageBus()
	mb.Register("agent-1")
	ch2 := mb.Register("agent-2")

	mb.ReportConflict("agent-1", []string{"middleware.go", "auth.go"}, "both modifying auth layer")

	select {
	case msg := <-ch2:
		if msg.Topic != "conflict" {
			t.Errorf("expected conflict topic, got %s", msg.Topic)
		}
		if msg.Priority != 1 {
			t.Errorf("conflict should be priority 1 (urgent), got %d", msg.Priority)
		}
		if !strings.Contains(msg.Content, "middleware.go") {
			t.Errorf("expected file names in content: %s", msg.Content)
		}
		if !strings.Contains(msg.Content, "both modifying auth layer") {
			t.Errorf("expected description in content: %s", msg.Content)
		}
	case <-time.After(50 * time.Millisecond):
		t.Fatal("agent-2 did not receive conflict report")
	}
}

func TestReportDiscovery(t *testing.T) {
	mb := NewMessageBus()
	mb.Register("agent-1")
	ch2 := mb.Register("agent-2")

	mb.ReportDiscovery("agent-1", "auth uses RS256 JWT tokens")

	select {
	case msg := <-ch2:
		if msg.Topic != "discovery" {
			t.Errorf("expected discovery topic, got %s", msg.Topic)
		}
		if msg.Content != "auth uses RS256 JWT tokens" {
			t.Errorf("unexpected content: %s", msg.Content)
		}
		if msg.From != "agent-1" {
			t.Errorf("unexpected from: %s", msg.From)
		}
	case <-time.After(50 * time.Millisecond):
		t.Fatal("agent-2 did not receive discovery report")
	}
}

func TestReportProgress(t *testing.T) {
	mb := NewMessageBus()
	mb.Register("agent-1")
	ch2 := mb.Register("agent-2")

	mb.ReportProgress("agent-1", 80, "rate limiting module complete")

	select {
	case msg := <-ch2:
		if msg.Topic != "progress" {
			t.Errorf("expected progress topic, got %s", msg.Topic)
		}
		if msg.Priority != 5 {
			t.Errorf("progress should be priority 5 (low), got %d", msg.Priority)
		}
		if !strings.Contains(msg.Content, "80%") {
			t.Errorf("expected percentage in content: %s", msg.Content)
		}
		if !strings.Contains(msg.Content, "rate limiting module complete") {
			t.Errorf("expected status in content: %s", msg.Content)
		}
	case <-time.After(50 * time.Millisecond):
		t.Fatal("agent-2 did not receive progress report")
	}
}

func TestBuildContextFromMessages(t *testing.T) {
	mb := NewMessageBus()
	mb.Register("agent-1")
	mb.Register("agent-2")
	mb.Register("agent-3")

	mb.ReportDiscovery("agent-2", "auth uses RS256 JWT tokens")
	mb.ReportConflict("agent-3", []string{"middleware.go"}, "both modifying middleware.go")
	mb.ReportProgress("agent-1", 80, "rate limiting complete")

	ctx := mb.BuildContextFromMessages("agent-1", 0)

	if !strings.Contains(ctx, "## Team Communication") {
		t.Error("expected header in context")
	}
	if !strings.Contains(ctx, "[agent-2] discovered:") {
		t.Error("expected discovery from agent-2")
	}
	if !strings.Contains(ctx, "RS256 JWT tokens") {
		t.Error("expected discovery content")
	}
	if !strings.Contains(ctx, "[agent-3] conflict:") {
		t.Error("expected conflict from agent-3")
	}
	// agent-1's own messages should not appear
	if strings.Contains(ctx, "[agent-1]") {
		t.Error("agent should not see its own messages in context")
	}
}

func TestBuildContextFromMessages_MaxTokens(t *testing.T) {
	mb := NewMessageBus()
	mb.Register("agent-1")
	mb.Register("agent-2")

	// Add many messages
	for i := 0; i < 50; i++ {
		mb.ReportDiscovery("agent-2", strings.Repeat("x", 100))
	}

	ctx := mb.BuildContextFromMessages("agent-1", 200)
	if len(ctx) > 200 {
		t.Errorf("context should be truncated to max tokens, got length %d", len(ctx))
	}
	// Should end at a complete line
	if ctx[len(ctx)-1] == '\n' {
		t.Error("should not end with trailing newline after truncation")
	}
}

func TestBuildContextFromMessages_TargetedMessages(t *testing.T) {
	mb := NewMessageBus()
	mb.Register("agent-1")
	mb.Register("agent-2")
	mb.Register("agent-3")

	// Send a message targeted only to agent-2
	msg := AgentMessage{
		From:    "agent-3",
		To:      "agent-2",
		Topic:   "discovery",
		Content: "secret for agent-2 only",
	}
	_ = mb.Send(msg)

	// agent-1 should NOT see this in their context
	ctx1 := mb.BuildContextFromMessages("agent-1", 0)
	if strings.Contains(ctx1, "secret for agent-2 only") {
		t.Error("agent-1 should not see messages targeted to agent-2")
	}

	// agent-2 SHOULD see this
	ctx2 := mb.BuildContextFromMessages("agent-2", 0)
	if !strings.Contains(ctx2, "secret for agent-2 only") {
		t.Error("agent-2 should see messages targeted to them")
	}
}

func TestConcurrentSendReceive(t *testing.T) {
	mb := NewMessageBus()
	const numAgents = 5
	const msgsPerAgent = 20

	channels := make([]<-chan AgentMessage, numAgents)
	for i := 0; i < numAgents; i++ {
		channels[i] = mb.Register(agentName(i))
	}

	var wg sync.WaitGroup

	// Each agent sends messages concurrently
	for i := 0; i < numAgents; i++ {
		wg.Add(1)
		go func(agentIdx int) {
			defer wg.Done()
			for j := 0; j < msgsPerAgent; j++ {
				mb.Broadcast(agentName(agentIdx), "progress",
					agentName(agentIdx)+" msg")
			}
		}(i)
	}

	wg.Wait()

	// Verify history captured all messages
	all := mb.GetHistory("", 0)
	expected := numAgents * msgsPerAgent
	if len(all) != expected {
		t.Errorf("expected %d messages in history, got %d", expected, len(all))
	}
}

func TestConcurrentRegisterSend(t *testing.T) {
	mb := NewMessageBus()
	mb.Register("sender")

	var wg sync.WaitGroup

	// Register agents concurrently while sending
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			mb.Register(agentName(idx + 100))
		}(i)
	}

	// Send messages concurrently
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			mb.Broadcast("sender", "discovery", "concurrent finding")
		}()
	}

	wg.Wait()
	// If no panic/deadlock, test passes
}

func TestMessageOrdering(t *testing.T) {
	mb := NewMessageBus()
	mb.Register("agent-1")
	mb.Register("agent-2")

	// Send messages in sequence
	for i := 0; i < 10; i++ {
		mb.Broadcast("agent-1", "progress", orderContent(i))
	}

	history := mb.GetHistory("progress", 0)
	if len(history) != 10 {
		t.Fatalf("expected 10 messages, got %d", len(history))
	}

	// Verify ordering is preserved
	for i, msg := range history {
		expected := orderContent(i)
		if msg.Content != expected {
			t.Errorf("message %d: expected %q, got %q", i, expected, msg.Content)
		}
	}
}

func TestRequestHelp_ReturnsID(t *testing.T) {
	mb := NewMessageBus()
	mb.Register("agent-1")
	mb.Register("agent-2")

	id := mb.RequestHelp("agent-1", "need help parsing config")
	if id == "" {
		t.Error("expected non-empty message ID")
	}

	// Verify it appears in history
	history := mb.GetHistory("request", 1)
	if len(history) != 1 {
		t.Fatal("expected 1 request in history")
	}
	if history[0].ID != id {
		t.Errorf("expected ID %s, got %s", id, history[0].ID)
	}
	if !history[0].RequiresResponse {
		t.Error("request should have RequiresResponse=true")
	}
}

func TestDuplicateSubscription(t *testing.T) {
	mb := NewMessageBus()
	mb.Register("agent-1")

	mb.Subscribe("agent-1", "discovery")
	mb.Subscribe("agent-1", "discovery") // duplicate

	mb.mu.RLock()
	subs := mb.subscribers["discovery"]
	mb.mu.RUnlock()

	count := 0
	for _, s := range subs {
		if s == "agent-1" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("expected 1 subscription, got %d", count)
	}
}

func TestSendAutoFillsFields(t *testing.T) {
	mb := NewMessageBus()
	mb.Register("agent-1")
	mb.Register("agent-2")

	msg := AgentMessage{
		From:    "agent-1",
		To:      "agent-2",
		Content: "hello",
	}
	if err := mb.Send(msg); err != nil {
		t.Fatal(err)
	}

	history := mb.GetHistory("", 1)
	if len(history) != 1 {
		t.Fatal("expected 1 message")
	}
	if history[0].ID == "" {
		t.Error("expected auto-generated ID")
	}
	if history[0].Timestamp.IsZero() {
		t.Error("expected auto-filled timestamp")
	}
	if history[0].Priority != 3 {
		t.Errorf("expected default priority 3, got %d", history[0].Priority)
	}
}

// Helper functions

func agentName(i int) string {
	return "agent-" + itoa(i)
}

func orderContent(i int) string {
	return "step-" + itoa(i)
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	s := ""
	neg := false
	if i < 0 {
		neg = true
		i = -i
	}
	for i > 0 {
		s = string(rune('0'+i%10)) + s
		i /= 10
	}
	if neg {
		s = "-" + s
	}
	return s
}
