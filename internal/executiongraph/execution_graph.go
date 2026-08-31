// Package executiongraph projects Hawk-owned runtime state into the portable
// graph contract. It is deliberately read-only: the scheduler, tools, policy
// engine, verification engines, and Swift remain their own sources of truth.
package executiongraph

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	graphcontracts "github.com/GrayCodeAI/eagle/graph"
	policycontracts "github.com/GrayCodeAI/eagle/policy"
	verifycontracts "github.com/GrayCodeAI/eagle/verify"
	"github.com/GrayCodeAI/hawk/internal/session"
	"github.com/GrayCodeAI/hawk/internal/taskruntime"
	"github.com/GrayCodeAI/hawk/internal/tool"
)

const SchemaVersion = "hawk.graph/v1"

// Export is a portable, deterministic projection of Hawk execution state.
type Export struct {
	SchemaVersion string                 `json:"schema_version"`
	GeneratedAt   time.Time              `json:"generated_at"`
	Scope         graphcontracts.Scope   `json:"scope"`
	Nodes         []graphcontracts.Node  `json:"nodes"`
	Edges         []graphcontracts.Edge  `json:"edges"`
	Events        []graphcontracts.Event `json:"events"`
}

// PolicyObservation attaches a policy verdict to an optional graph subject.
type PolicyObservation struct {
	ID           string
	Subject      graphcontracts.Ref
	Verdict      policycontracts.PermissionVerdict
	ReasonSHA256 string
	OccurredAt   time.Time
}

// VerificationSummary is an already-sanitized verification aggregate loaded
// from Hawk's runtime observation journal.
type VerificationSummary struct {
	Failed       bool
	FindingCount int
	MaxSeverity  string
	TargetSHA256 string
}

// VerificationObservation attaches a neutral verification report to an
// optional task, tool call, or session graph subject.
type VerificationObservation struct {
	ID         string
	Subject    graphcontracts.Ref
	Report     *verifycontracts.Report
	Summary    *VerificationSummary
	OccurredAt time.Time
}

// ContextObservation attaches the exact metadata-only knowledge subgraph used
// for inference to a Hawk execution subject.
type ContextObservation struct {
	ID         string
	Subject    graphcontracts.Ref
	Nodes      []graphcontracts.Node
	Edges      []graphcontracts.Edge
	Events     []graphcontracts.Event
	OccurredAt time.Time
}

// QualityObservation attaches a portable metadata-only quality subgraph to a
// Hawk execution subject.
type QualityObservation struct {
	ID         string
	Subject    graphcontracts.Ref
	Nodes      []graphcontracts.Node
	Edges      []graphcontracts.Edge
	Events     []graphcontracts.Event
	OccurredAt time.Time
}

// RuntimeObservation attaches a mixed operations/policy/quality subgraph to a
// Hawk execution subject.
type RuntimeObservation struct {
	ID         string
	Subject    graphcontracts.Ref
	Nodes      []graphcontracts.Node
	Edges      []graphcontracts.Edge
	Events     []graphcontracts.Event
	OccurredAt time.Time
}

// SwiftCheckpointRef links Hawk execution state to a checkpoint exported by
// Swift. Subject defaults to the Hawk session when left empty.
type SwiftCheckpointRef struct {
	CheckpointID   string
	SwiftSessionID string
	Subject        graphcontracts.Ref
	CreatedAt      time.Time
}

// SwiftSessionRef links Hawk execution state to a session authoritatively
// resolved by Swift. Subject defaults to the Hawk session when left empty.
type SwiftSessionRef struct {
	SessionID string
	Subject   graphcontracts.Ref
	CreatedAt time.Time
}

// Input contains read-only snapshots from Hawk's existing runtime owners.
type Input struct {
	Session             *session.Session
	Tasks               []*tool.Task
	RuntimeTasks        []*taskruntime.Task
	ContextObservations []ContextObservation
	QualityObservations []QualityObservation
	RuntimeObservations []RuntimeObservation
	PolicyObservations  []PolicyObservation
	Verifications       []VerificationObservation
	SwiftSessions       []SwiftSessionRef
	SwiftCheckpoints    []SwiftCheckpointRef
	GeneratedAt         time.Time
	Scope               graphcontracts.Scope
	ProducerVersion     string
}

