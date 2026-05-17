package session

import (
	"fmt"
	"time"
)

type ForkOptions struct {
	SourceThreadID string
	FromStepID     string
	NewThreadName  string
}

type ThreadFork struct {
	OriginalThreadID string    `json:"original_thread_id"`
	NewThreadID      string    `json:"new_thread_id"`
	ForkPointStepID  string    `json:"fork_point_step_id"`
	CreatedAt        time.Time `json:"created_at"`
}

type ForkableStore interface {
	GetForkCheckpoints(threadID string) ([]ForkCheckpoint, error)
	CreateThread(name string) (string, error)
	CopyCheckpoints(from string, to string, upToStep string) error
}

type ForkCheckpoint struct {
	StepID    string    `json:"step_id"`
	ThreadID  string    `json:"thread_id"`
	Data      []byte    `json:"data"`
	Kind      string    `json:"kind"`
	CreatedAt time.Time `json:"created_at"`
}

func ForkThread(store ForkableStore, opts ForkOptions) (*ThreadFork, error) {
	if opts.SourceThreadID == "" {
		return nil, fmt.Errorf("source thread ID required")
	}

	name := opts.NewThreadName
	if name == "" {
		name = fmt.Sprintf("fork-%s-%d", opts.SourceThreadID[:8], time.Now().Unix())
	}

	newThreadID, err := store.CreateThread(name)
	if err != nil {
		return nil, fmt.Errorf("create thread: %w", err)
	}

	if err := store.CopyCheckpoints(opts.SourceThreadID, newThreadID, opts.FromStepID); err != nil {
		return nil, fmt.Errorf("copy checkpoints: %w", err)
	}

	return &ThreadFork{
		OriginalThreadID: opts.SourceThreadID,
		NewThreadID:      newThreadID,
		ForkPointStepID:  opts.FromStepID,
		CreatedAt:        time.Now(),
	}, nil
}
