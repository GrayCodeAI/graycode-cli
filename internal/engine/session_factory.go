package engine

import (
	"context"
	"errors"
	"fmt"
	"strings"

	eyrieengine "github.com/GrayCodeAI/eyrie/engine"

	"github.com/GrayCodeAI/hawk/internal/tool"
)

// BuildChatClient returns an LLM client and whether deployment routing is active.
func BuildChatClient(ctx context.Context, selection eyrieengine.Selection, legacyProvider string) (ChatClient, string, bool, error) {
	_ = ctx // request contexts are applied per generation by the facade adapter
	provider := strings.TrimSpace(selection.Provider)
	if provider == "" {
		provider = legacyProvider
	}
	resolvedSelection := selection
	if strings.TrimSpace(resolvedSelection.Provider) == "" {
		resolvedSelection.Provider = provider
	}
	modelRuntime, err := eyrieengine.New(eyrieengine.Options{})
	if err != nil {
		return nil, provider, false, fmt.Errorf("eyrie transport: %w", err)
	}
	label := strings.TrimSpace(resolvedSelection.Provider)
	if label == "" {
		label = provider
	}
	return newEyrieEngineClient(modelRuntime), label, true, nil
}

// NewHawkSession constructs a Session using an engine-resolved selection.
func NewHawkSession(ctx context.Context, selection eyrieengine.Selection, provider, model, systemPrompt string, registry *tool.Registry) *Session {
	chat, label, deploy, err := BuildChatClient(ctx, selection, provider)
	if err != nil {
		chat = NewUnavailableChatClient(err)
	}
	resolvedModel := strings.TrimSpace(selection.Model)
	if resolvedModel == "" {
		resolvedModel = model
	}
	return NewSessionWithClient(chat, label, resolvedModel, systemPrompt, registry, deploy)
}

// RebuildSessionTransport rebuilds the LLM client from the engine-resolved selection.
func RebuildSessionTransport(ctx context.Context, s *Session, selection eyrieengine.Selection, legacyProvider string) error {
	if s == nil {
		return errors.New("session is nil")
	}
	chat, label, deploy, err := BuildChatClient(ctx, selection, legacyProvider)
	if err != nil {
		return err
	}
	s.ReattachTransport(chat, label, deploy)
	return nil
}