// Build creates and validates a deterministic execution-graph projection.
func Build(input Input) (Export, error) {
	if input.GeneratedAt.IsZero() {
		return Export{}, fmt.Errorf("execution graph: generated time is required")
	}
	input.GeneratedAt = input.GeneratedAt.UTC()

	acc := newAccumulator(input.GeneratedAt, input.Scope, input.ProducerVersion)
	sessionRef, err := acc.addSession(input.Session)
	if err != nil {
		return Export{}, err
	}
	if err := acc.addStructuredTasks(input.Tasks, sessionRef); err != nil {
		return Export{}, err
	}
	if err := acc.addRuntimeTasks(input.RuntimeTasks, sessionRef); err != nil {
		return Export{}, err
	}
	if err := acc.addContextObservations(input.ContextObservations); err != nil {
		return Export{}, err
	}
	if err := acc.addQualityObservations(input.QualityObservations); err != nil {
		return Export{}, err
	}
	if err := acc.addRuntimeObservations(input.RuntimeObservations); err != nil {
		return Export{}, err
	}
	if err := acc.addPolicyObservations(input.PolicyObservations); err != nil {
		return Export{}, err
	}
	if err := acc.addVerifications(input.Verifications); err != nil {
		return Export{}, err
	}
	if err := acc.addSwiftSessions(input.SwiftSessions, sessionRef); err != nil {
		return Export{}, err
	}
	if err := acc.addSwiftCheckpoints(input.SwiftCheckpoints, sessionRef); err != nil {
		return Export{}, err
	}

	sort.Slice(acc.export.Nodes, func(i, j int) bool { return acc.export.Nodes[i].ID < acc.export.Nodes[j].ID })
	sort.Slice(acc.export.Edges, func(i, j int) bool { return acc.export.Edges[i].ID < acc.export.Edges[j].ID })
	sort.Slice(acc.export.Events, func(i, j int) bool { return acc.export.Events[i].ID < acc.export.Events[j].ID })
	return acc.export, nil
}

type accumulator struct {
	export          Export
	generatedAt     time.Time
	producerVersion string
	nodes           map[string]graphcontracts.NodeKind
	edges           map[string]struct{}
	events          map[string]struct{}
}

func newAccumulator(generatedAt time.Time, scope graphcontracts.Scope, producerVersion string) *accumulator {
	return &accumulator{
		export: Export{
			SchemaVersion: SchemaVersion,
			GeneratedAt:   generatedAt,
			Scope:         scope,
			Nodes:         make([]graphcontracts.Node, 0),
			Edges:         make([]graphcontracts.Edge, 0),
			Events:        make([]graphcontracts.Event, 0),
		},
		generatedAt:     generatedAt,
		producerVersion: strings.TrimSpace(producerVersion),
		nodes:           make(map[string]graphcontracts.NodeKind),
		edges:           make(map[string]struct{}),
		events:          make(map[string]struct{}),
	}
}

func (a *accumulator) addSession(saved *session.Session) (graphcontracts.Ref, error) {
	if saved == nil {
		return graphcontracts.Ref{}, nil
	}
	sessionID := strings.TrimSpace(saved.ID)
	ref := graphcontracts.Ref{Kind: graphcontracts.NodeExecution, ID: sessionNodeID(sessionID)}
	createdAt := graphTime(saved.CreatedAt, a.generatedAt)
	attributes := baseAttributes("hawk_session")
	attributes["message_count"] = strconv.Itoa(len(saved.Messages))
	addAttribute(attributes, "model", saved.Model)
	addAttribute(attributes, "provider", saved.Provider)
	addAttribute(attributes, "agent", saved.Agent)
	addAttribute(attributes, "name_sha256", digest(saved.Name))

	node := graphcontracts.Node{
		ID:          ref.ID,
		Kind:        ref.Kind,
		Scope:       a.export.Scope,
		CreatedAt:   createdAt,
		EffectiveAt: graphTime(saved.UpdatedAt, createdAt),
		Provenance:  a.hawkProvenance(sessionID, "hawk://session/"+sessionID),
		Attributes:  attributes,
	}
	if err := a.addNode(node); err != nil {
		return graphcontracts.Ref{}, err
	}
	if err := a.addEvent(graphcontracts.Event{
		ID:             "hawk/event/session/" + sessionID + "/created",
		Type:           graphcontracts.EventCreated,
		Subject:        ref,
		Scope:          a.export.Scope,
		OccurredAt:     createdAt,
		CorrelationID:  sessionID,
		IdempotencyKey: "hawk/session/" + sessionID + "/created",
		Provenance:     node.Provenance,
	}); err != nil {
		return graphcontracts.Ref{}, err
	}
	if err := a.addSessionMessages(saved, ref); err != nil {
		return graphcontracts.Ref{}, err
	}
	return ref, nil
}

