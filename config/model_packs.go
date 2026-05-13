package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

// ModelRole defines a model configuration for a specific role within a pack.
type ModelRole struct {
	Provider    string  `json:"provider"`
	Model       string  `json:"model"`
	Temperature float64 `json:"temperature"`
	MaxTokens   int     `json:"max_tokens"`
	Purpose     string  `json:"purpose"`
}

// ModelPack is a named configuration that bundles model + provider + settings
// for different use cases.
type ModelPack struct {
	Name            string                 `json:"name"`
	Description     string                 `json:"description"`
	Models          map[string]ModelRole   `json:"models"`
	DefaultProvider string                 `json:"default_provider"`
	Settings        map[string]interface{} `json:"settings"`
	Tags            []string               `json:"tags"`
	Author          string                 `json:"author"`
}

// ModelPackRegistry holds all registered packs and tracks the active one.
type ModelPackRegistry struct {
	Packs      map[string]*ModelPack `json:"packs"`
	ActivePack string               `json:"active_pack"`
	mu         sync.RWMutex
}

// NewModelPackRegistry creates a registry pre-loaded with built-in packs.
func NewModelPackRegistry() *ModelPackRegistry {
	r := &ModelPackRegistry{
		Packs:      make(map[string]*ModelPack),
		ActivePack: "default",
	}

	r.Packs["default"] = &ModelPack{
		Name:        "default",
		Description: "Balanced defaults: sonnet for code, haiku for summarize, opus for complex tasks",
		Models: map[string]ModelRole{
			"code":      {Provider: "anthropic", Model: "claude-sonnet-4-6", Temperature: 0.2, MaxTokens: 4096, Purpose: "code generation and editing"},
			"chat":      {Provider: "anthropic", Model: "claude-sonnet-4-6", Temperature: 0.7, MaxTokens: 2048, Purpose: "interactive conversation"},
			"summarize": {Provider: "anthropic", Model: "claude-haiku-4-5", Temperature: 0.3, MaxTokens: 1024, Purpose: "summarization"},
			"review":    {Provider: "anthropic", Model: "claude-sonnet-4-6", Temperature: 0.1, MaxTokens: 4096, Purpose: "code review"},
			"plan":      {Provider: "anthropic", Model: "claude-opus-4-6", Temperature: 0.4, MaxTokens: 8192, Purpose: "complex planning and architecture"},
			"debug":     {Provider: "anthropic", Model: "claude-opus-4-6", Temperature: 0.2, MaxTokens: 4096, Purpose: "debugging complex issues"},
		},
		DefaultProvider: "anthropic",
		Settings:        map[string]interface{}{"stream": true},
		Tags:            []string{"recommended", "general"},
		Author:          "hawk",
	}

	r.Packs["budget"] = &ModelPack{
		Name:        "budget",
		Description: "Cost-optimized: haiku for everything, sonnet only for complex tasks",
		Models: map[string]ModelRole{
			"code":      {Provider: "anthropic", Model: "claude-haiku-4-5", Temperature: 0.2, MaxTokens: 4096, Purpose: "code generation and editing"},
			"chat":      {Provider: "anthropic", Model: "claude-haiku-4-5", Temperature: 0.7, MaxTokens: 2048, Purpose: "interactive conversation"},
			"summarize": {Provider: "anthropic", Model: "claude-haiku-4-5", Temperature: 0.3, MaxTokens: 1024, Purpose: "summarization"},
			"review":    {Provider: "anthropic", Model: "claude-haiku-4-5", Temperature: 0.1, MaxTokens: 2048, Purpose: "code review"},
			"plan":      {Provider: "anthropic", Model: "claude-sonnet-4-6", Temperature: 0.4, MaxTokens: 4096, Purpose: "complex planning"},
			"debug":     {Provider: "anthropic", Model: "claude-haiku-4-5", Temperature: 0.2, MaxTokens: 2048, Purpose: "debugging"},
		},
		DefaultProvider: "anthropic",
		Settings:        map[string]interface{}{"stream": true, "max_retries": 2},
		Tags:            []string{"cost-effective", "fast"},
		Author:          "hawk",
	}

	r.Packs["quality"] = &ModelPack{
		Name:        "quality",
		Description: "Quality-optimized: opus for code, sonnet for everything else",
		Models: map[string]ModelRole{
			"code":      {Provider: "anthropic", Model: "claude-opus-4-6", Temperature: 0.2, MaxTokens: 8192, Purpose: "code generation and editing"},
			"chat":      {Provider: "anthropic", Model: "claude-sonnet-4-6", Temperature: 0.7, MaxTokens: 4096, Purpose: "interactive conversation"},
			"summarize": {Provider: "anthropic", Model: "claude-sonnet-4-6", Temperature: 0.3, MaxTokens: 2048, Purpose: "summarization"},
			"review":    {Provider: "anthropic", Model: "claude-opus-4-6", Temperature: 0.1, MaxTokens: 8192, Purpose: "code review"},
			"plan":      {Provider: "anthropic", Model: "claude-opus-4-6", Temperature: 0.4, MaxTokens: 8192, Purpose: "complex planning and architecture"},
			"debug":     {Provider: "anthropic", Model: "claude-opus-4-6", Temperature: 0.2, MaxTokens: 8192, Purpose: "debugging complex issues"},
		},
		DefaultProvider: "anthropic",
		Settings:        map[string]interface{}{"stream": true, "max_retries": 3},
		Tags:            []string{"premium", "thorough"},
		Author:          "hawk",
	}

	r.Packs["speed"] = &ModelPack{
		Name:        "speed",
		Description: "Speed-optimized: haiku for everything, lowest latency",
		Models: map[string]ModelRole{
			"code":      {Provider: "anthropic", Model: "claude-haiku-4-5", Temperature: 0.2, MaxTokens: 2048, Purpose: "code generation"},
			"chat":      {Provider: "anthropic", Model: "claude-haiku-4-5", Temperature: 0.7, MaxTokens: 1024, Purpose: "interactive conversation"},
			"summarize": {Provider: "anthropic", Model: "claude-haiku-4-5", Temperature: 0.3, MaxTokens: 512, Purpose: "summarization"},
			"review":    {Provider: "anthropic", Model: "claude-haiku-4-5", Temperature: 0.1, MaxTokens: 2048, Purpose: "code review"},
			"plan":      {Provider: "anthropic", Model: "claude-haiku-4-5", Temperature: 0.4, MaxTokens: 2048, Purpose: "planning"},
			"debug":     {Provider: "anthropic", Model: "claude-haiku-4-5", Temperature: 0.2, MaxTokens: 2048, Purpose: "debugging"},
		},
		DefaultProvider: "anthropic",
		Settings:        map[string]interface{}{"stream": true, "timeout_ms": 5000},
		Tags:            []string{"fast", "low-latency"},
		Author:          "hawk",
	}

	r.Packs["local"] = &ModelPack{
		Name:        "local",
		Description: "Local models via ollama for all roles",
		Models: map[string]ModelRole{
			"code":      {Provider: "ollama", Model: "codellama:13b", Temperature: 0.2, MaxTokens: 4096, Purpose: "code generation"},
			"chat":      {Provider: "ollama", Model: "llama3:8b", Temperature: 0.7, MaxTokens: 2048, Purpose: "interactive conversation"},
			"summarize": {Provider: "ollama", Model: "llama3:8b", Temperature: 0.3, MaxTokens: 1024, Purpose: "summarization"},
			"review":    {Provider: "ollama", Model: "codellama:13b", Temperature: 0.1, MaxTokens: 4096, Purpose: "code review"},
			"plan":      {Provider: "ollama", Model: "llama3:70b", Temperature: 0.4, MaxTokens: 4096, Purpose: "planning"},
			"debug":     {Provider: "ollama", Model: "codellama:13b", Temperature: 0.2, MaxTokens: 4096, Purpose: "debugging"},
		},
		DefaultProvider: "ollama",
		Settings:        map[string]interface{}{"stream": true, "base_url": "http://localhost:11434"},
		Tags:            []string{"local", "private", "offline"},
		Author:          "hawk",
	}

	r.Packs["balanced"] = &ModelPack{
		Name:        "balanced",
		Description: "Balanced: sonnet for code/review, haiku for chat/summarize",
		Models: map[string]ModelRole{
			"code":      {Provider: "anthropic", Model: "claude-sonnet-4-6", Temperature: 0.2, MaxTokens: 4096, Purpose: "code generation and editing"},
			"chat":      {Provider: "anthropic", Model: "claude-haiku-4-5", Temperature: 0.7, MaxTokens: 2048, Purpose: "interactive conversation"},
			"summarize": {Provider: "anthropic", Model: "claude-haiku-4-5", Temperature: 0.3, MaxTokens: 1024, Purpose: "summarization"},
			"review":    {Provider: "anthropic", Model: "claude-sonnet-4-6", Temperature: 0.1, MaxTokens: 4096, Purpose: "code review"},
			"plan":      {Provider: "anthropic", Model: "claude-sonnet-4-6", Temperature: 0.4, MaxTokens: 4096, Purpose: "planning"},
			"debug":     {Provider: "anthropic", Model: "claude-sonnet-4-6", Temperature: 0.2, MaxTokens: 4096, Purpose: "debugging"},
		},
		DefaultProvider: "anthropic",
		Settings:        map[string]interface{}{"stream": true},
		Tags:            []string{"balanced", "general"},
		Author:          "hawk",
	}

	return r
}

