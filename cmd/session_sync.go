package cmd

import (
	"context"
	"fmt"
	"strings"

	"github.com/GrayCodeAI/eyrie/runtime"
	"github.com/GrayCodeAI/hawk/internal/engine"
)

// syncSessionFromPersistedSelection copies eyrie provider.json selection into the
// live session when the session fields are empty (status bar can show ActiveModel
// while the model field is still unset, which breaks deployment routing).
func syncSessionFromPersistedSelection(sess *engine.Session) {
	if sess == nil {
		return
	}
	selection := runtime.EffectiveSelection(context.Background(), runtime.SelectionOpts{
		ProviderOverride: strings.TrimSpace(sess.Provider()),
		ModelOverride:    strings.TrimSpace(sess.Model()),
	})

	if strings.TrimSpace(sess.Model()) == "" {
		if selection.Model != "" {
			sess.SetModel(selection.Model)
		}
	}

	if strings.TrimSpace(sess.Provider()) == "" {
		if selection.Provider != "" {
			sess.SetProvider(selection.Provider)
		}
	}
}

func (m *chatModel) syncSessionSelection() {
	syncSessionFromPersistedSelection(m.session)
	if m.session != nil {
		gw, model := m.sessionGatewayModel()
		applyLiveModelMetadata(m.session, gw, model)
	}
}

func (m *chatModel) ensureSessionReadyForChat() error {
	m.syncSessionSelection()
	if strings.TrimSpace(m.session.Model()) == "" {
		return fmt.Errorf("no model selected — open /config, go to Models, and pick one")
	}
	return nil
}