func (a *accumulator) addSessionMessages(saved *session.Session, sessionRef graphcontracts.Ref) error {
	results := make(map[string]session.ToolResult)
	for _, message := range saved.Messages {
		for _, result := range message.ToolResults {
			results[result.ToolUseID] = result
		}
	}

	currentTask := graphcontracts.Ref{}
	taskNumber := 0
	for messageIndex, message := range saved.Messages {
		if message.Role == "user" && len(message.ToolResults) == 0 && strings.TrimSpace(message.Content) != "" {
			taskNumber++
			taskID := fmt.Sprintf("%s/%d", saved.ID, taskNumber)
			currentTask = graphcontracts.Ref{
				Kind: graphcontracts.NodeExecution,
				ID:   "hawk/task-request/" + taskID,
			}
			node := graphcontracts.Node{
				ID:        currentTask.ID,
				Kind:      currentTask.Kind,
				Scope:     a.export.Scope,
				CreatedAt: a.generatedAt,
				Provenance: a.hawkProvenance(
					fmt.Sprintf("%s:message:%d", saved.ID, messageIndex),
					fmt.Sprintf("hawk://session/%s/message/%d", saved.ID, messageIndex),
				),
				Attributes: map[string]string{
					"entity_type":         "task_request",
					"data_classification": "metadata_only",
					"content_sha256":      digest(message.Content),
					"message_index":       strconv.Itoa(messageIndex),
					"temporal_precision":  "projection_time",
				},
			}
			if err := a.addNode(node); err != nil {
				return err
			}
			if err := a.addContainsEdge(sessionRef, currentTask, node.CreatedAt, node.Provenance); err != nil {
				return err
			}
			if err := a.addObservedEvent(currentTask, node.CreatedAt, node.Provenance, "task-request/"+taskID); err != nil {
				return err
			}
		}

		for toolIndex, call := range message.ToolUse {
			callID := strings.TrimSpace(call.ID)
			if callID == "" {
				callID = fmt.Sprintf("message-%d-tool-%d", messageIndex, toolIndex)
			}
			ref := graphcontracts.Ref{
				Kind: graphcontracts.NodeExecution,
				ID:   toolCallNodeID(saved.ID, callID),
			}
			status := "pending"
			isError := false
			if result, ok := results[call.ID]; ok {
				isError = result.IsError
				status = "completed"
				if isError {
					status = "failed"
				}
			}
			node := graphcontracts.Node{
				ID:        ref.ID,
				Kind:      ref.Kind,
				Scope:     a.export.Scope,
				CreatedAt: a.generatedAt,
				Provenance: a.hawkProvenance(
					callID,
					fmt.Sprintf("hawk://session/%s/tool-call/%s", saved.ID, callID),
				),
				Attributes: map[string]string{
					"entity_type":         "tool_call",
					"data_classification": "metadata_only",
					"tool_name":           call.Name,
					"argument_count":      strconv.Itoa(len(call.Arguments)),
					"status":              status,
					"is_error":            strconv.FormatBool(isError),
					"message_index":       strconv.Itoa(messageIndex),
					"temporal_precision":  "projection_time",
				},
			}
			if err := a.addNode(node); err != nil {
				return err
			}
			parent := currentTask
			if parent.ID == "" {
				parent = sessionRef
			}
			if err := a.addContainsEdge(parent, ref, node.CreatedAt, node.Provenance); err != nil {
				return err
			}
			if err := a.addObservedEvent(ref, node.CreatedAt, node.Provenance, "tool-call/"+saved.ID+"/"+callID); err != nil {
				return err
			}
		}
	}
	return nil
}

func (a *accumulator) addStructuredTasks(tasks []*tool.Task, sessionRef graphcontracts.Ref) error {
	for _, task := range tasks {
		if task == nil {
			continue
		}
		taskID := strings.TrimSpace(task.ID)
		ref := graphcontracts.Ref{Kind: graphcontracts.NodeExecution, ID: structuredTaskNodeID(taskID)}
		createdAt := graphTime(task.CreatedAt, a.generatedAt)
		attributes := baseAttributes("task")
		attributes["status"] = string(task.Status)
		addAttribute(attributes, "subject_sha256", digest(task.Subject))
		addAttribute(attributes, "description_sha256", digest(task.Description))
		addAttribute(attributes, "owner", task.Owner)
		node := graphcontracts.Node{
			ID:          ref.ID,
			Kind:        ref.Kind,
			Scope:       a.export.Scope,
			CreatedAt:   createdAt,
			EffectiveAt: graphTime(task.UpdatedAt, createdAt),
			Provenance:  a.hawkProvenance(taskID, "hawk://task/"+taskID),
			Attributes:  attributes,
		}
		if err := a.addNode(node); err != nil {
			return err
		}
		if err := a.addEvent(graphcontracts.Event{
			ID:             "hawk/event/task/" + taskID + "/created",
			Type:           graphcontracts.EventCreated,
			Subject:        ref,
			Scope:          a.export.Scope,
			OccurredAt:     createdAt,
			CorrelationID:  taskID,
			IdempotencyKey: "hawk/task/" + taskID + "/created",
			Provenance:     node.Provenance,
		}); err != nil {
			return err
		}
		if !task.UpdatedAt.IsZero() && task.UpdatedAt.After(task.CreatedAt) {
			if err := a.addEvent(graphcontracts.Event{
				ID:             "hawk/event/task/" + taskID + "/transitioned",
				Type:           graphcontracts.EventTransitioned,
				Subject:        ref,
				Scope:          a.export.Scope,
				OccurredAt:     task.UpdatedAt.UTC(),
				CorrelationID:  taskID,
				CausationID:    "hawk/event/task/" + taskID + "/created",
				IdempotencyKey: "hawk/task/" + taskID + "/transitioned/" + string(task.Status),
				Provenance:     node.Provenance,
			}); err != nil {
				return err
			}
		}
	}
	for _, task := range tasks {
		if task == nil {
			continue
		}
		from := graphcontracts.Ref{Kind: graphcontracts.NodeExecution, ID: structuredTaskNodeID(task.ID)}
		if task.ParentID != "" {
			if err := a.addContainsEdge(
				graphcontracts.Ref{Kind: graphcontracts.NodeExecution, ID: structuredTaskNodeID(task.ParentID)},
				from,
				graphTime(task.CreatedAt, a.generatedAt),
				a.hawkProvenance(task.ID, "hawk://task/"+task.ID),
			); err != nil {
				return err
			}
		} else if sessionRef.ID != "" {
			if err := a.addContainsEdge(
				sessionRef,
				from,
				graphTime(task.CreatedAt, a.generatedAt),
				a.hawkProvenance(task.ID, "hawk://task/"+task.ID),
			); err != nil {
				return err
			}
		}
		for _, dependency := range task.Dependencies {
			if dependency.Type == "parent-child" && task.ParentID == dependency.TargetID {
				continue
			}
			to := graphcontracts.Ref{Kind: graphcontracts.NodeExecution, ID: structuredTaskNodeID(dependency.TargetID)}
			kind := graphcontracts.EdgeReferences
			switch dependency.Type {
			case "blocks":
				kind = graphcontracts.EdgeDependsOn
			case "parent-child":
				kind = graphcontracts.EdgeContains
				from, to = to, from
			}
			if err := a.addEdge(graphcontracts.Edge{
				ID:        "hawk/edge/task/" + task.ID + "/" + dependency.Type + "/" + dependency.TargetID,
				Kind:      kind,
				From:      from,
				To:        to,
				Scope:     a.export.Scope,
				CreatedAt: graphTime(task.CreatedAt, a.generatedAt),
				Provenance: a.hawkProvenance(
					task.ID,
					"hawk://task/"+task.ID,
				),
				Attributes: map[string]string{"dependency_type": dependency.Type},
			}); err != nil {
				return err
			}
		}
	}
	return nil
}

