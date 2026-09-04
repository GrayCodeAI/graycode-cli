package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"
	"unicode"

	swiftcli "github.com/GrayCodeAI/swift/cli"
)

const (
	swiftCorrelationSchemaVersion = "swift.correlation/v1"
	maxSwiftCorrelationOutput     = 1 << 20
	maxSwiftSessionIDLength       = 512
)

var errSwiftCorrelationOutputLimit = errors.New("swift correlation output exceeds local limit")

type swiftCorrelationResolver interface {
	Resolve(context.Context, string) (swiftCorrelation, error)
}

type swiftCLICorrelationResolver struct{}

type swiftCorrelation struct {
	SchemaVersion            string                  `json:"schema_version"`
	GraycodeSessionID        string                  `json:"graycode_session_id"`
	CheckpointLookupComplete bool                    `json:"checkpoint_lookup_complete"`
	Matches                  []swiftCorrelationMatch `json:"matches"`
}

type swiftCorrelationMatch struct {
	SwiftSessionID string     `json:"swift_session_id"`
	CheckpointIDs  []string   `json:"checkpoint_ids"`
	StartedAt      time.Time  `json:"started_at"`
	EndedAt        *time.Time `json:"ended_at,omitempty"`
	Phase          string     `json:"phase,omitempty"`
}

func (swiftCLICorrelationResolver) Resolve(
	ctx context.Context,
	graycodeSessionID string,
) (swiftCorrelation, error) {
	graycodeSessionID = strings.TrimSpace(graycodeSessionID)
	if graycodeSessionID == "" {
		return swiftCorrelation{}, fmt.Errorf("resolve Swift correlation: Graycode session ID is required")
	}

	var stdout, stderr cappedBuffer
	stdout.max = maxSwiftCorrelationOutput
	stderr.max = maxSwiftCorrelationOutput
	root := swiftcli.NewRootCmd()
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetArgs([]string{"graph", "correlation", "--graycode-session", graycodeSessionID})
	// Internal composition must not emit telemetry or launch an asynchronous
	// version check after the read-only lookup completes.
	root.PersistentPostRun = nil
	root.PersistentPostRunE = nil
	if err := root.ExecuteContext(ctx); err != nil {
		return swiftCorrelation{}, fmt.Errorf("resolve Swift correlation through CLI: %w", err)
	}

	return decodeSwiftCorrelation(stdout.Bytes(), graycodeSessionID)
}

func decodeSwiftCorrelation(payload []byte, graycodeSessionID string) (swiftCorrelation, error) {
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	var correlation swiftCorrelation
	if err := decoder.Decode(&correlation); err != nil {
		return swiftCorrelation{}, fmt.Errorf("decode Swift correlation: %w", err)
	}
	if err := requireJSONEOF(decoder); err != nil {
		return swiftCorrelation{}, err
	}
	if err := normalizeSwiftCorrelation(&correlation, graycodeSessionID); err != nil {
		return swiftCorrelation{}, err
	}
	return correlation, nil
}

func normalizeSwiftCorrelation(correlation *swiftCorrelation, graycodeSessionID string) error {
	if correlation == nil {
		return fmt.Errorf("validate Swift correlation: response is nil")
	}
	if correlation.SchemaVersion != swiftCorrelationSchemaVersion {
		return fmt.Errorf(
			"validate Swift correlation: schema %q is not supported",
			correlation.SchemaVersion,
		)
	}
	if correlation.GraycodeSessionID != graycodeSessionID {
		return fmt.Errorf("validate Swift correlation: Graycode session identity mismatch")
	}

	seenSessions := make(map[string]struct{}, len(correlation.Matches))
	for i := range correlation.Matches {
		match := &correlation.Matches[i]
		match.SwiftSessionID = strings.TrimSpace(match.SwiftSessionID)
		if err := validateSwiftSessionID(match.SwiftSessionID); err != nil {
			return err
		}
		if _, exists := seenSessions[match.SwiftSessionID]; exists {
			return fmt.Errorf(
				"validate Swift correlation: duplicate Swift session %q",
				match.SwiftSessionID,
			)
		}
		seenSessions[match.SwiftSessionID] = struct{}{}

		if !correlation.CheckpointLookupComplete {
			match.CheckpointIDs = []string{}
			match.StartedAt = match.StartedAt.UTC()
			continue
		}
		seenCheckpoints := make(map[string]struct{}, len(match.CheckpointIDs))
		checkpointIDs := make([]string, 0, len(match.CheckpointIDs))
		for _, checkpointID := range match.CheckpointIDs {
			checkpointID = strings.TrimSpace(checkpointID)
			if err := validateSwiftCheckpointID(checkpointID); err != nil {
				return fmt.Errorf("validate Swift correlation: %w", err)
			}
			if _, exists := seenCheckpoints[checkpointID]; exists {
				continue
			}
			seenCheckpoints[checkpointID] = struct{}{}
			checkpointIDs = append(checkpointIDs, checkpointID)
		}
		sort.Strings(checkpointIDs)
		match.CheckpointIDs = checkpointIDs
		match.StartedAt = match.StartedAt.UTC()
	}
	sort.Slice(correlation.Matches, func(i, j int) bool {
		return correlation.Matches[i].SwiftSessionID < correlation.Matches[j].SwiftSessionID
	})
	return nil
}

func validateSwiftSessionID(sessionID string) error {
	if sessionID == "" {
		return fmt.Errorf("validate Swift correlation: Swift session ID is empty")
	}
	if len(sessionID) > maxSwiftSessionIDLength {
		return fmt.Errorf("validate Swift correlation: Swift session ID exceeds local limit")
	}
	if strings.ContainsAny(sessionID, `/\`) {
		return fmt.Errorf("validate Swift correlation: Swift session ID contains a path separator")
	}
	for _, value := range sessionID {
		if unicode.IsControl(value) {
			return fmt.Errorf("validate Swift correlation: Swift session ID contains a control character")
		}
	}
	return nil
}

func requireJSONEOF(decoder *json.Decoder) error {
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return fmt.Errorf("decode Swift correlation: unexpected trailing JSON")
		}
		return fmt.Errorf("decode Swift correlation trailing data: %w", err)
	}
	return nil
}

type cappedBuffer struct {
	bytes.Buffer
	max int
}

func (b *cappedBuffer) Write(value []byte) (int, error) {
	remaining := b.max - b.Len()
	if remaining <= 0 {
		return 0, errSwiftCorrelationOutputLimit
	}
	if len(value) > remaining {
		written, _ := b.Buffer.Write(value[:remaining])
		return written, errSwiftCorrelationOutputLimit
	}
	return b.Buffer.Write(value)
}
