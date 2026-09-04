package mission

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	graphcontracts "github.com/GrayCodeAI/graycode-cli/internal/contracts/graph"
	"github.com/GrayCodeAI/graycode-cli/internal/executiongraph"
)

const portableGraphFilename = "mission-graph.json"

func (m *Mission) persistPortableGraph() error {
	if m == nil || strings.TrimSpace(m.Dir) == "" {
		return nil
	}
	export, err := m.buildPortableGraph(time.Now().UTC())
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(export, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal mission graph: %w", err)
	}
	return os.WriteFile(filepath.Join(m.Dir, portableGraphFilename), data, 0o600)
}

func (m *Mission) buildPortableGraph(generatedAt time.Time) (executiongraph.Export, error) {
	if m == nil {
		return executiongraph.Export{}, fmt.Errorf("mission graph: mission is required")
	}
	if generatedAt.IsZero() {
		generatedAt = time.Now().UTC()
	}

	scope := graphcontracts.Scope{RepositoryID: portableGraphRepositoryID(m.Config.RepoDir)}
	export := executiongraph.Export{
		SchemaVersion: executiongraph.SchemaVersion,
		GeneratedAt:   generatedAt.UTC(),
		Scope:         scope,
		Nodes:         make([]graphcontracts.Node, 0, 1+len(m.Features)+len(m.WaveJoins)),
		Edges:         make([]graphcontracts.Edge, 0, len(m.Features)+len(m.WaveJoins)*2),
		Events:        make([]graphcontracts.Event, 0, 2+len(m.Features)+len(m.WaveJoins)),
	}

	missionRef := graphcontracts.Ref{Kind: graphcontracts.NodeExecution, ID: portableMissionNodeID(m.ID)}
	missionCreated := portableGraphTime(m.StartedAt, generatedAt)
	missionProvenance := portableProvenance(m.ID, "graycode://mission/"+m.ID)
	missionNode := graphcontracts.Node{
		ID:          missionRef.ID,
		Kind:        missionRef.Kind,
		Scope:       scope,
		CreatedAt:   missionCreated,
		EffectiveAt: portableGraphTime(m.CompletedAt, missionCreated),
		Provenance:  missionProvenance,
		Attributes: map[string]string{
			"entity_type":         "mission",
			"data_classification": "metadata_only",
			"status":              string(m.Status),
			"feature_count":       strconv.Itoa(len(m.Features)),
			"wave_count":          strconv.Itoa(len(m.WaveJoins)),
			"prompt_sha256":       portableDigest(m.Prompt),
		},
	}
	if err := portableAddNode(&export, missionNode); err != nil {
		return executiongraph.Export{}, err
	}
	if err := portableAddEvent(&export, graphcontracts.Event{
		ID:             "graycode/event/mission/" + m.ID + "/created",
		Type:           graphcontracts.EventCreated,
		Subject:        missionRef,
		Scope:          scope,
		OccurredAt:     missionCreated,
		CorrelationID:  m.ID,
		IdempotencyKey: "graycode/mission/" + m.ID + "/created",
		Provenance:     missionProvenance,
	}); err != nil {
		return executiongraph.Export{}, err
	}
	if !m.CompletedAt.IsZero() {
		if err := portableAddEvent(&export, graphcontracts.Event{
			ID:             "graycode/event/mission/" + m.ID + "/transitioned",
			Type:           graphcontracts.EventTransitioned,
			Subject:        missionRef,
			Scope:          scope,
			OccurredAt:     m.CompletedAt.UTC(),
			CorrelationID:  m.ID,
			CausationID:    "graycode/event/mission/" + m.ID + "/created",
			IdempotencyKey: "graycode/mission/" + m.ID + "/transitioned/" + string(m.Status),
			Provenance:     missionProvenance,
		}); err != nil {
			return executiongraph.Export{}, err
		}
	}

	for _, feature := range m.Features {
		featureID := strings.TrimSpace(feature.ID)
		if featureID == "" {
			continue
		}
		ref := graphcontracts.Ref{Kind: graphcontracts.NodeExecution, ID: portableFeatureNodeID(m.ID, featureID)}
		createdAt := portableGraphTime(feature.StartedAt, missionCreated)
		if createdAt.Equal(missionCreated) && !feature.CompletedAt.IsZero() {
			createdAt = portableGraphTime(feature.CompletedAt, missionCreated)
		}
		provenance := portableProvenance(featureID, "graycode://mission/"+m.ID+"/feature/"+featureID)
		node := graphcontracts.Node{
			ID:          ref.ID,
			Kind:        ref.Kind,
			Scope:       scope,
			CreatedAt:   createdAt,
			EffectiveAt: portableGraphTime(feature.CompletedAt, createdAt),
			Provenance:  provenance,
			Attributes: map[string]string{
				"entity_type":              "mission_feature",
				"data_classification":      "metadata_only",
				"status":                   string(feature.Status),
				"description_sha256":       portableDigest(feature.Description),
				"expected_behavior_sha256": portableDigest(feature.ExpectedBehavior),
				"branch_sha256":            portableDigest(feature.Branch),
				"tests_passed":             strconv.FormatBool(feature.Handoff != nil && feature.Handoff.TestsPassed),
			},
		}
		if err := portableAddNode(&export, node); err != nil {
			return executiongraph.Export{}, err
		}
		if err := portableAddEdge(&export, graphcontracts.Edge{
			ID:         "graycode/edge/mission/" + m.ID + "/contains/" + featureID,
			Kind:       graphcontracts.EdgeContains,
			From:       missionRef,
			To:         ref,
			Scope:      scope,
			CreatedAt:  createdAt,
			Provenance: provenance,
			Attributes: map[string]string{"entity_type": "mission_membership"},
		}); err != nil {
			return executiongraph.Export{}, err
		}
		if err := portableAddEvent(&export, graphcontracts.Event{
			ID:             "graycode/event/mission-feature/" + m.ID + "/" + featureID + "/observed",
			Type:           graphcontracts.EventObserved,
			Subject:        ref,
			Scope:          scope,
			OccurredAt:     portableGraphTime(feature.CompletedAt, createdAt),
			CorrelationID:  m.ID,
			IdempotencyKey: "graycode/mission-feature/" + m.ID + "/" + featureID + "/observed/" + string(feature.Status),
			Provenance:     provenance,
		}); err != nil {
			return executiongraph.Export{}, err
		}
	}

	for _, join := range m.WaveJoins {
		waveID := strconv.Itoa(join.Wave)
		ref := graphcontracts.Ref{Kind: graphcontracts.NodeOperations, ID: portableWaveNodeID(m.ID, join.Wave)}
		createdAt := portableGraphTime(join.StartedAt, missionCreated)
		completedAt := portableGraphTime(join.CompletedAt, createdAt)
		provenance := portableProvenance(waveID, "graycode://mission/"+m.ID+"/wave/"+waveID)
		node := graphcontracts.Node{
			ID:          ref.ID,
			Kind:        ref.Kind,
			Scope:       scope,
			CreatedAt:   createdAt,
			EffectiveAt: completedAt,
			Provenance:  provenance,
			Attributes: map[string]string{
				"entity_type":         "wave_join",
				"data_classification": "metadata_only",
				"wave":                waveID,
				"feature_count":       strconv.Itoa(len(join.FeatureIDs)),
				"completed_count":     strconv.Itoa(len(join.CompletedIDs)),
				"failed_count":        strconv.Itoa(len(join.FailedIDs)),
				"blocked_count":       strconv.Itoa(len(join.BlockedIDs)),
				"status":              portableWaveStatus(join),
				"summary_sha256":      portableDigest(join.Summary),
			},
		}
		if err := portableAddNode(&export, node); err != nil {
			return executiongraph.Export{}, err
		}
		if err := portableAddEdge(&export, graphcontracts.Edge{
			ID:         "graycode/edge/mission/" + m.ID + "/produced-wave/" + waveID,
			Kind:       graphcontracts.EdgeProduced,
			From:       missionRef,
			To:         ref,
			Scope:      scope,
			CreatedAt:  createdAt,
			Provenance: provenance,
			Attributes: map[string]string{"entity_type": "wave_join_produced"},
		}); err != nil {
			return executiongraph.Export{}, err
		}
		for _, featureID := range join.FeatureIDs {
			target := graphcontracts.Ref{
				Kind: graphcontracts.NodeExecution,
				ID:   portableFeatureNodeID(m.ID, featureID),
			}
			if err := portableAddEdge(&export, graphcontracts.Edge{
				ID:         "graycode/edge/mission-wave/" + m.ID + "/" + waveID + "/contains/" + featureID,
				Kind:       graphcontracts.EdgeContains,
				From:       ref,
				To:         target,
				Scope:      scope,
				CreatedAt:  createdAt,
				Provenance: provenance,
				Attributes: map[string]string{
					"entity_type": "wave_membership",
				},
			}); err != nil {
				return executiongraph.Export{}, err
			}
		}
		if err := portableAddEvent(&export, graphcontracts.Event{
			ID:             "graycode/event/mission-wave/" + m.ID + "/" + waveID + "/observed",
			Type:           graphcontracts.EventObserved,
			Subject:        ref,
			Scope:          scope,
			OccurredAt:     completedAt,
			CorrelationID:  m.ID,
			IdempotencyKey: "graycode/mission-wave/" + m.ID + "/" + waveID + "/observed",
			Provenance:     provenance,
		}); err != nil {
			return executiongraph.Export{}, err
		}
	}

	sort.Slice(export.Nodes, func(i, j int) bool { return export.Nodes[i].ID < export.Nodes[j].ID })
	sort.Slice(export.Edges, func(i, j int) bool { return export.Edges[i].ID < export.Edges[j].ID })
	sort.Slice(export.Events, func(i, j int) bool { return export.Events[i].ID < export.Events[j].ID })
	return export, nil
}