func (a *accumulator) addRuntimeTasks(tasks []*taskruntime.Task, sessionRef graphcontracts.Ref) error {
	for _, task := range tasks {
		if task == nil {
			continue
		}
		taskID := strings.TrimSpace(task.ID)
		ref := graphcontracts.Ref{Kind: graphcontracts.NodeExecution, ID: "hawk/runtime-task/" + taskID}
		createdAt := graphTime(task.StartedAt, a.generatedAt)
		attributes := baseAttributes("runtime_task")
		attributes["kind"] = string(task.Kind)
		attributes["status"] = string(task.Status)
		attributes["output_present"] = strconv.FormatBool(task.Output != "")
		attributes["error_present"] = strconv.FormatBool(task.Error != "")
		addAttribute(attributes, "label_sha256", digest(task.Prompt))
		node := graphcontracts.Node{
			ID:          ref.ID,
			Kind:        ref.Kind,
			Scope:       a.export.Scope,
			CreatedAt:   createdAt,
			EffectiveAt: graphTime(task.DoneAt, createdAt),
			Provenance:  a.hawkProvenance(taskID, "hawk://runtime-task/"+taskID),
			Attributes:  attributes,
		}
		if err := a.addNode(node); err != nil {
			return err
		}
		if sessionRef.ID != "" {
			if err := a.addContainsEdge(sessionRef, ref, createdAt, node.Provenance); err != nil {
				return err
			}
		}
		if err := a.addEvent(graphcontracts.Event{
			ID:             "hawk/event/runtime-task/" + taskID + "/created",
			Type:           graphcontracts.EventCreated,
			Subject:        ref,
			Scope:          a.export.Scope,
			OccurredAt:     createdAt,
			CorrelationID:  taskID,
			IdempotencyKey: "hawk/runtime-task/" + taskID + "/created",
			Provenance:     node.Provenance,
		}); err != nil {
			return err
		}
		if !task.DoneAt.IsZero() {
			if err := a.addEvent(graphcontracts.Event{
				ID:             "hawk/event/runtime-task/" + taskID + "/transitioned",
				Type:           graphcontracts.EventTransitioned,
				Subject:        ref,
				Scope:          a.export.Scope,
				OccurredAt:     task.DoneAt.UTC(),
				CorrelationID:  taskID,
				CausationID:    "hawk/event/runtime-task/" + taskID + "/created",
				IdempotencyKey: "hawk/runtime-task/" + taskID + "/transitioned/" + string(task.Status),
				Provenance:     node.Provenance,
			}); err != nil {
				return err
			}
		}
	}
	return nil
}

