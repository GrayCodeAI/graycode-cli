package scaffold

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// Skill represents a reusable agent skill — a learned task sequence that can be replayed.
type Skill struct {
	ID          string      `json:"id"`
	Name        string      `json:"name"`
	Description string      `json:"description"`
	Steps       []SkillStep `json:"steps"`
	Tags        []string    `json:"tags"`
	Language    string      `json:"language"`
	SuccessRate float64     `json:"success_rate"`
	UsageCount  int         `json:"usage_count"`
	CreatedAt   time.Time   `json:"created_at"`
	Author      string      `json:"author"` // "agent", "user", "community"
}

// SkillStep represents a single step in a skill sequence.
type SkillStep struct {
	Order           int    `json:"order"`
	Action          string `json:"action"` // "prompt", "tool_call", "check"
	Content         string `json:"content"`
	ToolName        string `json:"tool_name,omitempty"`
	ExpectedOutcome string `json:"expected_outcome,omitempty"`
	Fallback        string `json:"fallback,omitempty"`
}

// SkillResult captures the outcome of executing a skill.
type SkillResult struct {
	SkillID        string        `json:"skill_id"`
	Success        bool          `json:"success"`
	StepsCompleted int           `json:"steps_completed"`
	TotalSteps     int           `json:"total_steps"`
	Duration       time.Duration `json:"duration"`
	Outputs        []string      `json:"outputs"`
}

// SkillRegistry manages a collection of reusable skills.
type SkillRegistry struct {
	Skills map[string]*Skill `json:"skills"`
	Dir    string            `json:"dir"`
	mu     sync.RWMutex
}

// NewSkillRegistry creates a new SkillRegistry backed by the given directory.
func NewSkillRegistry(dir string) *SkillRegistry {
	return &SkillRegistry{
		Skills: make(map[string]*Skill),
		Dir:    dir,
	}
}

// Register adds a skill to the registry.
func (r *SkillRegistry) Register(skill *Skill) error {
	if skill == nil {
		return fmt.Errorf("skill cannot be nil")
	}
	if skill.ID == "" {
		return fmt.Errorf("skill ID cannot be empty")
	}
	if skill.Name == "" {
		return fmt.Errorf("skill Name cannot be empty")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.Skills[skill.ID] = skill
	return nil
}

// Get retrieves a skill by ID.
func (r *SkillRegistry) Get(id string) *Skill {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.Skills[id]
}

// Search finds skills matching the query and/or tags, ranked by relevance and success rate.
func (r *SkillRegistry) Search(query string, tags []string) []*Skill {
	r.mu.RLock()
	defer r.mu.RUnlock()

	type scored struct {
		skill *Skill
		score float64
	}

	queryLower := strings.ToLower(query)
	queryWords := strings.Fields(queryLower)

	var results []scored
	for _, skill := range r.Skills {
		score := 0.0

		// Tag filtering: if tags specified, skill must match at least one.
		if len(tags) > 0 {
			tagMatch := false
			for _, reqTag := range tags {
				for _, skillTag := range skill.Tags {
					if strings.EqualFold(reqTag, skillTag) {
						tagMatch = true
						score += 2.0
						break
					}
				}
			}
			if !tagMatch {
				continue
			}
		}

		// Keyword search against name, description, and tags.
		if query != "" {
			nameLower := strings.ToLower(skill.Name)
			descLower := strings.ToLower(skill.Description)

			for _, word := range queryWords {
				if strings.Contains(nameLower, word) {
					score += 3.0
				}
				if strings.Contains(descLower, word) {
					score += 1.0
				}
				for _, tag := range skill.Tags {
					if strings.EqualFold(tag, word) {
						score += 2.0
					}
				}
			}
			// If no keyword matched at all, skip unless tags matched.
			if score == 0 && len(tags) == 0 {
				continue
			}
		}

		// Boost by success rate.
		score += skill.SuccessRate * 2.0

		if score > 0 {
			results = append(results, scored{skill: skill, score: score})
		}
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].score > results[j].score
	})

	out := make([]*Skill, len(results))
	for i, r := range results {
		out[i] = r.skill
	}
	return out
}

