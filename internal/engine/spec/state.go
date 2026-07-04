package spec

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// StageMeta is persisted to .hawk/specs/<slug>/spec.json to enable
// cross-session spec workflow recovery.
type StageMeta struct {
	Slug      string    `json:"slug"`
	Stage     string    `json:"stage"` // "specify", "plan", "tasks", "implementing", "archived"
	Schema    string    `json:"schema"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Title     string    `json:"title,omitempty"`
}

// SpecsRoot returns the .hawk/specs directory, creating it if needed.
func SpecsRoot() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("getwd: %w", err)
	}
	dir := filepath.Join(cwd, ".hawk", "specs")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("mkdir specs: %w", err)
	}
	return dir, nil
}

// WriteStageMeta persists the spec stage metadata to disk.
func WriteStageMeta(slug, stage, schema, title string) error {
	dir, err := SpecsRoot()
	if err != nil {
		return err
	}
	meta := StageMeta{
		Slug:      slug,
		Stage:     stage,
		Schema:    schema,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		Title:     title,
	}

	// If meta already exists, preserve created_at and title
	existing := LoadStageMeta(slug)
	if existing != nil {
		meta.CreatedAt = existing.CreatedAt
		if existing.Title != "" {
			meta.Title = existing.Title
		}
	}

	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal meta: %w", err)
	}
	path := filepath.Join(dir, slug, "spec.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("mkdir meta: %w", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("write meta: %w", err)
	}
	return nil
}

// LoadStageMeta reads the spec stage metadata for a slug.
func LoadStageMeta(slug string) *StageMeta {
	dir, err := SpecsRoot()
	if err != nil {
		return nil
	}
	path := filepath.Join(dir, slug, "spec.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var m StageMeta
	if err := json.Unmarshal(data, &m); err != nil {
		return nil
	}
	return &m
}

// ListSpecs returns all spec slugs with their metadata.
func ListSpecs() ([]StageMeta, error) {
	dir, err := SpecsRoot()
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("list specs: %w", err)
	}
	var specs []StageMeta
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		m := LoadStageMeta(e.Name())
		if m == nil {
			m = &StageMeta{Slug: e.Name()}
		}
		specs = append(specs, *m)
	}
	return specs, nil
}

// StageFromFiles reads which artifacts exist in a spec directory and returns
// the inferred stage string.
func StageFromFiles(slug string) string {
	dir, err := SpecsRoot()
	if err != nil {
		return ""
	}
	specDir := filepath.Join(dir, slug)

	hasSpec := fileExists(filepath.Join(specDir, "spec.md"))
	hasPlan := fileExists(filepath.Join(specDir, "plan.md"))
	hasTasks := fileExists(filepath.Join(specDir, "tasks.md"))

	switch {
	case hasTasks:
		return "tasks"
	case hasPlan:
		return "plan"
	case hasSpec:
		return "specify"
	default:
		return "none"
	}
}

// StageEnumFromString converts a stage string to SpecStage integer.
func StageEnumFromString(s string) int {
	switch s {
	case "specify":
		return 1 // SpecStageSpecify
	case "plan":
		return 2 // SpecStagePlan
	case "tasks":
		return 3 // SpecStageTasks
	case "implementing":
		return 4 // SpecStageImplementing
	default:
		return 0 // SpecStageNone
	}
}

// StringFromStageEnum converts a SpecStage integer to a stage string.
func StringFromStageEnum(stage int) string {
	switch stage {
	case 1:
		return "specify"
	case 2:
		return "plan"
	case 3:
		return "tasks"
	case 4:
		return "implementing"
	default:
		return "none"
	}
}

// StageEnumDisplayName returns a human-readable display name for a stage string.
func StageEnumDisplayName(stage string) string {
	switch stage {
	case "specify":
		return "Specify"
	case "plan":
		return "Plan"
	case "tasks":
		return "Tasks"
	case "implementing":
		return "Implementing"
	case "archived":
		return "Archived"
	default:
		return "None"
	}
}

// DeleteSpec removes a spec directory entirely.
func DeleteSpec(slug string) error {
	dir, err := SpecsRoot()
	if err != nil {
		return err
	}
	specDir := filepath.Join(dir, slug)
	if _, err := os.Stat(specDir); os.IsNotExist(err) {
		return nil
	}
	return os.RemoveAll(specDir)
}
