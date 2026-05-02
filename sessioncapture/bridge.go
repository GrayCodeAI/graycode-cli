package sessioncapture

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

// Bridge wraps the trace CLI for Git-native session capture.
type Bridge struct {
	bin   string
	ready bool
}

// Status holds the parsed output of `trace status --json`.
type Status struct {
	Enabled   bool   `json:"enabled"`
	SessionID string `json:"session_id,omitempty"`
	Agent     string `json:"agent,omitempty"`
	Phase     string `json:"phase,omitempty"`
}

// Checkpoint represents a rewind point.
type Checkpoint struct {
	ID        string `json:"id"`
	SessionID string `json:"session_id"`
	Prompt    string `json:"prompt,omitempty"`
	CreatedAt string `json:"created_at"`
}

// NewBridge locates the trace binary and returns a bridge.
// Returns a no-op bridge if trace is not installed.
func NewBridge() *Bridge {
	b := &Bridge{}
	path, err := exec.LookPath("trace")
	if err != nil {
		return b
	}
	b.bin = path
	b.ready = true
	return b
}

// Ready reports whether the trace CLI is available.
func (b *Bridge) Ready() bool {
	return b.ready
}

// Enable runs `trace enable` in the given directory.
func (b *Bridge) Enable(ctx context.Context, dir string) error {
	if !b.ready {
		return fmt.Errorf("trace CLI not found")
	}
	_, err := b.run(ctx, dir, "enable", "--agent", "hawk")
	return err
}

// Disable runs `trace disable` in the given directory.
func (b *Bridge) Disable(ctx context.Context, dir string) error {
	if !b.ready {
		return fmt.Errorf("trace CLI not found")
	}
	_, err := b.run(ctx, dir, "disable")
	return err
}

// GetStatus returns the current trace session status.
func (b *Bridge) GetStatus(ctx context.Context, dir string) (*Status, error) {
	if !b.ready {
		return &Status{}, nil
	}
	out, err := b.run(ctx, dir, "status", "--json")
	if err != nil {
		return &Status{}, nil
	}
	var s Status
	if err := json.Unmarshal([]byte(out), &s); err != nil {
		return &Status{}, nil
	}
	return &s, nil
}

// ListCheckpoints returns available checkpoints for the current session.
func (b *Bridge) ListCheckpoints(ctx context.Context, dir string) ([]Checkpoint, error) {
	if !b.ready {
		return nil, nil
	}
	out, err := b.run(ctx, dir, "checkpoint", "list", "--json")
	if err != nil {
		return nil, nil
	}
	var cps []Checkpoint
	if err := json.Unmarshal([]byte(out), &cps); err != nil {
		return nil, nil
	}
	return cps, nil
}

// Rewind restores files to a given checkpoint ID.
func (b *Bridge) Rewind(ctx context.Context, dir string, checkpointID string) error {
	if !b.ready {
		return fmt.Errorf("trace CLI not found")
	}
	_, err := b.run(ctx, dir, "checkpoint", "rewind", "--id", checkpointID)
	return err
}

func (b *Bridge) run(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, b.bin, args...)
	cmd.Dir = dir
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("%s: %s", err, strings.TrimSpace(stderr.String()))
	}
	return strings.TrimSpace(stdout.String()), nil
}
