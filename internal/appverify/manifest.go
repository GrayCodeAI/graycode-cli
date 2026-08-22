package appverify

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/GrayCodeAI/hawk/internal/safewrite"
)

// ManifestPath returns the location of the persisted verify environment
// manifest for a project: <root>/.hawk/verify/environment.json.
func ManifestPath(root string) string {
	return filepath.Join(root, ".hawk", "verify", "environment.json")
}

// Manifest is the on-disk contract between recipe detection and execution.
// Once written, it is the highest-priority source of truth for verification:
// deterministic detection only runs when no manifest exists yet.
type Manifest struct {
	Recipe Recipe `json:"recipe"`
}

// LoadManifest reads the manifest from disk. os.ErrNotExist is returned when
// no manifest has been persisted, so callers can fall back to Detect.
func LoadManifest(root string) (Recipe, error) {
	raw, err := os.ReadFile(ManifestPath(root)) // #nosec G304 -- path built from caller-provided root
	if err != nil {
		return Recipe{}, err
	}
	var m Manifest
	if err := json.Unmarshal(raw, &m); err != nil {
		return Recipe{}, fmt.Errorf("appverify: parse manifest: %w", err)
	}
	// Re-validate through Normalize so a hand-edited or agent-written manifest
	// cannot smuggle unsafe values.
	canonical, err := json.Marshal(m.Recipe)
	if err != nil {
		return Recipe{}, err
	}
	return Normalize(canonical)
}

// SaveManifest persists the recipe as the project's verify manifest. Existing
// manifests are overwritten atomically.
func SaveManifest(root string, r Recipe) error {
	if r.Ecosystem == "" {
		return fmt.Errorf("appverify: refusing to save recipe without ecosystem")
	}
	data, err := json.MarshalIndent(Manifest{Recipe: r}, "", "  ")
	if err != nil {
		return fmt.Errorf("appverify: encode manifest: %w", err)
	}
	path := ManifestPath(root)
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return fmt.Errorf("appverify: create manifest dir: %w", err)
	}
	if err := safewrite.WriteFile(path, append(data, '\n')); err != nil {
		return fmt.Errorf("appverify: write manifest: %w", err)
	}
	return nil
}

// LoadOrDetect returns the persisted manifest recipe when present, otherwise
// runs deterministic detection and persists the result so subsequent runs are
// reproducible. The second return value reports whether the manifest already
// existed (false means it was just created from detection).
func LoadOrDetect(root string) (Recipe, bool, error) {
	if r, err := LoadManifest(root); err == nil {
		return r, true, nil
	} else if !os.IsNotExist(err) {
		// A corrupt manifest must not silently shadow detection; surface it.
		return Recipe{}, false, fmt.Errorf("appverify: existing manifest invalid: %w", err)
	}
	r := Detect(root)
	if err := SaveManifest(root, r); err != nil {
		// Detection still works without persistence; report the recipe anyway.
		return r, false, nil
	}
	return r, false, nil
}
