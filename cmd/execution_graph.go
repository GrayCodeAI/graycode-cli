package cmd

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	graphcontracts "github.com/GrayCodeAI/hawk-core-contracts/graph"
	policycontracts "github.com/GrayCodeAI/hawk-core-contracts/policy"
	"github.com/GrayCodeAI/hawk/internal/executiongraph"
	"github.com/GrayCodeAI/hawk/internal/graphjournal"
	"github.com/GrayCodeAI/hawk/internal/session"
	"github.com/GrayCodeAI/hawk/internal/taskruntime"
	"github.com/GrayCodeAI/hawk/internal/tool"
	"github.com/spf13/cobra"
)

func newExecutionGraphCmd() *cobra.Command {
	graphCmd := &cobra.Command{
		Use:   "graph",
		Short: "Inspect Hawk's portable execution graph",
		Long: `Project Hawk-owned sessions, task requests, structured tasks, runtime tasks,
tool calls, policy observations, verification results, and explicit Trace
checkpoint links into the shared graph contract.

This command is read-only. Existing runtime components remain the source of
truth for scheduling, tools, policy, verification, persistence, and tracing.`,
		Args: cobra.NoArgs,
	}

	var repositoryID string
	var traceCheckpointIDs []string
	var missionDir string
	exportCmd := &cobra.Command{
		Use:   "export [session-id]",
		Short: "Export a Hawk session or mission as graph JSON",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var export executiongraph.Export
			var err error
			if strings.TrimSpace(missionDir) != "" {
				if len(args) != 0 {
					return fmt.Errorf("session-id cannot be used with --mission-dir")
				}
				export, err = loadMissionGraphExport(missionDir)
			} else {
				export, err = buildExecutionGraphExport(
					args,
					repositoryID,
					traceCheckpointIDs,
					time.Now().UTC(),
				)
			}
			if err != nil {
				return err
			}

			encoder := json.NewEncoder(cmd.OutOrStdout())
			encoder.SetIndent("", "  ")
			if err := encoder.Encode(export); err != nil {
				return fmt.Errorf("encode execution graph: %w", err)
			}
			return nil
		},
	}
	exportCmd.Flags().StringVar(
		&repositoryID,
		"repository",
		"",
		"Repository scope override (defaults to the saved session directory name)",
	)
	exportCmd.Flags().StringVar(
		&missionDir,
		"mission-dir",
		"",
		"Read mission-graph.json from this mission directory instead of a session",
	)
	exportCmd.Flags().StringArrayVar(
		&traceCheckpointIDs,
		"trace-checkpoint",
		nil,
		"Link a Trace checkpoint ID; may be repeated",
	)
	graphCmd.AddCommand(exportCmd)
	return graphCmd
}

func buildExecutionGraphExport(
	args []string,
	repositoryID string,
	traceCheckpointIDs []string,
	now time.Time,
) (executiongraph.Export, error) {
	return buildExecutionGraphExportWithTrace(
		args,
		repositoryID,
		traceCheckpointIDs,
		now,
		traceCLICorrelationResolver{},
	)
}

func buildExecutionGraphExportWithTrace(
	args []string,
	repositoryID string,
	traceCheckpointIDs []string,
	now time.Time,
	resolver traceCorrelationResolver,
) (executiongraph.Export, error) {
	saved, err := loadExecutionGraphSession(args)
	if err != nil {
		return executiongraph.Export{}, err
	}
	if now.IsZero() {
		now = saved.UpdatedAt.UTC()
		if now.IsZero() {
			now = saved.CreatedAt.UTC()
		}
		if now.IsZero() {
			return executiongraph.Export{}, fmt.Errorf(
				"build execution graph: saved session has no stable snapshot time",
			)
		}
	}

	traceRefs := make([]executiongraph.TraceCheckpointRef, 0, len(traceCheckpointIDs))
	for _, checkpointID := range traceCheckpointIDs {
		checkpointID = strings.TrimSpace(checkpointID)
		if validationErr := validateTraceCheckpointID(checkpointID); validationErr != nil {
			return executiongraph.Export{}, validationErr
		}
		traceRefs = append(traceRefs, executiongraph.TraceCheckpointRef{
			CheckpointID: checkpointID,
			CreatedAt:    now,
		})
	}
	traceSessions := make([]executiongraph.TraceSessionRef, 0)
	if resolver != nil {
		correlation, correlationErr := resolver.Resolve(context.Background(), saved.ID)
		if correlationErr == nil {
			resolvedSessions, resolvedCheckpoints := traceReferencesFromCorrelation(correlation, now)
			traceSessions = append(traceSessions, resolvedSessions...)
			traceRefs = append(traceRefs, resolvedCheckpoints...)
		}
	}

	scope := graphcontracts.Scope{
		RepositoryID: executionGraphRepositoryID(repositoryID, saved.CWD),
	}
	policyObservations, verificationObservations, contextObservations, qualityObservations, runtimeObservations, err := loadRuntimeGraphObservations(saved)
	if err != nil {
		return executiongraph.Export{}, err
	}
	export, err := executiongraph.Build(executiongraph.Input{
		Session:             saved,
		Tasks:               tool.GetTaskStore().List(),
		RuntimeTasks:        taskruntime.Default.List(),
		ContextObservations: contextObservations,
		QualityObservations: qualityObservations,
		RuntimeObservations: runtimeObservations,
		PolicyObservations:  policyObservations,
		Verifications:       verificationObservations,
		TraceSessions:       traceSessions,
		TraceCheckpoints:    traceRefs,
		GeneratedAt:         now,
		Scope:               scope,
		ProducerVersion:     version,
	})
	if err != nil {
		return executiongraph.Export{}, fmt.Errorf("build execution graph: %w", err)
	}
	return export, nil
}

