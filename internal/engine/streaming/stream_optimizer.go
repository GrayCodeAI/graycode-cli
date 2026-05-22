package streaming

import (
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"
)

// StreamOptimizer buffers, deduplicates, and progressively renders LLM streaming
// output to improve perceived speed and terminal rendering quality.
type StreamOptimizer struct {
	BufferSize         int
	MinFlushInterval   time.Duration
	DeduplicateRepeats bool
	ProgressiveRender  bool

	mu         sync.Mutex
	buffer     strings.Builder
	lastFlush  time.Time
	totalChars int
	flushCount int
	startTime  time.Time
	dedupChars int
	lastChunk  string
}

// StreamStats holds statistics about stream processing.
type StreamStats struct {
	TotalChars        int
	FlushCount        int
	AvgFlushSize      int
	Duration          time.Duration
	CharsPerSecond    float64
	DeduplicatedChars int
	BufferedChars     int
}

// NewStreamOptimizer creates a StreamOptimizer with sensible defaults.
func NewStreamOptimizer() *StreamOptimizer {
	return &StreamOptimizer{
		BufferSize:         50,
		MinFlushInterval:   16 * time.Millisecond,
		DeduplicateRepeats: true,
		ProgressiveRender:  true,
		lastFlush:          time.Now(),
		startTime:          time.Now(),
	}
}

// Process takes raw stream chunks and returns optimized chunks that are buffered,
// deduplicated, and split at safe boundaries.
func (s *StreamOptimizer) Process(ch <-chan string) <-chan string {
	out := make(chan string, 16)
	go func() {
		defer close(out)
		ticker := time.NewTicker(s.MinFlushInterval)
		defer ticker.Stop()

		for {
			select {
			case chunk, ok := <-ch:
				if !ok {
					// Input closed, flush remaining buffer
					s.mu.Lock()
					remaining := s.buffer.String()
					s.buffer.Reset()
					if remaining != "" {
						s.totalChars += len(remaining)
						s.flushCount++
						s.lastFlush = time.Now()
					}
					s.mu.Unlock()
					if remaining != "" {
						out <- remaining
					}
					return
				}

				s.mu.Lock()
				// Deduplicate stuttered content
				if s.DeduplicateRepeats && chunk == s.lastChunk && chunk != "" {
					s.dedupChars += len(chunk)
					s.mu.Unlock()
					continue
				}
				s.lastChunk = chunk
				s.buffer.WriteString(chunk)

				if s.shouldFlushLocked() {
					buffered := s.buffer.String()
					buffered = s.DetectStutter(buffered)
					complete, remainder := s.DetectIncomplete(buffered)
					s.buffer.Reset()
					if remainder != "" {
						s.buffer.WriteString(remainder)
					}
					if complete != "" {
						s.totalChars += len(complete)
						s.flushCount++
						s.lastFlush = time.Now()
					}
					s.mu.Unlock()
					if complete != "" {
						out <- complete
					}
				} else {
					s.mu.Unlock()
				}

			case <-ticker.C:
				s.mu.Lock()
				if s.buffer.Len() > 0 && time.Since(s.lastFlush) >= s.MinFlushInterval {
					buffered := s.buffer.String()
					buffered = s.DetectStutter(buffered)
					complete, remainder := s.DetectIncomplete(buffered)
					s.buffer.Reset()
					if remainder != "" {
						s.buffer.WriteString(remainder)
					}
					if complete != "" {
						s.totalChars += len(complete)
						s.flushCount++
						s.lastFlush = time.Now()
					}
					s.mu.Unlock()
					if complete != "" {
						out <- complete
					}
				} else {
					s.mu.Unlock()
				}
			}
		}
	}()
	return out
}

// ShouldFlush reports whether the buffer should be flushed now.
func (s *StreamOptimizer) ShouldFlush() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.shouldFlushLocked()
}

func (s *StreamOptimizer) shouldFlushLocked() bool {
	bufLen := s.buffer.Len()
	if bufLen == 0 {
		return false
	}
	if bufLen >= s.BufferSize {
		return true
	}
	if time.Since(s.lastFlush) >= s.MinFlushInterval {
		return true
	}
	buf := s.buffer.String()
	// Complete line or paragraph
	if strings.HasSuffix(buf, "\n") || strings.HasSuffix(buf, "\n\n") {
		return true
	}
	// Complete code fence pair
	if hasCompleteCodeFence(buf) {
		return true
	}
	return false
}

// hasCompleteCodeFence checks if the buffer has an even number of code fences,
// meaning all opened fences are closed.
func hasCompleteCodeFence(buf string) bool {
	count := strings.Count(buf, "```")
	return count > 0 && count%2 == 0
}

// DetectStutter removes repeated content from the buffer. If the last N chars
// repeat the previous N chars, the duplicate is removed.
func (s *StreamOptimizer) DetectStutter(buffer string) string {
	if len(buffer) < 2 {
		return buffer
	}

	// Check for repeated substrings of various lengths
	for length := len(buffer) / 2; length >= 3; length-- {
		if len(buffer) < length*2 {
			continue
		}
		end := buffer[len(buffer)-length:]
		preceding := buffer[len(buffer)-length*2 : len(buffer)-length]
		if end == preceding {
			cleaned := buffer[:len(buffer)-length]
			s.mu.Lock()
			s.dedupChars += length
			s.mu.Unlock()
			return cleaned
		}
	}
	return buffer
}