func (a *accumulator) addContextObservations(observations []ContextObservation) error {
	for index, observation := range observations {
		occurredAt := graphTime(observation.OccurredAt, a.generatedAt)
		subject := observation.Subject
		if subject.ID != "" {
			if err := subject.Validate(); err != nil {
				return fmt.Errorf("execution graph: context subject: %w", err)
			}
			if _, exists := a.nodes[subject.ID]; !exists {
				return fmt.Errorf("execution graph: context subject references unknown node %q", subject.ID)
			}
		}

		for _, imported := range observation.Nodes {
			if imported.Kind != graphcontracts.NodeKnowledge {
				return fmt.Errorf("execution graph: context node %q must have knowledge kind", imported.ID)
			}
			imported.Scope = a.export.Scope
			if err := a.addNode(imported); err != nil {
				return err
			}
		}
		for _, imported := range observation.Edges {
			imported.Scope = a.export.Scope
			if err := a.addEdge(imported); err != nil {
				return err
			}
		}
		for _, imported := range observation.Events {
			imported.Scope = a.export.Scope
			if err := a.addEvent(imported); err != nil {
				return err
			}
		}
		if subject.ID == "" {
			continue
		}
		contextID := strings.TrimSpace(observation.ID)
		if contextID == "" {
			contextID = strconv.Itoa(index + 1)
		}
		for _, imported := range observation.Nodes {
			target := graphcontracts.Ref{Kind: imported.Kind, ID: imported.ID}
			edge := graphcontracts.Edge{
				ID:        "hawk/edge/context/" + contextID + "/" + digest(subject.ID+"\x00"+target.ID),
				Kind:      graphcontracts.EdgeReferences,
				From:      subject,
				To:        target,
				Scope:     a.export.Scope,
				CreatedAt: occurredAt,
				Provenance: a.hawkProvenance(
					contextID,
					"hawk://context-observation/"+contextID,
				),
				Attributes: baseAttributes("context_selection"),
			}
			if err := a.addEdge(edge); err != nil {
				return err
			}
		}
	}
	return nil
}

func (a *accumulator) addQualityObservations(observations []QualityObservation) error {
	for index, observation := range observations {
		occurredAt := graphTime(observation.OccurredAt, a.generatedAt)
		subject := observation.Subject
		if subject.ID != "" {
			if err := subject.Validate(); err != nil {
				return fmt.Errorf("execution graph: quality subject: %w", err)
			}
			if _, exists := a.nodes[subject.ID]; !exists {
				return fmt.Errorf("execution graph: quality subject references unknown node %q", subject.ID)
			}
		}
		for _, imported := range observation.Nodes {
			if imported.Kind != graphcontracts.NodeQuality {
				return fmt.Errorf("execution graph: quality node %q must have quality kind", imported.ID)
			}
			imported.Scope = a.export.Scope
			if err := a.addNode(imported); err != nil {
				return err
			}
		}
		for _, imported := range observation.Edges {
			imported.Scope = a.export.Scope
			if err := a.addEdge(imported); err != nil {
				return err
			}
		}
		for _, imported := range observation.Events {
			imported.Scope = a.export.Scope
			if err := a.addEvent(imported); err != nil {
				return err
			}
		}
		if subject.ID == "" {
			continue
		}
		qualityID := strings.TrimSpace(observation.ID)
		if qualityID == "" {
			qualityID = strconv.Itoa(index + 1)
		}
		for _, imported := range observation.Nodes {
			if imported.Attributes["entity"] != "report" {
				continue
			}
			target := graphcontracts.Ref{Kind: imported.Kind, ID: imported.ID}
			edge := graphcontracts.Edge{
				ID:        "hawk/edge/quality/" + qualityID + "/" + digest(subject.ID+"\x00"+target.ID),
				Kind:      graphcontracts.EdgeValidatedBy,
				From:      subject,
				To:        target,
				Scope:     a.export.Scope,
				CreatedAt: occurredAt,
				Provenance: a.hawkProvenance(
					qualityID,
					"hawk://quality-observation/"+qualityID,
				),
				Attributes: baseAttributes("quality_observation"),
			}
			if err := a.addEdge(edge); err != nil {
				return err
			}
		}
	}
	return nil
}