func traceReferencesFromCorrelation(
	correlation traceCorrelation,
	fallback time.Time,
) ([]executiongraph.TraceSessionRef, []executiongraph.TraceCheckpointRef) {
	sessions := make([]executiongraph.TraceSessionRef, 0, len(correlation.Matches))
	checkpoints := make([]executiongraph.TraceCheckpointRef, 0)
	for _, match := range correlation.Matches {
		sessions = append(sessions, executiongraph.TraceSessionRef{
			SessionID: match.TraceSessionID,
			CreatedAt: graphCorrelationTime(match.StartedAt, fallback),
		})
		for _, checkpointID := range match.CheckpointIDs {
			checkpoints = append(checkpoints, executiongraph.TraceCheckpointRef{
				CheckpointID:   checkpointID,
				TraceSessionID: match.TraceSessionID,
				CreatedAt:      fallback,
			})
		}
	}
	return sessions, checkpoints
}

func graphCorrelationTime(value, fallback time.Time) time.Time {
	if value.IsZero() {
		return fallback
	}
	return value.UTC()
}

func loadExecutionGraphSession(args []string) (*session.Session, error) {
	var (
		saved *session.Session
		err   error
	)
	if len(args) == 1 {
		saved, err = session.Load(strings.TrimSpace(args[0]))
	} else {
		saved, err = session.LoadLatest()
	}
	if err != nil {
		return nil, fmt.Errorf("load session for execution graph: %w", err)
	}
	return saved, nil
}

func loadMissionGraphExport(dir string) (executiongraph.Export, error) {
	dir = strings.TrimSpace(dir)
	if dir == "" {
		return executiongraph.Export{}, fmt.Errorf("mission directory is required")
	}
	path := filepath.Join(dir, "mission-graph.json")
	info, err := os.Stat(path)
	if err != nil {
		return executiongraph.Export{}, fmt.Errorf("stat mission graph %q: %w", path, err)
	}
	if info.IsDir() {
		return executiongraph.Export{}, fmt.Errorf("mission graph %q is a directory", path)
	}
	if info.Size() > 1<<20 {
		return executiongraph.Export{}, fmt.Errorf("mission graph is %d bytes; maximum is 1 MiB", info.Size())
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return executiongraph.Export{}, fmt.Errorf("read mission graph %q: %w", path, err)
	}
	var export executiongraph.Export
	if err := json.Unmarshal(data, &export); err != nil {
		return executiongraph.Export{}, fmt.Errorf("decode mission graph: %w", err)
	}
	if err := validatePortableGraphExport(export); err != nil {
		return executiongraph.Export{}, fmt.Errorf("validate mission graph: %w", err)
	}
	return export, nil
}

func validatePortableGraphExport(export executiongraph.Export) error {
	if export.SchemaVersion != executiongraph.SchemaVersion {
		return fmt.Errorf("unsupported schema version %q", export.SchemaVersion)
	}
	if export.GeneratedAt.IsZero() {
		return fmt.Errorf("generated_at is required")
	}
	nodes := make(map[graphcontracts.Ref]struct{}, len(export.Nodes))
	for _, node := range export.Nodes {
		if err := node.Validate(); err != nil {
			return fmt.Errorf("node %q: %w", node.ID, err)
		}
		nodes[graphcontracts.Ref{Kind: node.Kind, ID: node.ID}] = struct{}{}
	}
	for _, edge := range export.Edges {
		if err := edge.Validate(); err != nil {
			return fmt.Errorf("edge %q: %w", edge.ID, err)
		}
		if _, ok := nodes[edge.From]; !ok {
			return fmt.Errorf("edge %q references missing source %s/%s", edge.ID, edge.From.Kind, edge.From.ID)
		}
		if _, ok := nodes[edge.To]; !ok {
			return fmt.Errorf("edge %q references missing target %s/%s", edge.ID, edge.To.Kind, edge.To.ID)
		}
	}
	for _, event := range export.Events {
		if err := event.Validate(); err != nil {
			return fmt.Errorf("event %q: %w", event.ID, err)
		}
		if _, ok := nodes[event.Subject]; !ok {
			return fmt.Errorf("event %q references missing subject %s/%s", event.ID, event.Subject.Kind, event.Subject.ID)
		}
	}
	return nil
}

