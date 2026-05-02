package trace

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"
)

// SessionSpan represents a single operation within a session trace tree.
// It forms a tree structure via the Children slice, with ParentID linking
// back to the parent span.
type SessionSpan struct {
	ID         string            `json:"id"`
	ParentID   string            `json:"parent_id,omitempty"`
	Name       string            `json:"name"`
	StartTime  time.Time         `json:"start_time"`
	EndTime    time.Time         `json:"end_time,omitempty"`
	Duration   time.Duration     `json:"duration,omitempty"`
	Status     SpanStatus        `json:"status"`
	Attributes map[string]string `json:"attributes,omitempty"`
	Children   []*SessionSpan    `json:"children,omitempty"`
}

// SpanStatus represents the outcome of a span.
type SpanStatus int

const (
	// SpanOK indicates the span completed successfully.
	SpanOK SpanStatus = iota
	// SpanError indicates the span completed with an error.
	SpanError
)

// String returns a human-readable representation of the SpanStatus.
func (s SpanStatus) String() string {
	switch s {
	case SpanOK:
		return "OK"
	case SpanError:
		return "Error"
	default:
		return "Unknown"
	}
}

// MarshalJSON implements json.Marshaler for SpanStatus.
func (s SpanStatus) MarshalJSON() ([]byte, error) {
	return json.Marshal(s.String())
}

// UnmarshalJSON implements json.Unmarshaler for SpanStatus.
func (s *SpanStatus) UnmarshalJSON(data []byte) error {
	var str string
	if err := json.Unmarshal(data, &str); err != nil {
		return err
	}
	switch str {
	case "OK":
		*s = SpanOK
	case "Error":
		*s = SpanError
	default:
		return fmt.Errorf("unknown SpanStatus: %q", str)
	}
	return nil
}

// TraceSummary holds aggregate statistics for a session trace.
type TraceSummary struct {
	TotalDuration time.Duration `json:"total_duration"`
	LLMCalls      int           `json:"llm_calls"`
	ToolCalls     int           `json:"tool_calls"`
	TotalTokensIn int           `json:"total_tokens_in"`
	TotalTokensOut int          `json:"total_tokens_out"`
	TotalCostUSD  float64       `json:"total_cost_usd"`
	Errors        int           `json:"errors"`
}

// SessionTrace groups all operations in a session into a parent-child
// hierarchy, inspired by Helicone's session-level trace trees.
type SessionTrace struct {
	SessionID string    `json:"session_id"`
	StartTime time.Time `json:"start_time"`
	Root      *SessionSpan `json:"root"`

	mu      sync.Mutex
	spans   map[string]*SessionSpan
	current string // current active span ID
	spanSeq int64  // monotonic counter for generating unique IDs
}

// NewSessionTrace creates a new trace for a session. It initializes a root
// span that represents the entire session.
func NewSessionTrace(sessionID string) *SessionTrace {
	now := time.Now()
	rootID := fmt.Sprintf("ss-%s-0", sessionID)

	root := &SessionSpan{
		ID:         rootID,
		Name:       "session",
		StartTime:  now,
		Status:     SpanOK,
		Attributes: map[string]string{"session.id": sessionID},
	}

	st := &SessionTrace{
		SessionID: sessionID,
		StartTime: now,
		Root:      root,
		spans:     make(map[string]*SessionSpan),
		current:   rootID,
		spanSeq:   0,
	}
	st.spans[rootID] = root
	return st
}

// StartSpan begins a new span as a child of the current active span.
// It returns the new span's ID. The new span becomes the current active span,
// forming a stack-like push behavior.
func (st *SessionTrace) StartSpan(name string, attrs map[string]string) string {
	st.mu.Lock()
	defer st.mu.Unlock()

	st.spanSeq++
	spanID := fmt.Sprintf("ss-%s-%d", st.SessionID, st.spanSeq)

	parentID := st.current

	span := &SessionSpan{
		ID:         spanID,
		ParentID:   parentID,
		Name:       name,
		StartTime:  time.Now(),
		Status:     SpanOK,
		Attributes: make(map[string]string),
	}

	// Copy provided attributes.
	for k, v := range attrs {
		span.Attributes[k] = v
	}

	// Link as child of parent.
	if parent, ok := st.spans[parentID]; ok {
		parent.Children = append(parent.Children, span)
	}

	st.spans[spanID] = span
	st.current = spanID

	return spanID
}

// EndSpan completes the span identified by spanID and returns the current
// active span to its parent. If spanID does not match the current active span,
// the span is still ended but the active span pointer is only adjusted if it
// matches.
func (st *SessionTrace) EndSpan(spanID string, status SpanStatus) {
	st.mu.Lock()
	defer st.mu.Unlock()

	span, ok := st.spans[spanID]
	if !ok {
		return
	}

	span.EndTime = time.Now()
	span.Duration = span.EndTime.Sub(span.StartTime)
	span.Status = status

	// Pop back to parent if this is the current span.
	if st.current == spanID {
		st.current = span.ParentID
	}
}

// GetSpan retrieves a span by ID. Returns nil if not found.
func (st *SessionTrace) GetSpan(spanID string) *SessionSpan {
	st.mu.Lock()
	defer st.mu.Unlock()

	return st.spans[spanID]
}

// CurrentSpanID returns the ID of the currently active span.
func (st *SessionTrace) CurrentSpanID() string {
	st.mu.Lock()
	defer st.mu.Unlock()
	return st.current
}