func (a *accumulator) addRuntimeObservations(observations []RuntimeObservation) error {
	for index, observation := range observations {
		subject := observation.Subject
		if subject.ID != "" {
			if err := subject.Validate(); err != nil {
				return fmt.Errorf("execution graph: runtime subject: %w", err)
			}
			if _, exists := a.nodes[subject.ID]; !exists {
				return fmt.Errorf("execution graph: runtime subject references unknown node %q", subject.ID)
			}
		}
		for _, imported := range observation.Nodes {
			switch imported.Kind {
			case graphcontracts.NodeOperations, graphcontracts.NodePolicy, graphcontracts.NodeQuality:
			default:
				return fmt.Errorf("execution graph: runtime node %q has unsupported kind %q", imported.ID, imported.Kind)
			}
			imported.Scope = a.export.Scope
			if err := a.addNode(imported); err != nil {
				return err
			}
		}
		for _, imported := range observation.Edges {
			imported.Scope = a.export.Scope
			if err := a.addEdge(imported); err != nil {
				return err
			}
		}
		for _, imported := range observation.Events {
			imported.Scope = a.export.Scope
			if err := a.addEvent(imported); err != nil {
				return err
			}
		}
		if subject.ID == "" {
			continue
		}
		id := strings.TrimSpace(observation.ID)
		if id == "" {
			id = strconv.Itoa(index + 1)
		}
		occurredAt := graphTime(observation.OccurredAt, a.generatedAt)
		for _, imported := range observation.Nodes {
			target := graphcontracts.Ref{Kind: imported.Kind, ID: imported.ID}
			var edgeKind graphcontracts.EdgeKind
			switch imported.Kind {
			case graphcontracts.NodeOperations:
				edgeKind = graphcontracts.EdgeProduced
			case graphcontracts.NodePolicy:
				edgeKind = graphcontracts.EdgeGovernedBy
			case graphcontracts.NodeQuality:
				edgeKind = graphcontracts.EdgeValidatedBy
			}
			edge := graphcontracts.Edge{
				ID:        "hawk/edge/runtime/" + id + "/" + digest(subject.ID+"\x00"+target.ID),
				Kind:      edgeKind,
				From:      subject,
				To:        target,
				Scope:     a.export.Scope,
				CreatedAt: occurredAt,
				Provenance: a.hawkProvenance(
					id,
					"hawk://runtime-observation/"+id,
				),
				Attributes: baseAttributes("runtime_observation"),
			}
			if err := a.addEdge(edge); err != nil {
				return err
			}
		}
	}
	return nil
}

func (a *accumulator) addPolicyObservations(observations []PolicyObservation) error {
	for index, observation := range observations {
		id := strings.TrimSpace(observation.ID)
		if id == "" {
			id = strconv.Itoa(index + 1)
		}
		ref := graphcontracts.Ref{Kind: graphcontracts.NodePolicy, ID: "hawk/policy/" + id}
		occurredAt := graphTime(observation.OccurredAt, a.generatedAt)
		attributes := baseAttributes("policy_verdict")
		attributes["allowed"] = strconv.FormatBool(observation.Verdict.Allowed)
		attributes["risk"] = observation.Verdict.Risk.String()
		attributes["confidence"] = strconv.FormatFloat(observation.Verdict.Confidence, 'f', -1, 64)
		addAttribute(attributes, "rule", observation.Verdict.Rule)
		addAttribute(attributes, "source", observation.Verdict.Source)
		reasonSHA256 := strings.TrimSpace(observation.ReasonSHA256)
		if reasonSHA256 == "" {
			reasonSHA256 = digest(observation.Verdict.Reason)
		} else {
			reasonSHA256 = sanitizedSHA256(reasonSHA256)
		}
		addAttribute(attributes, "reason_sha256", reasonSHA256)
		node := graphcontracts.Node{
			ID:         ref.ID,
			Kind:       ref.Kind,
			Scope:      a.export.Scope,
			CreatedAt:  occurredAt,
			Provenance: a.hawkProvenance(id, "hawk://policy/"+id),
			Attributes: attributes,
		}
		if err := a.addNode(node); err != nil {
			return err
		}
		if observation.Subject.ID != "" {
			if err := observation.Subject.Validate(); err != nil {
				return fmt.Errorf("execution graph: policy subject: %w", err)
			}
			if err := a.addEdge(graphcontracts.Edge{
				ID:         "hawk/edge/" + observation.Subject.ID + "/governed-by/" + ref.ID,
				Kind:       graphcontracts.EdgeGovernedBy,
				From:       observation.Subject,
				To:         ref,
				Scope:      a.export.Scope,
				CreatedAt:  occurredAt,
				Provenance: node.Provenance,
			}); err != nil {
				return err
			}
		}
		if err := a.addObservedEvent(ref, occurredAt, node.Provenance, "policy/"+id); err != nil {
			return err
		}
	}
	return nil
}

func (a *accumulator) addVerifications(observations []VerificationObservation) error {
	for index, observation := range observations {
		if observation.Report == nil && observation.Summary == nil {
			continue
		}
		id := strings.TrimSpace(observation.ID)
		if id == "" {
			id = strconv.Itoa(index + 1)
		}
		ref := graphcontracts.Ref{Kind: graphcontracts.NodeQuality, ID: "hawk/verification/" + id}
		occurredAt := graphTime(observation.OccurredAt, a.generatedAt)
		attributes := baseAttributes("verification")
		if observation.Summary != nil {
			attributes["failed"] = strconv.FormatBool(observation.Summary.Failed)
			attributes["finding_count"] = strconv.Itoa(observation.Summary.FindingCount)
			attributes["max_severity"] = observation.Summary.MaxSeverity
			addAttribute(attributes, "target_sha256", sanitizedSHA256(observation.Summary.TargetSHA256))
		} else {
			attributes["failed"] = strconv.FormatBool(observation.Report.Failed())
			attributes["finding_count"] = strconv.Itoa(len(observation.Report.Findings))
			attributes["max_severity"] = observation.Report.MaxSeverity().String()
			addAttribute(attributes, "target_sha256", digest(observation.Report.Target))
		}
		node := graphcontracts.Node{
			ID:         ref.ID,
			Kind:       ref.Kind,
			Scope:      a.export.Scope,
			CreatedAt:  occurredAt,
			Provenance: a.hawkProvenance(id, "hawk://verification/"+id),
			Attributes: attributes,
		}
		if err := a.addNode(node); err != nil {
			return err
		}
		if observation.Subject.ID != "" {
			if err := observation.Subject.Validate(); err != nil {
				return fmt.Errorf("execution graph: verification subject: %w", err)
			}
			if err := a.addEdge(graphcontracts.Edge{
				ID:         "hawk/edge/" + observation.Subject.ID + "/validated-by/" + ref.ID,
				Kind:       graphcontracts.EdgeValidatedBy,
				From:       observation.Subject,
				To:         ref,
				Scope:      a.export.Scope,
				CreatedAt:  occurredAt,
				Provenance: node.Provenance,
			}); err != nil {
				return err
			}
		}
		if err := a.addObservedEvent(ref, occurredAt, node.Provenance, "verification/"+id); err != nil {
			return err
		}
	}
	return nil
}