// DetectIncomplete splits the buffer at the last safe break point.
// Safe breaks are: newline, space, period, comma.
// If inside an unclosed code fence, no flush happens until the fence is closed.
func (s *StreamOptimizer) DetectIncomplete(buffer string) (complete, remainder string) {
	if buffer == "" {
		return "", ""
	}

	// If inside an unclosed code fence, don't flush
	fenceCount := strings.Count(buffer, "```")
	if fenceCount%2 != 0 {
		return "", buffer
	}

	// Find last safe break point
	lastBreak := -1
	for i := len(buffer) - 1; i >= 0; i-- {
		ch := rune(buffer[i])
		if ch == '\n' || ch == ' ' || ch == '.' || ch == ',' {
			lastBreak = i
			break
		}
	}

	if lastBreak == -1 {
		// No safe break found; if buffer is large enough, flush all
		if len(buffer) >= s.BufferSize {
			return buffer, ""
		}
		return "", buffer
	}

	return buffer[:lastBreak+1], buffer[lastBreak+1:]
}

// WordWrap wraps text at word boundaries for terminal display.
func (s *StreamOptimizer) WordWrap(text string, width int) string {
	if width <= 0 {
		return text
	}

	var result strings.Builder
	lines := strings.Split(text, "\n")

	for li, line := range lines {
		if li > 0 {
			result.WriteByte('\n')
		}

		if len(line) <= width {
			result.WriteString(line)
			continue
		}

		words := strings.Fields(line)
		col := 0
		for i, word := range words {
			wlen := len(word)
			if col == 0 {
				result.WriteString(word)
				col = wlen
			} else if col+1+wlen > width {
				result.WriteByte('\n')
				result.WriteString(word)
				col = wlen
			} else {
				if i > 0 {
					result.WriteByte(' ')
					col++
				}
				result.WriteString(word)
				col += wlen
			}
		}
	}
	return result.String()
}

// ProgressIndicator returns a progress indicator string based on elapsed time
// and characters received.
func (s *StreamOptimizer) ProgressIndicator(elapsed time.Duration, charsReceived int) string {
	if charsReceived > 0 && elapsed > 0 {
		rate := float64(charsReceived) / elapsed.Seconds()
		return fmt.Sprintf("(%d chars/s)", int(rate))
	}
	if elapsed < 2*time.Second {
		spinChars := []rune{'⠋', '⠙', '⠹', '⠸', '⠼', '⠴', '⠦', '⠧', '⠇', '⠏'}
		idx := int(elapsed.Milliseconds()/100) % len(spinChars)
		return string(spinChars[idx])
	}
	return fmt.Sprintf("thinking... (%.1fs)", elapsed.Seconds())
}

// Stats returns statistics about the stream processing session.
func (s *StreamOptimizer) Stats() StreamStats {
	s.mu.Lock()
	defer s.mu.Unlock()

	duration := time.Since(s.startTime)
	var avgFlush int
	if s.flushCount > 0 {
		avgFlush = s.totalChars / s.flushCount
	}
	var cps float64
	if duration.Seconds() > 0 {
		cps = float64(s.totalChars) / duration.Seconds()
	}

	return StreamStats{
		TotalChars:        s.totalChars,
		FlushCount:        s.flushCount,
		AvgFlushSize:      avgFlush,
		Duration:          duration,
		CharsPerSecond:    cps,
		DeduplicatedChars: s.dedupChars,
		BufferedChars:     s.buffer.Len(),
	}
}

// Reset clears all state and statistics.
func (s *StreamOptimizer) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.buffer.Reset()
	s.lastFlush = time.Now()
	s.totalChars = 0
	s.flushCount = 0
	s.startTime = time.Now()
	s.dedupChars = 0
	s.lastChunk = ""
}

// pathCollapseRegex matches file paths for collapsing.
var pathCollapseRegex = regexp.MustCompile(`(/[^\s:]+/([^/\s:]+/[^/\s:]+))`)

// OptimizeToolOutput optimizes non-streaming tool output for display.
// It collapses long paths and repeated similar lines.
func (s *StreamOptimizer) OptimizeToolOutput(output string) string {
	if output == "" {
		return ""
	}

	lines := strings.Split(output, "\n")

	// Collapse long paths
	for i, line := range lines {
		lines[i] = pathCollapseRegex.ReplaceAllStringFunc(line, func(match string) string {
			parts := strings.Split(match, "/")
			if len(parts) > 4 {
				// Keep the last two path components
				return ".../" + parts[len(parts)-2] + "/" + parts[len(parts)-1]
			}
			return match
		})
	}

	// Collapse repeated similar lines
	var result []string
	i := 0
	for i < len(lines) {
		line := lines[i]
		// Count consecutive similar lines
		count := 1
		for i+count < len(lines) && isSimilar(line, lines[i+count]) {
			count++
		}
		if count > 3 {
			result = append(result, lines[i])
			result = append(result, lines[i+1])
			result = append(result, fmt.Sprintf("... (%d more similar lines)", count-2))
			i += count
		} else {
			result = append(result, line)
			i++
		}
	}

	return strings.Join(result, "\n")
}

// isSimilar checks if two lines are structurally similar (same length range
// and similar prefix).
func isSimilar(a, b string) bool {
	if a == "" && b == "" {
		return true
	}
	if a == "" || b == "" {
		return false
	}

	// Check length similarity (within 20%)
	lenA, lenB := len(a), len(b)
	if lenA == 0 || lenB == 0 {
		return false
	}
	ratio := float64(lenA) / float64(lenB)
	if ratio < 0.8 || ratio > 1.2 {
		return false
	}

	// Check if they share a common prefix (at least 40% of shorter line)
	minLen := lenA
	if lenB < minLen {
		minLen = lenB
	}
	threshold := minLen * 40 / 100
	if threshold < 1 {
		threshold = 1
	}

	common := 0
	for i := 0; i < minLen; i++ {
		if a[i] == b[i] {
			common++
		} else {
			break
		}
	}
	return common >= threshold
}
