package observability

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// Feedback represents explicit user feedback on an interaction.
type Feedback struct {
	ID        string    `json:"id"`
	SessionID string    `json:"session_id"`
	Rating    int       `json:"rating"`
	Comment   string    `json:"comment"`
	Category  string    `json:"category"`
	Context   string    `json:"context"`
	Timestamp time.Time `json:"timestamp"`
	TaskType  string    `json:"task_type"`
}

// FeedbackCollector gathers explicit and implicit user satisfaction signals
// and uses them to identify trends and improvement opportunities.
type FeedbackCollector struct {
	Entries         []Feedback       `json:"entries"`
	ImplicitSignals []ImplicitSignal `json:"implicit_signals"`
	Dir             string           `json:"dir"`
	mu              sync.RWMutex
}

// ImplicitSignal represents an inferred satisfaction signal from user behavior.
type ImplicitSignal struct {
	Type      string    `json:"type"`
	SessionID string    `json:"session_id"`
	ToolName  string    `json:"tool_name"`
	Timestamp time.Time `json:"timestamp"`
}

// validCategories defines the set of allowed feedback categories.
var validCategories = map[string]bool{
	"quality":     true,
	"speed":       true,
	"accuracy":    true,
	"helpfulness": true,
}

// validSignalTypes defines the set of allowed implicit signal types.
var validSignalTypes = map[string]bool{
	"accepted": true,
	"rejected": true,
	"edited":   true,
	"undone":   true,
	"retried":  true,
}

// NewFeedbackCollector creates a new FeedbackCollector that persists data to dir.
func NewFeedbackCollector(dir string) *FeedbackCollector {
	return &FeedbackCollector{
		Entries:         make([]Feedback, 0),
		ImplicitSignals: make([]ImplicitSignal, 0),
		Dir:             dir,
	}
}

// RecordExplicit records explicit user feedback with a rating and optional comment.
// Rating must be between 1 and 5. Category must be one of: quality, speed, accuracy, helpfulness.
func (fc *FeedbackCollector) RecordExplicit(rating int, comment, category, sessionID, taskType string) error {
	if rating < 1 || rating > 5 {
		return fmt.Errorf("rating must be between 1 and 5, got %d", rating)
	}
	if !validCategories[category] {
		return fmt.Errorf("invalid category %q: must be one of quality, speed, accuracy, helpfulness", category)
	}

	fc.mu.Lock()
	defer fc.mu.Unlock()

	fb := Feedback{
		ID:        fmt.Sprintf("fb_%d", time.Now().UnixNano()),
		SessionID: sessionID,
		Rating:    rating,
		Comment:   comment,
		Category:  category,
		Timestamp: time.Now(),
		TaskType:  taskType,
	}
	fc.Entries = append(fc.Entries, fb)
	return nil
}

// RecordImplicit records an implicit satisfaction signal inferred from user behavior.
// Signal types: accepted (positive), rejected/undone/retried (negative), edited (neutral-negative).
func (fc *FeedbackCollector) RecordImplicit(signal ImplicitSignal) error {
	if !validSignalTypes[signal.Type] {
		return fmt.Errorf("invalid signal type %q: must be one of accepted, rejected, edited, undone, retried", signal.Type)
	}

	fc.mu.Lock()
	defer fc.mu.Unlock()

	if signal.Timestamp.IsZero() {
		signal.Timestamp = time.Now()
	}
	fc.ImplicitSignals = append(fc.ImplicitSignals, signal)
	return nil
}

// GetSatisfactionScore computes a weighted average satisfaction score from all signals.
// Explicit ratings are weighted 3x compared to implicit signals.
// Returns a value between 0.0 and 5.0.
func (fc *FeedbackCollector) GetSatisfactionScore() float64 {
	fc.mu.RLock()
	defer fc.mu.RUnlock()

	if len(fc.Entries) == 0 && len(fc.ImplicitSignals) == 0 {
		return 0.0
	}

	var explicitSum float64
	var explicitCount int
	for _, e := range fc.Entries {
		explicitSum += float64(e.Rating)
		explicitCount++
	}

	var implicitSum float64
	var implicitCount int
	for _, s := range fc.ImplicitSignals {
		score := implicitSignalScore(s.Type)
		implicitSum += score
		implicitCount++
	}

	// Explicit ratings weighted 3x
	explicitWeight := 3.0
	implicitWeight := 1.0

	totalWeight := float64(explicitCount)*explicitWeight + float64(implicitCount)*implicitWeight
	if totalWeight == 0 {
		return 0.0
	}

	weightedSum := explicitSum*explicitWeight + implicitSum*implicitWeight
	score := weightedSum / totalWeight

	// Clamp to [0, 5]
	if score < 0 {
		score = 0
	}
	if score > 5 {
		score = 5
	}
	return math.Round(score*100) / 100
}

