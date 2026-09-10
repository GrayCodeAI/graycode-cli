package mission

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Reflexion captures what went wrong in a failed worker attempt and what to
// try next, following the Reflexion verbal-reinforcement pattern (Shinn et al.,
// NeurIPS 2023, arXiv 2303.11366). It is persisted per feature so a later
// retry — or a human reviewing the mission — can see why a feature failed and
// what to attempt next, without re-deriving it from the transcript.
type Reflexion struct {
	FeatureID     string    `json:"feature_id"`
	Attempt       int       `json:"attempt"`
	WhatWentWrong string    `json:"what_went_wrong"`
	WhatToTryNext string    `json:"what_to_try_next"`
	TestsPassed   bool      `json:"tests_passed"`
	CommitID      string    `json:"commit_id,omitempty"`
	RecordedAt    time.Time `json:"recorded_at"`
}

// ReflexionStore persists and loads per-feature reflexions under a mission dir.
// Each feature's reflexions are append-only JSONL, oldest first.
type ReflexionStore struct {
	dir string
}

// NewReflexionStore creates a store rooted at missionDir/reflexions.
func NewReflexionStore(missionDir string) *ReflexionStore {
	return &ReflexionStore{dir: filepath.Join(missionDir, "reflexions")}
}

// reflexionPath returns the file for a feature's reflexions.
func (s *ReflexionStore) reflexionPath(featureID string) string {
	return filepath.Join(s.dir, featureID+".jsonl")
}

// Reflect builds a structured reflexion from a worker handoff. The
// what-to-try-next guidance is deterministic and derives from whether tests
// passed and whether a commit was produced, so it is testable and useful even
// before an LLM generates richer prose.
func Reflect(feature *Feature, handoff *Handoff, attempt int) Reflexion {
	whatWentWrong := "worker completed without a passing test result"
	whatToTryNext := "inspect the failing tests, fix the root cause, and re-run the suite"

	testsPassed := handoff != nil && handoff.TestsPassed
	commitID := ""
	if handoff != nil {
		commitID = handoff.CommitID
		if handoff.Summary != "" {
			whatWentWrong = handoff.Summary
		}
		if !testsPassed {
			whatToTryNext = "run the test suite, identify the failing tests, fix the root cause, and re-run until green"
		} else if commitID == "" {
			whatToTryNext = "the changes are not committed; stage and commit them with a descriptive message"
		}
	}

	return Reflexion{
		FeatureID:     feature.ID,
		Attempt:       attempt,
		WhatWentWrong: truncate(whatWentWrong, 500),
		WhatToTryNext: whatToTryNext,
		TestsPassed:   testsPassed,
		CommitID:      commitID,
		RecordedAt:    time.Now(),
	}
}

// Record persists a reflexion for a feature (append-only JSONL).
func (s *ReflexionStore) Record(r Reflexion) error {
	if s == nil || s.dir == "" {
		return nil
	}
	if err := os.MkdirAll(s.dir, 0o755); err != nil {
		return err
	}
	data, err := json.Marshal(r)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(s.reflexionPath(r.FeatureID), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	if _, err := f.Write(append(data, '\n')); err != nil {
		return err
	}
	return nil
}

// Load returns all reflexions recorded for a feature, oldest first. A missing
// file yields an empty slice, not an error.
func (s *ReflexionStore) Load(featureID string) ([]Reflexion, error) {
	if s == nil || s.dir == "" {
		return nil, nil
	}
	data, err := os.ReadFile(s.reflexionPath(featureID))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []Reflexion
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		if line == "" {
			continue
		}
		var r Reflexion
		if err := json.Unmarshal([]byte(line), &r); err != nil {
			continue // skip a corrupt line rather than failing the whole load
		}
		out = append(out, r)
	}
	return out, nil
}
