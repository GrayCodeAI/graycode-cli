package spec

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"time"
)

var validSlug = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)

func specDir(root, slug string) (string, error) {
	if !validSlug.MatchString(slug) || slug == "." || slug == ".." {
		return "", fmt.Errorf("invalid spec slug %q", slug)
	}
	return filepath.Join(root, slug), nil
}

// StageMeta is persisted to .graycode/specs/<slug>/spec.json to enable
// cross-session spec workflow recovery.
type StageMeta struct {
	Slug      string    `json:"slug"`
	Stage     string    `json:"stage"` // "specify", "plan", "tasks", "implementing", "archived"
	Schema    string    `json:"schema"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Title     string    `json:"title,omitempty"`
}

// SpecsRoot returns the .graycode/specs directory, creating it if needed.
func SpecsRoot() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("getwd: %w", err)
	}
	dir := filepath.Join(cwd, ".graycode", "specs")
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
	specPath, err := specDir(dir, slug)
	if err != nil {
		return err
	}
	path := filepath.Join(specPath, "spec.json")
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
	specPath, err := specDir(dir, slug)
	if err != nil {
		return nil
	}
	path := filepath.Join(specPath, "spec.json")
	data, err := os.ReadFile(path) // #nosec G304 -- path is confined by specDir's validated slug
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
	specPath, err := specDir(dir, slug)
	if err != nil {
		return ""
	}

	hasSpec := fileExists(filepath.Join(specPath, "spec.md"))
	hasPlan := fileExists(filepath.Join(specPath, "plan.md"))
	hasTasks := fileExists(filepath.Join(specPath, "tasks.md"))

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
	case "proposal":
		return 1 // SpecStageProposal
	case "specify":
		return 2 // SpecStageSpecify
	case "design":
		return 3 // SpecStageDesign
	case "plan":
		return 4 // SpecStagePlan
	case "tasks":
		return 5 // SpecStageTasks
	case "implementing":
		return 6 // SpecStageImplementing
	default:
		return 0 // SpecStageNone
	}
}

// StringFromStageEnum converts a SpecStage integer to a stage string.
func StringFromStageEnum(stage int) string {
	switch stage {
	case 1:
		return "proposal"
	case 2:
		return "specify"
	case 3:
		return "design"
	case 4:
		return "plan"
	case 5:
		return "tasks"
	case 6:
		return "implementing"
	default:
		return "none"
	}
}

// StageEnumDisplayName returns a human-readable display name for a stage string.
func StageEnumDisplayName(stage string) string {
	switch stage {
	case "proposal":
		return "Proposal"
	case "specify":
		return "Specify"
	case "design":
		return "Design"
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
	specPath, err := specDir(dir, slug)
	if err != nil {
		return err
	}
	if _, err := os.Stat(specPath); os.IsNotExist(err) {
		return nil
	}
	return os.RemoveAll(specPath)
}