// Execute runs a skill step-by-step, substituting variables and tracking results.
func (r *SkillRegistry) Execute(ctx context.Context, skillID string, vars map[string]string, execFn func(string) (string, error)) (*SkillResult, error) {
	skill := r.Get(skillID)
	if skill == nil {
		return nil, fmt.Errorf("skill not found: %s", skillID)
	}

	start := time.Now()
	result := &SkillResult{
		SkillID:    skillID,
		TotalSteps: len(skill.Steps),
		Outputs:    make([]string, 0, len(skill.Steps)),
	}

	// Sort steps by order.
	steps := make([]SkillStep, len(skill.Steps))
	copy(steps, skill.Steps)
	sort.Slice(steps, func(i, j int) bool {
		return steps[i].Order < steps[j].Order
	})

	for _, step := range steps {
		select {
		case <-ctx.Done():
			result.Duration = time.Since(start)
			r.UpdateStats(skillID, false)
			return result, ctx.Err()
		default:
		}

		content := substituteVars(step.Content, vars)

		output, err := execFn(content)
		if err != nil {
			// Try fallback if available.
			if step.Fallback != "" {
				fallbackContent := substituteVars(step.Fallback, vars)
				output, err = execFn(fallbackContent)
			}
			if err != nil {
				result.Duration = time.Since(start)
				result.Outputs = append(result.Outputs, fmt.Sprintf("error: %v", err))
				r.UpdateStats(skillID, false)
				return result, fmt.Errorf("step %d failed: %w", step.Order, err)
			}
		}

		result.StepsCompleted++
		result.Outputs = append(result.Outputs, output)
	}

	result.Success = true
	result.Duration = time.Since(start)
	r.UpdateStats(skillID, true)
	return result, nil
}

// substituteVars replaces {{key}} placeholders with values from vars.
func substituteVars(content string, vars map[string]string) string {
	for k, v := range vars {
		content = strings.ReplaceAll(content, "{{"+k+"}}", v)
	}
	return content
}

// LearnFromSession extracts a reusable skill from a successful session.
func (r *SkillRegistry) LearnFromSession(goal string, toolCalls []string, outcome string) *Skill {
	steps := make([]SkillStep, 0, len(toolCalls))
	for i, call := range toolCalls {
		action := "tool_call"
		if strings.HasPrefix(call, "check:") {
			action = "check"
			call = strings.TrimPrefix(call, "check:")
		} else if strings.HasPrefix(call, "prompt:") {
			action = "prompt"
			call = strings.TrimPrefix(call, "prompt:")
		}

		// Generalize specific paths/names into variables.
		generalized := generalizePaths(call)

		steps = append(steps, SkillStep{
			Order:   i + 1,
			Action:  action,
			Content: generalized,
		})
	}

	id := fmt.Sprintf("learned_%d", time.Now().UnixNano())
	skill := &Skill{
		ID:          id,
		Name:        goal,
		Description: fmt.Sprintf("Learned from session: %s", outcome),
		Steps:       steps,
		Tags:        skillExtractTags(goal, toolCalls),
		SuccessRate: 1.0,
		UsageCount:  1,
		CreatedAt:   time.Now(),
		Author:      "agent",
	}
	return skill
}

// generalizePaths replaces absolute path-like segments with variable placeholders.
func generalizePaths(content string) string {
	// Replace common absolute path patterns with variables.
	words := strings.Fields(content)
	for i, word := range words {
		if strings.HasPrefix(word, "/") && strings.Contains(word, "/") {
			parts := strings.Split(word, "/")
			if len(parts) > 2 {
				// Keep the last component as the variable name hint.
				last := parts[len(parts)-1]
				if last != "" {
					words[i] = "{{path_" + sanitizeVarName(last) + "}}"
				}
			}
		}
	}
	return strings.Join(words, " ")
}

// sanitizeVarName makes a string safe for use as a variable name.
func sanitizeVarName(s string) string {
	s = strings.ReplaceAll(s, ".", "_")
	s = strings.ReplaceAll(s, "-", "_")
	s = strings.ReplaceAll(s, " ", "_")
	return strings.ToLower(s)
}

