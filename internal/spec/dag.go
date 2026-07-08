package spec

import (
	"fmt"
	"strings"

	"github.com/GrayCodeAI/hawk/internal/fsutil"
)

// ArtifactState represents the current status of an artifact in the workflow.
type ArtifactState int

const (
	ArtifactBlocked ArtifactState = iota // dependencies not met
	ArtifactReady                        // dependencies met, not yet created
	ArtifactDone                         // output file exists
)

func (s ArtifactState) String() string {
	switch s {
	case ArtifactBlocked:
		return "blocked"
	case ArtifactReady:
		return "ready"
	case ArtifactDone:
		return "done"
	default:
		return "unknown"
	}
}

// ArtifactStatus is the computed state of a single artifact.
type ArtifactStatus struct {
	ID          string        `json:"id"`
	State       ArtifactState `json:"state"`
	OutputPath  string        `json:"output_path,omitempty"`
	MissingDeps []string      `json:"missing_deps,omitempty"`
}

// Graph represents the artifact dependency DAG for a schema within a
// specific change directory.
type Graph struct {
	schema    *Schema
	changeDir string // root of the current spec directory (e.g. .hawk/specs/<slug>/)
	artifacts []Artifact
	// adjacency list: artifact ID -> IDs that depend on it
	dependents map[string][]string
	// in-degree count per artifact
	inDegree map[string]int
}

// NewGraph creates a new artifact DAG for the given schema and change directory.
func NewGraph(schema *Schema, changeDir string) *Graph {
	g := &Graph{
		schema:     schema,
		changeDir:  changeDir,
		artifacts:  schema.Artifacts,
		dependents: make(map[string][]string),
		inDegree:   make(map[string]int),
	}

	// Build adjacency and in-degree maps
	for _, a := range g.artifacts {
		if _, ok := g.dependents[a.ID]; !ok {
			g.dependents[a.ID] = nil
		}
		if _, ok := g.inDegree[a.ID]; !ok {
			g.inDegree[a.ID] = 0
		}
	}
	for _, a := range g.artifacts {
		for _, dep := range a.Requires {
			g.dependents[dep] = append(g.dependents[dep], a.ID)
			g.inDegree[a.ID]++
		}
	}

	return g
}

// Artifacts returns all artifacts in the graph.
func (g *Graph) Artifacts() []Artifact {
	result := make([]Artifact, len(g.artifacts))
	copy(result, g.artifacts)
	return result
}

// TopologicalOrder returns artifact IDs in dependency order using Kahn's
// algorithm. Returns an error if a cycle is detected.
func (g *Graph) TopologicalOrder() ([]string, error) {
	inDegree := make(map[string]int)
	for k, v := range g.inDegree {
		inDegree[k] = v
	}

	var queue []string
	for _, id := range g.schema.IDs() {
		if inDegree[id] == 0 {
			queue = append(queue, id)
		}
	}

	var order []string
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		order = append(order, id)
		for _, dep := range g.dependents[id] {
			inDegree[dep]--
			if inDegree[dep] == 0 {
				queue = append(queue, dep)
			}
		}
	}

	if len(order) != len(g.artifacts) {
		return nil, fmt.Errorf("cycle detected in artifact dependency graph")
	}
	return order, nil
}

// Status returns the computed state of a single artifact.
func (g *Graph) Status(artifactID string) ArtifactStatus {
	a := g.schema.ArtifactByID(artifactID)
	if a == nil {
		return ArtifactStatus{ID: artifactID, State: ArtifactBlocked}
	}

	outputPath := g.resolveOutput(a.Generates)
	exists := fileExists(outputPath)

	if exists {
		return ArtifactStatus{
			ID:         artifactID,
			State:      ArtifactDone,
			OutputPath: outputPath,
		}
	}

	var missing []string
	for _, depID := range a.Requires {
		depStatus := g.Status(depID)
		if depStatus.State != ArtifactDone {
			missing = append(missing, depID)
		}
	}

	if len(missing) > 0 {
		return ArtifactStatus{
			ID:          artifactID,
			State:       ArtifactBlocked,
			MissingDeps: missing,
		}
	}

	return ArtifactStatus{
		ID:         artifactID,
		State:      ArtifactReady,
		OutputPath: outputPath,
	}
}

// AllStatus returns the state of all artifacts in the graph.
func (g *Graph) AllStatus() []ArtifactStatus {
	statuses := make([]ArtifactStatus, 0, len(g.artifacts))
	for _, a := range g.artifacts {
		statuses = append(statuses, g.Status(a.ID))
	}
	return statuses
}

// NextActionable returns IDs of artifacts that are ready to be created
// (dependencies met, output doesn't exist yet).
func (g *Graph) NextActionable() []string {
	var ready []string
	for _, a := range g.artifacts {
		s := g.Status(a.ID)
		if s.State == ArtifactReady {
			ready = append(ready, a.ID)
		}
	}
	return ready
}

// IsComplete returns true when all artifacts are in the Done state.
func (g *Graph) IsComplete() bool {
	for _, a := range g.artifacts {
		if g.Status(a.ID).State != ArtifactDone {
			return false
		}
	}
	return true
}

// Progress returns (completed, total) artifact counts.
func (g *Graph) Progress() (int, int) {
	done := 0
	for _, a := range g.artifacts {
		if g.Status(a.ID).State == ArtifactDone {
			done++
		}
	}
	return done, len(g.artifacts)
}

// FormatStatus returns a human-readable status summary.
func (g *Graph) FormatStatus() string {
	order, err := g.TopologicalOrder()
	if err != nil {
		return fmt.Sprintf("error: %v", err)
	}

	var b strings.Builder
	done, total := g.Progress()
	fmt.Fprintf(&b, "Progress: %d/%d artifacts\n", done, total)

	for _, id := range order {
		s := g.Status(id)
		icon := "○"
		switch s.State {
		case ArtifactDone:
			icon = "+"
		case ArtifactReady:
			icon = "▶"
		case ArtifactBlocked:
			icon = "◉"
		}
		fmt.Fprintf(&b, "  %s %s", icon, id)
		if s.OutputPath != "" {
			fmt.Fprintf(&b, " → %s", s.OutputPath)
		}
		if len(s.MissingDeps) > 0 {
			fmt.Fprintf(&b, " (needs: %s)", strings.Join(s.MissingDeps, ", "))
		}
		b.WriteString("\n")
	}

	if g.schema.Apply != nil {
		applyReady := true
		for _, req := range g.schema.Apply.Requires {
			if g.Status(req).State != ArtifactDone {
				applyReady = false
				break
			}
		}
		if applyReady {
			b.WriteString("  ▶ apply → ready to implement\n")
		} else {
			b.WriteString("  ◉ apply → blocked (needs all artifacts done)\n")
		}
	}

	return strings.TrimRight(b.String(), "\n")
}

// resolveOutput converts a generates pattern to an absolute path within the change dir.
func (g *Graph) resolveOutput(pattern string) string {
	if g.changeDir == "" {
		return pattern
	}
	return g.changeDir + "/" + pattern
}

func fileExists(path string) bool {
	if path == "" {
		return false
	}
	return fsutil.Exists(path)
}