func portableMissionNodeID(missionID string) string {
	return "graycode/mission/" + strings.TrimSpace(missionID)
}

func portableFeatureNodeID(missionID, featureID string) string {
	return "graycode/mission-feature/" + strings.TrimSpace(missionID) + "/" + strings.TrimSpace(featureID)
}

func portableWaveNodeID(missionID string, wave int) string {
	return "graycode/mission-wave/" + strings.TrimSpace(missionID) + "/" + strconv.Itoa(wave)
}

func portableWaveStatus(join WaveJoin) string {
	if len(join.FailedIDs) > 0 {
		return "failed"
	}
	if len(join.BlockedIDs) > 0 {
		return "partial"
	}
	return "completed"
}

func portableGraphRepositoryID(repoDir string) string {
	repoDir = strings.TrimSpace(repoDir)
	if repoDir == "" {
		return ""
	}
	return filepath.Base(filepath.Clean(repoDir))
}

func portableProvenance(sourceID, evidenceURI string) graphcontracts.Provenance {
	provenance := graphcontracts.Provenance{
		Producer: "graycode",
		SourceID: strings.TrimSpace(sourceID),
	}
	if uri := strings.TrimSpace(evidenceURI); uri != "" {
		provenance.Evidence = []graphcontracts.ArtifactRef{{URI: uri}}
	}
	return provenance
}

func portableAddNode(export *executiongraph.Export, node graphcontracts.Node) error {
	if err := node.Validate(); err != nil {
		return fmt.Errorf("mission graph node %q: %w", node.ID, err)
	}
	export.Nodes = append(export.Nodes, node)
	return nil
}

func portableAddEdge(export *executiongraph.Export, edge graphcontracts.Edge) error {
	if err := edge.Validate(); err != nil {
		return fmt.Errorf("mission graph edge %q: %w", edge.ID, err)
	}
	export.Edges = append(export.Edges, edge)
	return nil
}

func portableAddEvent(export *executiongraph.Export, event graphcontracts.Event) error {
	if err := event.Validate(); err != nil {
		return fmt.Errorf("mission graph event %q: %w", event.ID, err)
	}
	export.Events = append(export.Events, event)
	return nil
}

func portableGraphTime(value, fallback time.Time) time.Time {
	if value.IsZero() {
		return fallback.UTC()
	}
	return value.UTC()
}

func portableDigest(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}