// implicitSignalScore maps signal types to a 1-5 scale equivalent.
func implicitSignalScore(signalType string) float64 {
	switch signalType {
	case "accepted":
		return 4.5
	case "edited":
		return 3.0
	case "rejected":
		return 2.0
	case "undone":
		return 1.5
	case "retried":
		return 2.0
	default:
		return 3.0
	}
}

// GetTrends analyzes recent feedback to identify satisfaction trends.
func (fc *FeedbackCollector) GetTrends() string {
	fc.mu.RLock()
	defer fc.mu.RUnlock()

	if len(fc.Entries) < 2 {
		return "Insufficient data for trend analysis"
	}

	// Sort entries by timestamp
	sorted := make([]Feedback, len(fc.Entries))
	copy(sorted, fc.Entries)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Timestamp.Before(sorted[j].Timestamp)
	})

	// Compare recent half vs older half
	mid := len(sorted) / 2
	olderEntries := sorted[:mid]
	recentEntries := sorted[mid:]

	olderAvg := avgRating(olderEntries)
	recentAvg := avgRating(recentEntries)
	diff := recentAvg - olderAvg

	var trends []string

	if diff > 0.1 {
		trends = append(trends, fmt.Sprintf("Quality improving: +%.1f over last %d sessions", diff, len(recentEntries)))
	} else if diff < -0.1 {
		trends = append(trends, fmt.Sprintf("Quality declining: %.1f over last %d sessions", diff, len(recentEntries)))
	} else {
		trends = append(trends, fmt.Sprintf("Quality stable over last %d sessions", len(recentEntries)))
	}

	// Check for speed complaints
	recentCount := feedbackMinInt(10, len(sorted))
	recentSlice := sorted[len(sorted)-recentCount:]
	speedComplaints := 0
	for _, e := range recentSlice {
		if e.Category == "speed" && e.Rating <= 3 {
			speedComplaints++
		}
	}
	if speedComplaints > 0 {
		trends = append(trends, fmt.Sprintf("Speed complaints in %d/%d recent sessions", speedComplaints, recentCount))
	}

	return strings.Join(trends, "; ")
}

// GetByCategory returns all feedback entries matching the given category.
func (fc *FeedbackCollector) GetByCategory(category string) []Feedback {
	fc.mu.RLock()
	defer fc.mu.RUnlock()

	var result []Feedback
	for _, e := range fc.Entries {
		if e.Category == category {
			result = append(result, e)
		}
	}
	return result
}

// IdentifyIssues analyzes negative feedback patterns and returns actionable insights.
func (fc *FeedbackCollector) IdentifyIssues() []string {
	fc.mu.RLock()
	defer fc.mu.RUnlock()

	var issues []string

	// Count undos
	undoCount := 0
	retryCount := 0
	retryAfterCodeWrite := 0
	rejectionCount := 0

	for _, s := range fc.ImplicitSignals {
		switch s.Type {
		case "undone":
			undoCount++
		case "retried":
			retryCount++
			if s.ToolName == "code_write" || s.ToolName == "file_write" {
				retryAfterCodeWrite++
			}
		case "rejected":
			rejectionCount++
		}
	}

	if undoCount >= 3 {
		issues = append(issues, "Multiple undos suggest incorrect edits")
	}

	if retryAfterCodeWrite >= 2 {
		issues = append(issues, "Retries after code_write indicate quality issues")
	} else if retryCount >= 3 {
		issues = append(issues, "Frequent retries suggest misunderstanding user intent")
	}

	if rejectionCount >= 3 {
		issues = append(issues, "High rejection rate indicates poor suggestion relevance")
	}

	// Check for low-rated categories
	categoryScores := make(map[string][]int)
	for _, e := range fc.Entries {
		categoryScores[e.Category] = append(categoryScores[e.Category], e.Rating)
	}

	for cat, ratings := range categoryScores {
		if len(ratings) >= 2 {
			avg := 0.0
			for _, r := range ratings {
				avg += float64(r)
			}
			avg /= float64(len(ratings))
			if avg < 3.0 {
				issues = append(issues, fmt.Sprintf("Low %s scores (avg %.1f/5) need attention", cat, avg))
			}
		}
	}

	if len(issues) == 0 {
		return []string{"No issues identified"}
	}
	return issues
}

