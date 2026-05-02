package trace

import (
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestNewSessionTrace(t *testing.T) {
	st := NewSessionTrace("test-session-1")

	if st.SessionID != "test-session-1" {
		t.Fatalf("expected session ID 'test-session-1', got %q", st.SessionID)
	}
	if st.Root == nil {
		t.Fatal("expected non-nil root span")
	}
	if st.Root.Name != "session" {
		t.Fatalf("expected root span name 'session', got %q", st.Root.Name)
	}
	if st.Root.Attributes["session.id"] != "test-session-1" {
		t.Fatalf("expected root attribute session.id='test-session-1', got %q", st.Root.Attributes["session.id"])
	}
	if st.StartTime.IsZero() {
		t.Fatal("expected non-zero start time")
	}
}

func TestSessionTrace_StartSpan(t *testing.T) {
	st := NewSessionTrace("s1")

	spanID := st.StartSpan("api.chat", map[string]string{"model": "claude-sonnet"})
	if spanID == "" {
		t.Fatal("expected non-empty span ID")
	}

	span := st.GetSpan(spanID)
	if span == nil {
		t.Fatal("expected to find span by ID")
	}
	if span.Name != "api.chat" {
		t.Fatalf("expected span name 'api.chat', got %q", span.Name)
	}
	if span.Attributes["model"] != "claude-sonnet" {
		t.Fatalf("expected model attribute 'claude-sonnet', got %q", span.Attributes["model"])
	}
	if span.ParentID != st.Root.ID {
		t.Fatalf("expected parent to be root (%s), got %q", st.Root.ID, span.ParentID)
	}
}

func TestSessionTrace_SpanNesting(t *testing.T) {
	st := NewSessionTrace("s2")

	// Create a chain: session -> api.chat -> tool.Bash
	chatID := st.StartSpan("api.chat", map[string]string{"model": "claude-sonnet"})
	bashID := st.StartSpan("tool.Bash", map[string]string{"exit_code": "0"})

	// Verify parent-child chain.
	chatSpan := st.GetSpan(chatID)
	bashSpan := st.GetSpan(bashID)

	if chatSpan.ParentID != st.Root.ID {
		t.Fatalf("api.chat parent should be root, got %q", chatSpan.ParentID)
	}
	if bashSpan.ParentID != chatID {
		t.Fatalf("tool.Bash parent should be api.chat (%s), got %q", chatID, bashSpan.ParentID)
	}

	// Verify children slices.
	if len(st.Root.Children) != 1 {
		t.Fatalf("expected root to have 1 child, got %d", len(st.Root.Children))
	}
	if st.Root.Children[0].ID != chatID {
		t.Fatalf("expected root child to be api.chat")
	}
	if len(chatSpan.Children) != 1 {
		t.Fatalf("expected api.chat to have 1 child, got %d", len(chatSpan.Children))
	}
	if chatSpan.Children[0].ID != bashID {
		t.Fatalf("expected api.chat child to be tool.Bash")
	}
}

func TestSessionTrace_EndSpanPopsStack(t *testing.T) {
	st := NewSessionTrace("s3")

	chatID := st.StartSpan("api.chat", nil)
	bashID := st.StartSpan("tool.Bash", nil)

	// Current should be tool.Bash.
	if st.CurrentSpanID() != bashID {
		t.Fatalf("expected current span to be tool.Bash, got %q", st.CurrentSpanID())
	}

	// End tool.Bash, should pop back to api.chat.
	st.EndSpan(bashID, SpanOK)
	if st.CurrentSpanID() != chatID {
		t.Fatalf("expected current span to pop back to api.chat, got %q", st.CurrentSpanID())
	}

	// End api.chat, should pop back to root.
	st.EndSpan(chatID, SpanOK)
	if st.CurrentSpanID() != st.Root.ID {
		t.Fatalf("expected current span to pop back to root, got %q", st.CurrentSpanID())
	}
}

func TestSessionTrace_EndSpanSetsDuration(t *testing.T) {
	st := NewSessionTrace("s4")

	spanID := st.StartSpan("api.chat", nil)
	time.Sleep(10 * time.Millisecond)
	st.EndSpan(spanID, SpanOK)

	span := st.GetSpan(spanID)
	if span.Duration < 5*time.Millisecond {
		t.Fatalf("expected duration >= 5ms, got %v", span.Duration)
	}
	if span.EndTime.IsZero() {
		t.Fatal("expected non-zero end time")
	}
}

func TestSessionTrace_EndSpanWithError(t *testing.T) {
	st := NewSessionTrace("s5")

	spanID := st.StartSpan("tool.Bash", map[string]string{"exit_code": "1"})
	st.EndSpan(spanID, SpanError)

	span := st.GetSpan(spanID)
	if span.Status != SpanError {
		t.Fatalf("expected SpanError status, got %v", span.Status)
	}
}

func TestSessionTrace_EndSpanNonexistent(t *testing.T) {
	st := NewSessionTrace("s6")
	// Should not panic.
	st.EndSpan("nonexistent-id", SpanOK)
}

func TestSessionTrace_GetSpanNotFound(t *testing.T) {
	st := NewSessionTrace("s7")
	span := st.GetSpan("no-such-span")
	if span != nil {
		t.Fatal("expected nil for nonexistent span")
	}
}

func TestSessionTrace_NilAttributes(t *testing.T) {
	st := NewSessionTrace("s8")

	// Passing nil attrs should work without panic.
	spanID := st.StartSpan("api.chat", nil)
	span := st.GetSpan(spanID)
	if span.Attributes == nil {
		t.Fatal("expected non-nil attributes map even when nil is passed")
	}
}

func TestSessionTrace_SiblingSpans(t *testing.T) {
	st := NewSessionTrace("s9")

	// Create two sibling children under root.
	chat1ID := st.StartSpan("api.chat", map[string]string{"model": "claude-sonnet", "tokens_in": "1500"})
	st.EndSpan(chat1ID, SpanOK)

	chat2ID := st.StartSpan("api.chat", map[string]string{"model": "claude-sonnet", "tokens_in": "2200"})
	st.EndSpan(chat2ID, SpanOK)

	if len(st.Root.Children) != 2 {
		t.Fatalf("expected 2 children of root, got %d", len(st.Root.Children))
	}
	if st.Root.Children[0].ID != chat1ID {
		t.Fatal("first child should be chat1")
	}
	if st.Root.Children[1].ID != chat2ID {
		t.Fatal("second child should be chat2")
	}
}

func TestSessionTrace_Summary(t *testing.T) {
	st := NewSessionTrace("s10")

	// LLM call 1.
	c1 := st.StartSpan("api.chat", map[string]string{
		"model":      "claude-sonnet",
		"tokens_in":  "1500",
		"tokens_out": "200",
		"cost":       "0.005",
	})
	st.EndSpan(c1, SpanOK)

	// Tool call.
	t1 := st.StartSpan("tool.Bash", map[string]string{"exit_code": "0"})
	st.EndSpan(t1, SpanOK)

	// LLM call 2 with error.
	c2 := st.StartSpan("api.chat", map[string]string{
		"model":      "claude-sonnet",
		"tokens_in":  "2200",
		"tokens_out": "500",
		"cost":       "0.010",
	})
	st.EndSpan(c2, SpanError)

	// Tool call 2.
	t2 := st.StartSpan("tool.Edit", nil)
	st.EndSpan(t2, SpanOK)

	// End root to set total duration.
	st.EndSpan(st.Root.ID, SpanOK)

	summary := st.Summary()

	if summary.LLMCalls != 2 {
		t.Fatalf("expected 2 LLM calls, got %d", summary.LLMCalls)
	}
	if summary.ToolCalls != 2 {
		t.Fatalf("expected 2 tool calls, got %d", summary.ToolCalls)
	}
	if summary.Errors != 1 {
		t.Fatalf("expected 1 error, got %d", summary.Errors)
	}
	if summary.TotalTokensIn != 3700 {
		t.Fatalf("expected 3700 tokens in, got %d", summary.TotalTokensIn)
	}
	if summary.TotalTokensOut != 700 {
		t.Fatalf("expected 700 tokens out, got %d", summary.TotalTokensOut)
	}
	// Cost comparison with tolerance for floating point.
	expectedCost := 0.015
	if summary.TotalCostUSD < expectedCost-0.0001 || summary.TotalCostUSD > expectedCost+0.0001 {
		t.Fatalf("expected cost ~$0.015, got $%.4f", summary.TotalCostUSD)
	}
	if summary.TotalDuration == 0 {
		t.Fatal("expected non-zero total duration")
	}
}

func TestSessionTrace_SummaryNoTokens(t *testing.T) {
	st := NewSessionTrace("s11")

	// Span with no token/cost attributes.
	id := st.StartSpan("review", nil)
	st.EndSpan(id, SpanOK)

	summary := st.Summary()
	if summary.TotalTokensIn != 0 {
		t.Fatalf("expected 0 tokens in, got %d", summary.TotalTokensIn)
	}
	if summary.TotalCostUSD != 0 {
		t.Fatalf("expected 0 cost, got %f", summary.TotalCostUSD)
	}
}

func TestSessionTrace_FormatTree(t *testing.T) {
	st := NewSessionTrace("ft1")

	// Build a tree:
	// session
	// ├── api.chat (with model, tokens)
	// ├── tool.Bash (with exit_code)
	// └── api.chat
	//     └── tool.Edit

	c1 := st.StartSpan("api.chat", map[string]string{"model": "claude-sonnet", "tokens_in": "1500"})
	st.EndSpan(c1, SpanOK)

	t1 := st.StartSpan("tool.Bash", map[string]string{"exit_code": "0"})
	st.EndSpan(t1, SpanOK)

	c2 := st.StartSpan("api.chat", map[string]string{"model": "claude-sonnet", "tokens_in": "2200"})
	te := st.StartSpan("tool.Edit", nil)
	st.EndSpan(te, SpanOK)
	st.EndSpan(c2, SpanOK)

	st.EndSpan(st.Root.ID, SpanOK)

	tree := st.FormatTree()

	// Verify structure.
	if !strings.Contains(tree, "session [") {
		t.Fatal("tree should start with session")
	}
	if !strings.Contains(tree, "├── api.chat [") {
		t.Fatal("tree should contain api.chat with ├── prefix")
	}
	if !strings.Contains(tree, "├── tool.Bash [") {
		t.Fatal("tree should contain tool.Bash with ├── prefix")
	}
	if !strings.Contains(tree, "└── api.chat [") {
		t.Fatal("tree should contain last api.chat with └── prefix")
	}
	if !strings.Contains(tree, "    └── tool.Edit [") {
		t.Fatal("tree should contain nested tool.Edit with proper indentation")
	}
	if !strings.Contains(tree, "model=claude-sonnet") {
		t.Fatal("tree should show model attribute")
	}
	if !strings.Contains(tree, "exit=0") {
		t.Fatal("tree should show exit_code attribute")
	}
	if !strings.Contains(tree, "in=1500") {
		t.Fatal("tree should show tokens_in attribute")
	}
}

func TestSessionTrace_FormatTreeWithErrors(t *testing.T) {
	st := NewSessionTrace("ft2")

	id := st.StartSpan("api.chat", nil)
	st.EndSpan(id, SpanError)
	st.EndSpan(st.Root.ID, SpanOK)

	tree := st.FormatTree()
	if !strings.Contains(tree, "ERROR") {
		t.Fatal("tree should mark error spans with ERROR")
	}
}

func TestSessionTrace_FormatTreeEmpty(t *testing.T) {
	st := NewSessionTrace("ft3")
	st.EndSpan(st.Root.ID, SpanOK)

	tree := st.FormatTree()
	if !strings.Contains(tree, "session [") {
		t.Fatal("tree for empty session should still show root")
	}
	// Root with no children: just one line.
	lines := strings.Split(strings.TrimSpace(tree), "\n")
	if len(lines) != 1 {
		t.Fatalf("expected 1 line for root-only tree, got %d", len(lines))
	}
}

func TestSessionTrace_ToJSON(t *testing.T) {
	st := NewSessionTrace("json1")

	c1 := st.StartSpan("api.chat", map[string]string{
		"model":      "claude-sonnet",
		"tokens_in":  "1000",
		"tokens_out": "200",
	})
	st.EndSpan(c1, SpanOK)
	st.EndSpan(st.Root.ID, SpanOK)

	data, err := st.ToJSON()
	if err != nil {
		t.Fatalf("ToJSON error: %v", err)
	}

	// Parse the JSON back.
	var exported sessionTraceExport
	if err := json.Unmarshal(data, &exported); err != nil {
		t.Fatalf("failed to unmarshal JSON: %v", err)
	}

	if exported.SessionID != "json1" {
		t.Fatalf("expected session_id 'json1', got %q", exported.SessionID)
	}
	if exported.Root == nil {
		t.Fatal("expected non-nil root in JSON")
	}
	if exported.Root.Name != "session" {
		t.Fatalf("expected root name 'session', got %q", exported.Root.Name)
	}
	if len(exported.Root.Children) != 1 {
		t.Fatalf("expected 1 child in JSON root, got %d", len(exported.Root.Children))
	}
	child := exported.Root.Children[0]
	if child.Name != "api.chat" {
		t.Fatalf("expected child name 'api.chat', got %q", child.Name)
	}
	if child.Attributes["model"] != "claude-sonnet" {
		t.Fatalf("expected model 'claude-sonnet' in JSON, got %q", child.Attributes["model"])
	}
}

func TestSessionTrace_ToJSONSpanStatus(t *testing.T) {
	st := NewSessionTrace("json2")

	id := st.StartSpan("api.chat", nil)
	st.EndSpan(id, SpanError)
	st.EndSpan(st.Root.ID, SpanOK)

	data, err := st.ToJSON()
	if err != nil {
		t.Fatalf("ToJSON error: %v", err)
	}

	// Verify status fields serialize as strings.
	jsonStr := string(data)
	if !strings.Contains(jsonStr, `"status": "OK"`) {
		t.Fatal("expected root status to serialize as \"OK\"")
	}
	if !strings.Contains(jsonStr, `"status": "Error"`) {
		t.Fatal("expected child status to serialize as \"Error\"")
	}
}

func TestSessionTrace_ConcurrentSpanOperations(t *testing.T) {
	st := NewSessionTrace("conc1")

	var wg sync.WaitGroup
	spanIDs := make([]string, 50)

	// Start 50 spans concurrently under root.
	// Since StartSpan modifies `current`, concurrent calls will create
	// spans in an interleaved manner. The key requirement is no panics
	// or data races.
	wg.Add(50)
	for i := 0; i < 50; i++ {
		go func(idx int) {
			defer wg.Done()
			id := st.StartSpan("concurrent.op", map[string]string{
				"index": strings.Repeat("x", idx), // unique attr
			})
			spanIDs[idx] = id
		}(i)
	}
	wg.Wait()

	// End all spans concurrently.
	wg.Add(50)
	for i := 0; i < 50; i++ {
		go func(idx int) {
			defer wg.Done()
			st.EndSpan(spanIDs[idx], SpanOK)
		}(i)
	}
	wg.Wait()

	// Verify all spans were created.
	for i, id := range spanIDs {
		span := st.GetSpan(id)
		if span == nil {
			t.Fatalf("span %d with ID %q not found", i, id)
		}
		if span.EndTime.IsZero() {
			t.Fatalf("span %d was not ended", i)
		}
	}
}

func TestSessionTrace_ConcurrentSummary(t *testing.T) {
	st := NewSessionTrace("conc2")

	// Create some spans first.
	for i := 0; i < 10; i++ {
		id := st.StartSpan("api.chat", map[string]string{
			"tokens_in":  "100",
			"tokens_out": "50",
			"cost":       "0.001",
		})
		st.EndSpan(id, SpanOK)
	}

	// Concurrently call Summary.
	var wg sync.WaitGroup
	wg.Add(20)
	for i := 0; i < 20; i++ {
		go func() {
			defer wg.Done()
			summary := st.Summary()
			if summary.LLMCalls != 10 {
				t.Errorf("expected 10 LLM calls, got %d", summary.LLMCalls)
			}
		}()
	}
	wg.Wait()
}

func TestSessionTrace_ConcurrentFormatTree(t *testing.T) {
	st := NewSessionTrace("conc3")

	for i := 0; i < 5; i++ {
		id := st.StartSpan("tool.Bash", map[string]string{"exit_code": "0"})
		st.EndSpan(id, SpanOK)
	}

	// Concurrently call FormatTree.
	var wg sync.WaitGroup
	wg.Add(10)
	for i := 0; i < 10; i++ {
		go func() {
			defer wg.Done()
			tree := st.FormatTree()
			if !strings.Contains(tree, "session [") {
				t.Errorf("tree output missing session root")
			}
		}()
	}
	wg.Wait()
}

func TestSessionTrace_DeepNesting(t *testing.T) {
	st := NewSessionTrace("deep1")

	// Create a deep chain: session -> a -> b -> c -> d -> e.
	ids := make([]string, 5)
	names := []string{"a", "b", "c", "d", "e"}
	for i, name := range names {
		ids[i] = st.StartSpan(name, nil)
	}

	// End them in reverse order (stack behavior).
	for i := len(ids) - 1; i >= 0; i-- {
		st.EndSpan(ids[i], SpanOK)
	}

	// Verify nesting.
	current := st.Root
	for i, name := range names {
		if len(current.Children) != 1 {
			t.Fatalf("expected 1 child at depth %d, got %d", i, len(current.Children))
		}
		child := current.Children[0]
		if child.Name != name {
			t.Fatalf("expected name %q at depth %d, got %q", name, i, child.Name)
		}
		current = child
	}

	// The deepest span should have no children.
	if len(current.Children) != 0 {
		t.Fatalf("expected leaf to have 0 children, got %d", len(current.Children))
	}

	// FormatTree should render all levels.
	tree := st.FormatTree()
	for _, name := range names {
		if !strings.Contains(tree, name+" [") {
			t.Fatalf("tree should contain span %q", name)
		}
	}
}

func TestSpanStatus_String(t *testing.T) {
	if SpanOK.String() != "OK" {
		t.Fatalf("expected 'OK', got %q", SpanOK.String())
	}
	if SpanError.String() != "Error" {
		t.Fatalf("expected 'Error', got %q", SpanError.String())
	}
}

func TestSpanStatus_JSONRoundTrip(t *testing.T) {
	type wrapper struct {
		Status SpanStatus `json:"status"`
	}

	for _, tc := range []struct {
		status SpanStatus
		want   string
	}{
		{SpanOK, "OK"},
		{SpanError, "Error"},
	} {
		data, err := json.Marshal(wrapper{Status: tc.status})
		if err != nil {
			t.Fatalf("marshal error: %v", err)
		}
		if !strings.Contains(string(data), `"`+tc.want+`"`) {
			t.Fatalf("expected JSON to contain %q, got %s", tc.want, data)
		}

		var w wrapper
		if err := json.Unmarshal(data, &w); err != nil {
			t.Fatalf("unmarshal error: %v", err)
		}
		if w.Status != tc.status {
			t.Fatalf("expected status %v after roundtrip, got %v", tc.status, w.Status)
		}
	}
}

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		d    time.Duration
		want string
	}{
		{500 * time.Microsecond, "500µs"},
		{1 * time.Millisecond, "1ms"},
		{999 * time.Millisecond, "999ms"},
		{1500 * time.Millisecond, "1.5s"},
		{12300 * time.Millisecond, "12.3s"},
		{90 * time.Second, "1.5m"},
	}

	for _, tc := range tests {
		got := formatDuration(tc.d)
		if got != tc.want {
			t.Errorf("formatDuration(%v) = %q, want %q", tc.d, got, tc.want)
		}
	}
}