// GetModel returns the ModelRole for the given role from the active pack.
// If the role is not found, it returns a zero-value ModelRole.
func (r *ModelPackRegistry) GetModel(role string) ModelRole {
	r.mu.RLock()
	defer r.mu.RUnlock()

	pack, ok := r.Packs[r.ActivePack]
	if !ok {
		return ModelRole{}
	}
	mr, ok := pack.Models[role]
	if !ok {
		return ModelRole{}
	}
	return mr
}

// SetActive switches the active pack. Returns an error if the pack does not exist.
func (r *ModelPackRegistry) SetActive(packName string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.Packs[packName]; !ok {
		available := make([]string, 0, len(r.Packs))
		for name := range r.Packs {
			available = append(available, name)
		}
		sort.Strings(available)
		return fmt.Errorf("model pack %q not found; available: %s", packName, strings.Join(available, ", "))
	}
	r.ActivePack = packName
	return nil
}

// Register adds a new pack to the registry. If a pack with the same name
// already exists it will be overwritten.
func (r *ModelPackRegistry) Register(pack *ModelPack) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.Packs[pack.Name] = pack
}

// List returns all registered packs sorted by name.
func (r *ModelPackRegistry) List() []*ModelPack {
	r.mu.RLock()
	defer r.mu.RUnlock()

	names := make([]string, 0, len(r.Packs))
	for name := range r.Packs {
		names = append(names, name)
	}
	sort.Strings(names)

	packs := make([]*ModelPack, 0, len(names))
	for _, name := range names {
		packs = append(packs, r.Packs[name])
	}
	return packs
}