func loadRuntimeGraphObservations(
	saved *session.Session,
) (
	[]executiongraph.PolicyObservation,
	[]executiongraph.VerificationObservation,
	[]executiongraph.ContextObservation,
	[]executiongraph.QualityObservation,
	[]executiongraph.RuntimeObservation,
	error,
) {
	if saved == nil {
		return nil, nil, nil, nil, nil, nil
	}
	entries, err := graphjournal.Load(saved.ID)
	if err != nil {
		return nil, nil, nil, nil, nil, fmt.Errorf("load runtime graph observations: %w", err)
	}
	toolCallIDs := make(map[string]struct{})
	for _, message := range saved.Messages {
		for _, call := range message.ToolUse {
			toolCallIDs[call.ID] = struct{}{}
		}
	}
	policies := make([]executiongraph.PolicyObservation, 0)
	verifications := make([]executiongraph.VerificationObservation, 0)
	contexts := make([]executiongraph.ContextObservation, 0)
	qualities := make([]executiongraph.QualityObservation, 0)
	runtimes := make([]executiongraph.RuntimeObservation, 0)
	for _, entry := range entries {
		subject := graphcontracts.Ref{
			Kind: graphcontracts.NodeExecution,
			ID:   "hawk/session/" + saved.ID,
		}
		if _, ok := toolCallIDs[entry.ToolCallID]; ok && entry.ToolCallID != "" {
			subject = graphcontracts.Ref{
				Kind: graphcontracts.NodeExecution,
				ID:   "hawk/tool-call/" + saved.ID + "/" + entry.ToolCallID,
			}
		}
		if entry.Policy != nil {
			policies = append(policies, executiongraph.PolicyObservation{
				ID:           entry.ID,
				Subject:      subject,
				Verdict:      policyVerdictFromJournal(entry.Policy),
				ReasonSHA256: entry.Policy.ReasonSHA256,
				OccurredAt:   entry.OccurredAt,
			})
		}
		if entry.Verification != nil {
			verifications = append(verifications, executiongraph.VerificationObservation{
				ID:      entry.ID,
				Subject: subject,
				Summary: &executiongraph.VerificationSummary{
					Failed:       entry.Verification.Failed,
					FindingCount: entry.Verification.FindingCount,
					MaxSeverity:  entry.Verification.MaxSeverity,
					TargetSHA256: entry.Verification.TargetSHA256,
				},
				OccurredAt: entry.OccurredAt,
			})
		}
		if entry.Context != nil {
			contexts = append(contexts, executiongraph.ContextObservation{
				ID:         entry.ID,
				Subject:    subject,
				Nodes:      entry.Context.Nodes,
				Edges:      entry.Context.Edges,
				Events:     entry.Context.Events,
				OccurredAt: entry.OccurredAt,
			})
		}
		if entry.Quality != nil {
			qualities = append(qualities, executiongraph.QualityObservation{
				ID:         entry.ID,
				Subject:    subject,
				Nodes:      entry.Quality.Nodes,
				Edges:      entry.Quality.Edges,
				Events:     entry.Quality.Events,
				OccurredAt: entry.OccurredAt,
			})
		}
		if entry.Runtime != nil {
			runtimes = append(runtimes, executiongraph.RuntimeObservation{
				ID:         entry.ID,
				Subject:    subject,
				Nodes:      entry.Runtime.Nodes,
				Edges:      entry.Runtime.Edges,
				Events:     entry.Runtime.Events,
				OccurredAt: entry.OccurredAt,
			})
		}
	}
	return policies, verifications, contexts, qualities, runtimes, nil
}

func policyVerdictFromJournal(summary *graphjournal.PolicySummary) policycontracts.PermissionVerdict {
	if summary == nil {
		return policycontracts.PermissionVerdict{}
	}
	return policycontracts.PermissionVerdict{
		Allowed:    summary.Allowed,
		Risk:       summary.Risk,
		Confidence: summary.Confidence,
		Rule:       summary.Rule,
		Source:     summary.Source,
	}
}

func executionGraphRepositoryID(override, sessionCWD string) string {
	if override = strings.TrimSpace(override); override != "" {
		return override
	}
	path := strings.TrimSpace(sessionCWD)
	if path == "" {
		path, _ = os.Getwd()
	}
	if path == "" {
		return ""
	}
	return filepath.Base(filepath.Clean(path))
}

func validateTraceCheckpointID(checkpointID string) error {
	if len(checkpointID) != 12 {
		return fmt.Errorf("invalid Trace checkpoint ID %q: must be 12 lowercase hex characters", checkpointID)
	}
	decoded, err := hex.DecodeString(checkpointID)
	if err != nil || hex.EncodeToString(decoded) != checkpointID {
		return fmt.Errorf("invalid Trace checkpoint ID %q: must be 12 lowercase hex characters", checkpointID)
	}
	return nil
}

func init() {
	rootCmd.AddCommand(newExecutionGraphCmd())
}