func (a *accumulator) addSwiftSessions(sessions []SwiftSessionRef, sessionRef graphcontracts.Ref) error {
	for _, traceSession := range sessions {
		swiftSessionID := strings.TrimSpace(traceSession.SessionID)
		ref := graphcontracts.Ref{Kind: graphcontracts.NodeExecution, ID: "swift/session/" + swiftSessionID}
		createdAt := graphTime(traceSession.CreatedAt, a.generatedAt)
		node := graphcontracts.Node{
			ID:        ref.ID,
			Kind:      ref.Kind,
			Scope:     a.export.Scope,
			CreatedAt: createdAt,
			Provenance: graphcontracts.Provenance{
				Producer: "swift",
				SourceID: swiftSessionID,
				Evidence: []graphcontracts.ArtifactRef{{URI: "swift://session/" + swiftSessionID}},
			},
			Attributes: baseAttributes("swift_session"),
		}
		if err := a.addNode(node); err != nil {
			return err
		}
		subject := traceSession.Subject
		if subject.ID == "" {
			subject = sessionRef
		}
		if subject.ID == "" {
			continue
		}
		if err := subject.Validate(); err != nil {
			return fmt.Errorf("execution graph: swift session subject: %w", err)
		}
		if err := a.addEdge(graphcontracts.Edge{
			ID:         "hawk/edge/" + subject.ID + "/references/" + ref.ID,
			Kind:       graphcontracts.EdgeReferences,
			From:       subject,
			To:         ref,
			Scope:      a.export.Scope,
			CreatedAt:  createdAt,
			Provenance: a.hawkProvenance(swiftSessionID, "swift://session/"+swiftSessionID),
		}); err != nil {
			return err
		}
	}
	return nil
}

func (a *accumulator) addSwiftCheckpoints(checkpoints []SwiftCheckpointRef, sessionRef graphcontracts.Ref) error {
	for _, checkpoint := range checkpoints {
		checkpointID := strings.TrimSpace(checkpoint.CheckpointID)
		ref := graphcontracts.Ref{Kind: graphcontracts.NodeExecution, ID: "swift/checkpoint/" + checkpointID}
		createdAt := graphTime(checkpoint.CreatedAt, a.generatedAt)
		node := graphcontracts.Node{
			ID:        ref.ID,
			Kind:      ref.Kind,
			Scope:     a.export.Scope,
			CreatedAt: createdAt,
			Provenance: graphcontracts.Provenance{
				Producer: "swift",
				SourceID: checkpointID,
				Evidence: []graphcontracts.ArtifactRef{{URI: "swift://checkpoint/" + checkpointID}},
			},
			Attributes: baseAttributes("checkpoint"),
		}
		if err := a.addNode(node); err != nil {
			return err
		}
		subject := checkpoint.Subject
		if subject.ID == "" {
			subject = sessionRef
		}
		if subject.ID == "" {
			continue
		}
		if err := subject.Validate(); err != nil {
			return fmt.Errorf("execution graph: swift checkpoint subject: %w", err)
		}
		if err := a.addEdge(graphcontracts.Edge{
			ID:         "hawk/edge/" + subject.ID + "/references/" + ref.ID,
			Kind:       graphcontracts.EdgeReferences,
			From:       subject,
			To:         ref,
			Scope:      a.export.Scope,
			CreatedAt:  createdAt,
			Provenance: a.hawkProvenance(checkpointID, "swift://checkpoint/"+checkpointID),
		}); err != nil {
			return err
		}
		swiftSessionID := strings.TrimSpace(checkpoint.SwiftSessionID)
		if swiftSessionID != "" {
			if err := a.addSwiftSessions([]SwiftSessionRef{{
				SessionID: swiftSessionID,
				CreatedAt: createdAt,
			}}, sessionRef); err != nil {
				return err
			}
			traceSessionRef := graphcontracts.Ref{
				Kind: graphcontracts.NodeExecution,
				ID:   "swift/session/" + swiftSessionID,
			}
			if err := a.addEdge(graphcontracts.Edge{
				ID:         "swift/edge/" + swiftSessionID + "/produced/" + checkpointID,
				Kind:       graphcontracts.EdgeProduced,
				From:       traceSessionRef,
				To:         ref,
				Scope:      a.export.Scope,
				CreatedAt:  createdAt,
				Provenance: node.Provenance,
			}); err != nil {
				return err
			}
		}
	}
	return nil
}