// FormatPack returns a human-readable formatted string for a model pack.
func FormatPack(pack *ModelPack) string {
	if pack == nil {
		return ""
	}

	var b strings.Builder
	b.WriteString(fmt.Sprintf("Model Pack: %q\n", pack.Name))
	b.WriteString(strings.Repeat("─", 20))
	b.WriteString("\n")

	// Define a consistent role order for display.
	roleOrder := []string{"code", "review", "chat", "summarize", "plan", "debug"}

	for _, role := range roleOrder {
		mr, ok := pack.Models[role]
		if !ok {
			continue
		}
		b.WriteString(fmt.Sprintf("%-10s %s (temp: %.1f)\n", role+":", mr.Model, mr.Temperature))
	}

	// Print any extra roles not in the standard order.
	extra := make([]string, 0)
	for role := range pack.Models {
		found := false
		for _, r := range roleOrder {
			if role == r {
				found = true
				break
			}
		}
		if !found {
			extra = append(extra, role)
		}
	}
	sort.Strings(extra)
	for _, role := range extra {
		mr := pack.Models[role]
		b.WriteString(fmt.Sprintf("%-10s %s (temp: %.1f)\n", role+":", mr.Model, mr.Temperature))
	}

	b.WriteString(fmt.Sprintf("\nProvider: %s\n", pack.DefaultProvider))
	return b.String()
}

// costPerToken returns approximate cost per 1K tokens for known models.
// These are rough estimates for cost comparison purposes.
func costPerToken(model string) float64 {
	switch {
	case strings.Contains(model, "opus"):
		return 0.075 // $75 per 1M tokens average (input+output)
	case strings.Contains(model, "sonnet"):
		return 0.015 // $15 per 1M tokens average
	case strings.Contains(model, "haiku"):
		return 0.005 // $5 per 1M tokens average
	case strings.Contains(model, "llama"), strings.Contains(model, "codellama"):
		return 0.0 // local models are free
	default:
		return 0.01
	}
}

// EstimateCost estimates the cost of a session with the given pack based on
// approximate tokens per session distributed evenly across roles.
func EstimateCost(pack *ModelPack, tokensPerSession int) float64 {
	if pack == nil || len(pack.Models) == 0 {
		return 0.0
	}

	totalCost := 0.0
	roleCount := len(pack.Models)
	tokensPerRole := tokensPerSession / roleCount

	for _, mr := range pack.Models {
		cost := costPerToken(mr.Model) * float64(tokensPerRole) / 1000.0
		totalCost += cost
	}
	return totalCost
}