func TestSessionTrace_ComplexTree(t *testing.T) {
	// Build a realistic tree matching the example in the spec:
	// session [...]
	// ├── api.chat [...] model=claude-sonnet in=1500
	// ├── tool.Bash [...] exit=0
	// ├── api.chat [...] model=claude-sonnet in=2200
	// │   └── tool.Edit [...]
	// └── api.chat [...] model=claude-sonnet in=800

	st := NewSessionTrace("complex1")

	c1 := st.StartSpan("api.chat", map[string]string{"model": "claude-sonnet", "tokens_in": "1500"})
	st.EndSpan(c1, SpanOK)

	b1 := st.StartSpan("tool.Bash", map[string]string{"exit_code": "0"})
	st.EndSpan(b1, SpanOK)

	c2 := st.StartSpan("api.chat", map[string]string{"model": "claude-sonnet", "tokens_in": "2200"})
	e1 := st.StartSpan("tool.Edit", nil)
	st.EndSpan(e1, SpanOK)
	st.EndSpan(c2, SpanOK)

	c3 := st.StartSpan("api.chat", map[string]string{"model": "claude-sonnet", "tokens_in": "800"})
	st.EndSpan(c3, SpanOK)

	st.EndSpan(st.Root.ID, SpanOK)

	// Verify structure.
	if len(st.Root.Children) != 4 {
		t.Fatalf("expected 4 children of root, got %d", len(st.Root.Children))
	}

	// Verify the third child (api.chat with tool.Edit nested).
	thirdChild := st.Root.Children[2]
	if thirdChild.Name != "api.chat" {
		t.Fatalf("expected third child to be api.chat, got %q", thirdChild.Name)
	}
	if len(thirdChild.Children) != 1 {
		t.Fatalf("expected third child to have 1 child, got %d", len(thirdChild.Children))
	}
	if thirdChild.Children[0].Name != "tool.Edit" {
		t.Fatalf("expected nested child to be tool.Edit, got %q", thirdChild.Children[0].Name)
	}

	// Verify FormatTree output structure.
	tree := st.FormatTree()
	lines := strings.Split(strings.TrimRight(tree, "\n"), "\n")

	// Expect 6 lines: session, api.chat, tool.Bash, api.chat, tool.Edit, api.chat.
	if len(lines) != 6 {
		t.Fatalf("expected 6 lines in tree, got %d:\n%s", len(lines), tree)
	}

	// Summary check.
	summary := st.Summary()
	if summary.LLMCalls != 3 {
		t.Fatalf("expected 3 LLM calls, got %d", summary.LLMCalls)
	}
	if summary.ToolCalls != 2 {
		t.Fatalf("expected 2 tool calls, got %d", summary.ToolCalls)
	}
	if summary.TotalTokensIn != 4500 {
		t.Fatalf("expected 4500 tokens in, got %d", summary.TotalTokensIn)
	}
}
