package engine

import (
	"context"
	"errors"
	"fmt"
	"strings"

	eyrieengine "github.com/GrayCodeAI/eyrie/engine"

	hawkconfig "github.com/GrayCodeAI/hawk/internal/config"
	"github.com/GrayCodeAI/hawk/internal/tool"
)

// BuildChatClient returns an LLM client and whether deployment routing is active.
func BuildChatClient(ctx context.Context, selection eyrieengine.Selection, legacyProvider string) (ChatClient, string, bool, error) {
	modelRuntime, err := hawkconfig.NewEyrieEngine()
	if err != nil {
		return nil, requestedProvider(selection, legacyProvider), false, fmt.Errorf("eyrie transport: %w", err)
	}
	return buildChatClientWithRuntime(ctx, modelRuntime, selection, legacyProvider)
}

func buildChatClientWithRuntime(ctx context.Context, modelRuntime *eyrieengine.Engine, selection eyrieengine.Selection, legacyProvider string) (ChatClient, string, bool, error) {
	_ = ctx // request contexts are applied per generation by the facade adapter
	provider := strings.TrimSpace(selection.Provider)
	if provider == "" {
		provider = legacyProvider
	}
	resolvedSelection := selection
	if strings.TrimSpace(resolvedSelection.Provider) == "" {
		resolvedSelection.Provider = provider
	}
	if modelRuntime == nil {
		return nil, provider, false, errors.New("eyrie transport: runtime is nil")
	}
	label := strings.TrimSpace(resolvedSelection.Provider)
	if label == "" {
		label = provider
	}
	return newEyrieEngineClient(modelRuntime), label, true, nil
}

// BuildChatClientForSettings composes the model runtime from one effective
// settings snapshot. It is the command-facing path for --settings isolation.
func BuildChatClientForSettings(ctx context.Context, settings hawkconfig.Settings, selection eyrieengine.Selection, legacyProvider string) (ChatClient, string, bool, error) {
	modelRuntime, err := hawkconfig.NewEyrieEngineForSettings(settings)
	if err != nil {
		return nil, requestedProvider(selection, legacyProvider), false, fmt.Errorf("eyrie transport: %w", err)
	}
	return buildChatClientWithRuntime(ctx, modelRuntime, selection, legacyProvider)
}

func requestedProvider(selection eyrieengine.Selection, legacyProvider string) string {
	if provider := strings.TrimSpace(selection.Provider); provider != "" {
		return provider
	}
	return strings.TrimSpace(legacyProvider)
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

// NewHawkSessionForSettings constructs a session with an invocation-scoped
// Eyrie engine, including custom gateways from an explicit settings file.
func NewHawkSessionForSettings(ctx context.Context, settings hawkconfig.Settings, selection eyrieengine.Selection, provider, model, systemPrompt string, registry *tool.Registry) *Session {
	chat, label, deploy, err := BuildChatClientForSettings(ctx, settings, selection, provider)
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

func RebuildSessionTransportForSettings(ctx context.Context, settings hawkconfig.Settings, s *Session, selection eyrieengine.Selection, legacyProvider string) error {
	if s == nil {
		return errors.New("session is nil")
	}
	chat, label, deploy, err := BuildChatClientForSettings(ctx, settings, selection, legacyProvider)
	if err != nil {
		return err
	}
	s.ReattachTransport(chat, label, deploy)
	return nil
}