func (a *accumulator) addContainsEdge(
	from, to graphcontracts.Ref,
	createdAt time.Time,
	provenance graphcontracts.Provenance,
) error {
	if from.ID == "" || to.ID == "" {
		return nil
	}
	return a.addEdge(graphcontracts.Edge{
		ID:         "hawk/edge/" + from.ID + "/contains/" + to.ID,
		Kind:       graphcontracts.EdgeContains,
		From:       from,
		To:         to,
		Scope:      a.export.Scope,
		CreatedAt:  createdAt,
		Provenance: provenance,
	})
}

func (a *accumulator) addObservedEvent(
	subject graphcontracts.Ref,
	occurredAt time.Time,
	provenance graphcontracts.Provenance,
	suffix string,
) error {
	return a.addEvent(graphcontracts.Event{
		ID:             "hawk/event/" + suffix + "/observed",
		Type:           graphcontracts.EventObserved,
		Subject:        subject,
		Scope:          a.export.Scope,
		OccurredAt:     occurredAt,
		CorrelationID:  subject.ID,
		IdempotencyKey: "hawk/" + suffix + "/observed",
		Provenance:     provenance,
	})
}

func (a *accumulator) addNode(node graphcontracts.Node) error {
	if err := node.Validate(); err != nil {
		return fmt.Errorf("execution graph: node %q: %w", node.ID, err)
	}
	if kind, exists := a.nodes[node.ID]; exists {
		if kind != node.Kind {
			return fmt.Errorf("execution graph: node %q has conflicting kinds %q and %q", node.ID, kind, node.Kind)
		}
		return nil
	}
	a.nodes[node.ID] = node.Kind
	a.export.Nodes = append(a.export.Nodes, node)
	return nil
}

func (a *accumulator) addEdge(edge graphcontracts.Edge) error {
	if err := edge.Validate(); err != nil {
		return fmt.Errorf("execution graph: edge %q: %w", edge.ID, err)
	}
	if _, exists := a.nodes[edge.From.ID]; !exists {
		return fmt.Errorf("execution graph: edge %q has unknown source node %q", edge.ID, edge.From.ID)
	}
	if _, exists := a.nodes[edge.To.ID]; !exists {
		return fmt.Errorf("execution graph: edge %q has unknown target node %q", edge.ID, edge.To.ID)
	}
	if _, exists := a.edges[edge.ID]; exists {
		return nil
	}
	a.edges[edge.ID] = struct{}{}
	a.export.Edges = append(a.export.Edges, edge)
	return nil
}

func (a *accumulator) addEvent(event graphcontracts.Event) error {
	if err := event.Validate(); err != nil {
		return fmt.Errorf("execution graph: event %q: %w", event.ID, err)
	}
	if _, exists := a.events[event.ID]; exists {
		return nil
	}
	a.events[event.ID] = struct{}{}
	a.export.Events = append(a.export.Events, event)
	return nil
}

func (a *accumulator) hawkProvenance(sourceID, evidenceURI string) graphcontracts.Provenance {
	provenance := graphcontracts.Provenance{
		Producer: "hawk",
		Version:  a.producerVersion,
		SourceID: strings.TrimSpace(sourceID),
	}
	if strings.TrimSpace(evidenceURI) != "" {
		provenance.Evidence = []graphcontracts.ArtifactRef{{URI: evidenceURI}}
	}
	return provenance
}

func baseAttributes(entityType string) map[string]string {
	return map[string]string{
		"entity_type":         entityType,
		"data_classification": "metadata_only",
	}
}

func addAttribute(attributes map[string]string, key, value string) {
	if value = strings.TrimSpace(value); value != "" {
		attributes[key] = value
	}
}

func graphTime(value, fallback time.Time) time.Time {
	if value.IsZero() {
		return fallback
	}
	return value.UTC()
}

func digest(value string) string {
	if strings.TrimSpace(value) == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func sanitizedSHA256(value string) string {
	value = strings.TrimSpace(value)
	if len(value) == sha256.Size*2 {
		decoded, err := hex.DecodeString(value)
		if err == nil && hex.EncodeToString(decoded) == value {
			return value
		}
	}
	return digest(value)
}

func sessionNodeID(sessionID string) string {
	return "hawk/session/" + sessionID
}

func structuredTaskNodeID(taskID string) string {
	return "hawk/task/" + taskID
}

func toolCallNodeID(sessionID, toolUseID string) string {
	return "hawk/tool-call/" + sessionID + "/" + toolUseID
}