// packFilePath returns the path where packs are persisted.
func packFilePath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot determine home directory: %w", err)
	}
	dir := filepath.Join(home, ".hawk")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("cannot create config directory: %w", err)
	}
	return filepath.Join(dir, "model_packs.json"), nil
}

// packFileData is the JSON structure persisted to disk.
type packFileData struct {
	Packs      map[string]*ModelPack `json:"packs"`
	ActivePack string               `json:"active_pack"`
}

// Save persists the registry to disk as JSON.
func (r *ModelPackRegistry) Save() error {
	r.mu.RLock()
	data := packFileData{
		Packs:      r.Packs,
		ActivePack: r.ActivePack,
	}
	r.mu.RUnlock()

	fp, err := packFilePath()
	if err != nil {
		return err
	}

	raw, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal model packs: %w", err)
	}

	if err := os.WriteFile(fp, raw, 0o644); err != nil {
		return fmt.Errorf("failed to write model packs file: %w", err)
	}
	return nil
}

// Load reads the registry from disk, merging with built-in packs.
func (r *ModelPackRegistry) Load() error {
	fp, err := packFilePath()
	if err != nil {
		return err
	}

	raw, err := os.ReadFile(fp)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // No file yet, use defaults.
		}
		return fmt.Errorf("failed to read model packs file: %w", err)
	}

	var data packFileData
	if err := json.Unmarshal(raw, &data); err != nil {
		return fmt.Errorf("failed to parse model packs file: %w", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	// Merge loaded packs into existing (built-in packs are preserved unless overridden).
	for name, pack := range data.Packs {
		r.Packs[name] = pack
	}
	if data.ActivePack != "" {
		if _, ok := r.Packs[data.ActivePack]; ok {
			r.ActivePack = data.ActivePack
		}
	}
	return nil
}

// Compare produces a side-by-side comparison of two packs.
func (r *ModelPackRegistry) Compare(packA, packB string) string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	a, okA := r.Packs[packA]
	b, okB := r.Packs[packB]

	if !okA {
		return fmt.Sprintf("pack %q not found", packA)
	}
	if !okB {
		return fmt.Sprintf("pack %q not found", packB)
	}

	var sb strings.Builder
	headerA := fmt.Sprintf("%-30s", packA)
	headerB := fmt.Sprintf("%-30s", packB)
	sb.WriteString(fmt.Sprintf("%-12s %s %s\n", "Role", headerA, headerB))
	sb.WriteString(strings.Repeat("─", 72))
	sb.WriteString("\n")

	roleOrder := []string{"code", "review", "chat", "summarize", "plan", "debug"}

	// Collect all roles from both packs.
	allRoles := make(map[string]bool)
	for role := range a.Models {
		allRoles[role] = true
	}
	for role := range b.Models {
		allRoles[role] = true
	}
	// Add extra roles not in the standard order.
	extras := make([]string, 0)
	for role := range allRoles {
		found := false
		for _, r := range roleOrder {
			if role == r {
				found = true
				break
			}
		}
		if !found {
			extras = append(extras, role)
		}
	}
	sort.Strings(extras)
	displayOrder := append(roleOrder, extras...)

	for _, role := range displayOrder {
		if !allRoles[role] {
			continue
		}
		colA := "(none)"
		colB := "(none)"
		if mr, ok := a.Models[role]; ok {
			colA = fmt.Sprintf("%s (%.1f)", mr.Model, mr.Temperature)
		}
		if mr, ok := b.Models[role]; ok {
			colB = fmt.Sprintf("%s (%.1f)", mr.Model, mr.Temperature)
		}
		sb.WriteString(fmt.Sprintf("%-12s %-30s %-30s\n", role, colA, colB))
	}

	sb.WriteString(strings.Repeat("─", 72))
	sb.WriteString("\n")
	sb.WriteString(fmt.Sprintf("%-12s %-30s %-30s\n", "Provider", a.DefaultProvider, b.DefaultProvider))

	costA := EstimateCost(a, 100000)
	costB := EstimateCost(b, 100000)
	sb.WriteString(fmt.Sprintf("%-12s %-30s %-30s\n", "Est. Cost", fmt.Sprintf("$%.4f/100K tok", costA), fmt.Sprintf("$%.4f/100K tok", costB)))

	return sb.String()
}