// Summary returns aggregate statistics for the session trace by walking
// all spans and parsing their attributes for token counts and cost.
func (st *SessionTrace) Summary() TraceSummary {
	st.mu.Lock()
	defer st.mu.Unlock()

	var summary TraceSummary

	// Compute total duration from root.
	if st.Root != nil {
		if !st.Root.EndTime.IsZero() {
			summary.TotalDuration = st.Root.Duration
		} else {
			summary.TotalDuration = time.Since(st.Root.StartTime)
		}
	}

	for _, span := range st.spans {
		// Count LLM calls (spans starting with "api.").
		if strings.HasPrefix(span.Name, "api.") {
			summary.LLMCalls++
		}

		// Count tool calls (spans starting with "tool.").
		if strings.HasPrefix(span.Name, "tool.") {
			summary.ToolCalls++
		}

		// Count errors.
		if span.Status == SpanError {
			summary.Errors++
		}

		// Parse token and cost attributes.
		if v, ok := span.Attributes["tokens_in"]; ok {
			if n, err := strconv.Atoi(v); err == nil {
				summary.TotalTokensIn += n
			}
		}
		if v, ok := span.Attributes["tokens_out"]; ok {
			if n, err := strconv.Atoi(v); err == nil {
				summary.TotalTokensOut += n
			}
		}
		if v, ok := span.Attributes["cost"]; ok {
			if f, err := strconv.ParseFloat(v, 64); err == nil {
				summary.TotalCostUSD += f
			}
		}
	}

	return summary
}

// FormatTree renders the trace as an indented tree string using box-drawing
// characters, similar to the `tree` command output.
func (st *SessionTrace) FormatTree() string {
	st.mu.Lock()
	defer st.mu.Unlock()

	if st.Root == nil {
		return ""
	}

	var b strings.Builder
	formatSpan(&b, st.Root, "", true, true)
	return b.String()
}

// formatSpan recursively renders a span and its children into the builder.
func formatSpan(b *strings.Builder, span *SessionSpan, prefix string, isLast bool, isRoot bool) {
	// Write the connector.
	if isRoot {
		// No connector for the root span.
	} else if isLast {
		b.WriteString(prefix + "└── ")
	} else {
		b.WriteString(prefix + "├── ")
	}

	// Write span name and duration.
	dur := span.Duration
	if dur == 0 && !span.StartTime.IsZero() {
		if !span.EndTime.IsZero() {
			dur = span.EndTime.Sub(span.StartTime)
		} else {
			dur = time.Since(span.StartTime)
		}
	}
	b.WriteString(fmt.Sprintf("%s [%s]", span.Name, formatDuration(dur)))

	// Write selected attributes inline.
	attrParts := formatInlineAttrs(span)
	if len(attrParts) > 0 {
		b.WriteString(" " + strings.Join(attrParts, " "))
	}

	// Mark errors.
	if span.Status == SpanError {
		b.WriteString(" ERROR")
	}

	b.WriteString("\n")

	// Recurse into children.
	childPrefix := prefix
	if !isRoot {
		if isLast {
			childPrefix += "    "
		} else {
			childPrefix += "│   "
		}
	}

	for i, child := range span.Children {
		isLastChild := i == len(span.Children)-1
		formatSpan(b, child, childPrefix, isLastChild, false)
	}
}

// formatInlineAttrs selects and formats key attributes for inline display.
func formatInlineAttrs(span *SessionSpan) []string {
	var parts []string

	// Display order: model, tokens_in, tokens_out, cost, exit_code, error.message
	displayKeys := []string{"model", "tokens_in", "tokens_out", "cost", "exit_code", "error.message"}

	for _, key := range displayKeys {
		if v, ok := span.Attributes[key]; ok {
			switch key {
			case "tokens_in":
				parts = append(parts, fmt.Sprintf("in=%s", v))
			case "tokens_out":
				parts = append(parts, fmt.Sprintf("out=%s", v))
			case "cost":
				parts = append(parts, fmt.Sprintf("cost=$%s", v))
			case "exit_code":
				parts = append(parts, fmt.Sprintf("exit=%s", v))
			default:
				parts = append(parts, fmt.Sprintf("%s=%s", key, v))
			}
		}
	}

	return parts
}

// formatDuration formats a duration in a concise human-readable way.
func formatDuration(d time.Duration) string {
	if d < time.Millisecond {
		return fmt.Sprintf("%dµs", d.Microseconds())
	}
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	if d < time.Minute {
		return fmt.Sprintf("%.1fs", d.Seconds())
	}
	return fmt.Sprintf("%.1fm", d.Minutes())
}

// ToJSON exports the full session trace as JSON for external tools
// such as Jaeger, Grafana, or custom dashboards.
func (st *SessionTrace) ToJSON() ([]byte, error) {
	st.mu.Lock()
	defer st.mu.Unlock()

	return json.MarshalIndent(st.exportView(), "", "  ")
}

// sessionTraceExport is the JSON-serializable view of a SessionTrace.
type sessionTraceExport struct {
	SessionID string       `json:"session_id"`
	StartTime time.Time    `json:"start_time"`
	Root      *SessionSpan `json:"root"`
}

// exportView returns a snapshot suitable for JSON marshaling.
// Must be called under lock.
func (st *SessionTrace) exportView() sessionTraceExport {
	return sessionTraceExport{
		SessionID: st.SessionID,
		StartTime: st.StartTime,
		Root:      st.Root,
	}
}