// skillExtractTags infers tags from goal text and tool calls.
func skillExtractTags(goal string, toolCalls []string) []string {
	tagSet := make(map[string]bool)
	combined := strings.ToLower(goal + " " + strings.Join(toolCalls, " "))

	keywords := map[string]string{
		"test":      "testing",
		"_test.go":  "testing",
		"go test":   "testing",
		"lint":      "linting",
		"fmt":       "formatting",
		"refactor":  "refactoring",
		"docker":    "docker",
		"git":       "git",
		"deploy":    "deployment",
		"migration": "migration",
		".go":       "go",
		".py":       "python",
		".ts":       "typescript",
		".js":       "javascript",
		".rs":       "rust",
		"debug":     "debugging",
		"fix":       "bugfix",
		"doc":       "documentation",
		"benchmark": "performance",
		"perf":      "performance",
	}

	for keyword, tag := range keywords {
		if strings.Contains(combined, keyword) {
			tagSet[tag] = true
		}
	}

	tags := make([]string, 0, len(tagSet))
	for tag := range tagSet {
		tags = append(tags, tag)
	}
	sort.Strings(tags)
	return tags
}

// FormatSkill produces a human-readable representation of a skill.
func FormatSkill(skill *Skill) string {
	if skill == nil {
		return ""
	}

	var b strings.Builder
	_, _ = fmt.Fprintf(&b, "Skill: %q\n", skill.Name)

	if len(skill.Tags) > 0 {
		_, _ = fmt.Fprintf(&b, "Tags: %v\n", skill.Tags)
	}

	if skill.UsageCount > 0 {
		successes := int(skill.SuccessRate * float64(skill.UsageCount))
		_, _ = fmt.Fprintf(&b, "Success rate: %.0f%% (%d/%d)\n", skill.SuccessRate*100, successes, skill.UsageCount)
	}

	if len(skill.Steps) > 0 {
		b.WriteString("Steps:\n")
		// Sort steps by order for display.
		steps := make([]SkillStep, len(skill.Steps))
		copy(steps, skill.Steps)
		sort.Slice(steps, func(i, j int) bool {
			return steps[i].Order < steps[j].Order
		})
		for _, step := range steps {
			_, _ = fmt.Fprintf(&b, "  %d. %s\n", step.Order, step.Content)
		}
	}

	return b.String()
}

// ListByTag returns all skills with the given tag.
func (r *SkillRegistry) ListByTag(tag string) []*Skill {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var results []*Skill
	for _, skill := range r.Skills {
		for _, t := range skill.Tags {
			if strings.EqualFold(t, tag) {
				results = append(results, skill)
				break
			}
		}
	}
	return results
}

// Save persists the registry to disk as JSON.
func (r *SkillRegistry) Save() error {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if err := os.MkdirAll(r.Dir, 0o750); err != nil {
		return fmt.Errorf("create skill dir: %w", err)
	}

	data, err := json.MarshalIndent(r.Skills, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal skills: %w", err)
	}

	path := filepath.Join(r.Dir, "skills.json")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("write skills file: %w", err)
	}
	return nil
}

// Load reads the registry from disk.
func (r *SkillRegistry) Load() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	path := filepath.Join(r.Dir, "skills.json")
	data, err := os.ReadFile(path) // #nosec G304 -- path provided by caller via tool/task parameters, inherent to this dev CLI's file operations
	if err != nil {
		if os.IsNotExist(err) {
			return nil // No file yet, start empty.
		}
		return fmt.Errorf("read skills file: %w", err)
	}

	skills := make(map[string]*Skill)
	if err := json.Unmarshal(data, &skills); err != nil {
		return fmt.Errorf("unmarshal skills: %w", err)
	}
	r.Skills = skills
	return nil
}

// Remove deletes a skill from the registry.
func (r *SkillRegistry) Remove(id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.Skills[id]; !ok {
		return fmt.Errorf("skill not found: %s", id)
	}
	delete(r.Skills, id)
	return nil
}

// UpdateStats updates the success rate and usage count for a skill.
func (r *SkillRegistry) UpdateStats(id string, success bool) {
	r.mu.Lock()
	defer r.mu.Unlock()

	skill, ok := r.Skills[id]
	if !ok {
		return
	}

	totalSuccesses := skill.SuccessRate * float64(skill.UsageCount)
	skill.UsageCount++
	if success {
		totalSuccesses++
	}
	skill.SuccessRate = totalSuccesses / float64(skill.UsageCount)
}