// FormatReport generates a human-readable feedback report.
func (fc *FeedbackCollector) FormatReport() string {
	fc.mu.RLock()
	defer fc.mu.RUnlock()

	var sb strings.Builder

	sb.WriteString("Feedback Report:\n")
	sb.WriteString("═══════════════\n")

	// Overall satisfaction
	score := fc.getSatisfactionScoreLocked()
	sb.WriteString(fmt.Sprintf("Satisfaction: %.1f/5.0 (based on %d ratings)\n", score, len(fc.Entries)))

	// Implicit signal acceptance rate
	acceptRate := fc.getAcceptanceRateLocked()
	sb.WriteString(fmt.Sprintf("Implicit signals: %.0f%% acceptance rate\n", acceptRate*100))

	sb.WriteString("\nBy category:\n")
	categories := []string{"quality", "speed", "accuracy", "helpfulness"}
	for _, cat := range categories {
		avg := fc.categoryAvgLocked(cat)
		stars := renderStars(avg)
		sb.WriteString(fmt.Sprintf("  %-12s %.1f/5 %s\n", feedbackCapitalize(cat)+":", avg, stars))
	}

	// Trends
	sb.WriteString("\n")
	trends := fc.getTrendsLocked()
	sb.WriteString(fmt.Sprintf("Trends: %s\n", trends))

	// Issues
	issues := fc.identifyIssuesLocked()
	if len(issues) == 1 && issues[0] == "No issues identified" {
		sb.WriteString("Issues: none identified\n")
	} else {
		sb.WriteString(fmt.Sprintf("Issues: %s\n", strings.Join(issues, "; ")))
	}

	return sb.String()
}

// Save persists the feedback data to disk.
func (fc *FeedbackCollector) Save() error {
	fc.mu.RLock()
	defer fc.mu.RUnlock()

	if fc.Dir == "" {
		return fmt.Errorf("no directory configured for feedback persistence")
	}

	if err := os.MkdirAll(fc.Dir, 0o755); err != nil {
		return fmt.Errorf("creating feedback dir: %w", err)
	}

	data := struct {
		Entries         []Feedback       `json:"entries"`
		ImplicitSignals []ImplicitSignal `json:"implicit_signals"`
	}{
		Entries:         fc.Entries,
		ImplicitSignals: fc.ImplicitSignals,
	}

	raw, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling feedback: %w", err)
	}

	path := filepath.Join(fc.Dir, "feedback.json")
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		return fmt.Errorf("writing feedback file: %w", err)
	}
	return nil
}

// Load reads persisted feedback data from disk.
func (fc *FeedbackCollector) Load() error {
	fc.mu.Lock()
	defer fc.mu.Unlock()

	if fc.Dir == "" {
		return fmt.Errorf("no directory configured for feedback persistence")
	}

	path := filepath.Join(fc.Dir, "feedback.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // No data yet is not an error
		}
		return fmt.Errorf("reading feedback file: %w", err)
	}

	var data struct {
		Entries         []Feedback       `json:"entries"`
		ImplicitSignals []ImplicitSignal `json:"implicit_signals"`
	}
	if err := json.Unmarshal(raw, &data); err != nil {
		return fmt.Errorf("unmarshaling feedback: %w", err)
	}

	fc.Entries = data.Entries
	fc.ImplicitSignals = data.ImplicitSignals
	return nil
}

// --- internal helpers (must be called with lock held) ---

