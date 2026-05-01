package session

import (
	"crypto/rand"
	"fmt"
	"time"
)

// Fork creates a new session branched from the given session at the given message index.
// All messages up to and including atIndex are copied to the new session.
func Fork(sessionID string, atIndex int) (*Session, error) {
	original, err := Load(sessionID)
	if err != nil {
		return nil, fmt.Errorf("load session for fork: %w", err)
	}

	if atIndex < 0 || atIndex >= len(original.Messages) {
		return nil, fmt.Errorf("invalid fork index %d (session has %d messages)", atIndex, len(original.Messages))
	}

	newID := generateForkID()

	forked := &Session{
		ID:        newID,
		Model:     original.Model,
		Provider:  original.Provider,
		CWD:       original.CWD,
		Name:      fmt.Sprintf("fork of %s at %d", sessionID, atIndex),
		Messages:  make([]Message, atIndex+1),
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	copy(forked.Messages, original.Messages[:atIndex+1])

	if err := Save(forked); err != nil {
		return nil, fmt.Errorf("save forked session: %w", err)
	}

	return forked, nil
}

func generateForkID() string {
	b := make([]byte, 4)
	rand.Read(b)
	return fmt.Sprintf("%x", b)
}