func (fc *FeedbackCollector) getSatisfactionScoreLocked() float64 {
	if len(fc.Entries) == 0 && len(fc.ImplicitSignals) == 0 {
		return 0.0
	}

	var explicitSum float64
	var explicitCount int
	for _, e := range fc.Entries {
		explicitSum += float64(e.Rating)
		explicitCount++
	}

	var implicitSum float64
	var implicitCount int
	for _, s := range fc.ImplicitSignals {
		implicitSum += implicitSignalScore(s.Type)
		implicitCount++
	}

	explicitWeight := 3.0
	implicitWeight := 1.0

	totalWeight := float64(explicitCount)*explicitWeight + float64(implicitCount)*implicitWeight
	if totalWeight == 0 {
		return 0.0
	}

	weightedSum := explicitSum*explicitWeight + implicitSum*implicitWeight
	score := weightedSum / totalWeight

	if score < 0 {
		score = 0
	}
	if score > 5 {
		score = 5
	}
	return math.Round(score*10) / 10
}

func (fc *FeedbackCollector) getAcceptanceRateLocked() float64 {
	if len(fc.ImplicitSignals) == 0 {
		return 0.0
	}
	accepted := 0
	for _, s := range fc.ImplicitSignals {
		if s.Type == "accepted" {
			accepted++
		}
	}
	return float64(accepted) / float64(len(fc.ImplicitSignals))
}

func (fc *FeedbackCollector) categoryAvgLocked(category string) float64 {
	var sum float64
	var count int
	for _, e := range fc.Entries {
		if e.Category == category {
			sum += float64(e.Rating)
			count++
		}
	}
	if count == 0 {
		return 0.0
	}
	return math.Round((sum/float64(count))*10) / 10
}

func (fc *FeedbackCollector) getTrendsLocked() string {
	if len(fc.Entries) < 2 {
		return "insufficient data"
	}

	sorted := make([]Feedback, len(fc.Entries))
	copy(sorted, fc.Entries)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Timestamp.Before(sorted[j].Timestamp)
	})

	mid := len(sorted) / 2
	olderAvg := avgRating(sorted[:mid])
	recentAvg := avgRating(sorted[mid:])
	diff := recentAvg - olderAvg

	if diff > 0.1 {
		return fmt.Sprintf("improving (+%.1f from last week)", diff)
	} else if diff < -0.1 {
		return fmt.Sprintf("declining (%.1f from last week)", diff)
	}
	return "stable"
}

func (fc *FeedbackCollector) identifyIssuesLocked() []string {
	var issues []string

	undoCount := 0
	retryCount := 0
	retryAfterCodeWrite := 0
	rejectionCount := 0

	for _, s := range fc.ImplicitSignals {
		switch s.Type {
		case "undone":
			undoCount++
		case "retried":
			retryCount++
			if s.ToolName == "code_write" || s.ToolName == "file_write" {
				retryAfterCodeWrite++
			}
		case "rejected":
			rejectionCount++
		}
	}

	if undoCount >= 3 {
		issues = append(issues, "Multiple undos suggest incorrect edits")
	}
	if retryAfterCodeWrite >= 2 {
		issues = append(issues, "Retries after code_write indicate quality issues")
	} else if retryCount >= 3 {
		issues = append(issues, "Frequent retries suggest misunderstanding user intent")
	}
	if rejectionCount >= 3 {
		issues = append(issues, "High rejection rate indicates poor suggestion relevance")
	}

	if len(issues) == 0 {
		return []string{"No issues identified"}
	}
	return issues
}

// --- package-level helpers ---

func avgRating(entries []Feedback) float64 {
	if len(entries) == 0 {
		return 0
	}
	sum := 0.0
	for _, e := range entries {
		sum += float64(e.Rating)
	}
	return sum / float64(len(entries))
}

func renderStars(rating float64) string {
	if rating <= 0 {
		return ""
	}
	full := int(rating)
	remainder := rating - float64(full)

	var sb strings.Builder
	for i := 0; i < full; i++ {
		sb.WriteString("⭐")
	}

	if remainder >= 0.75 {
		sb.WriteString("¾")
	} else if remainder >= 0.5 {
		sb.WriteString("½")
	} else if remainder >= 0.25 {
		sb.WriteString("¼")
	}

	return sb.String()
}

func feedbackCapitalize(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

func feedbackMinInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
