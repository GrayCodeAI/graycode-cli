package cmd

import (
	"context"
	"fmt"
	"strings"

	hawkconfig "github.com/GrayCodeAI/hawk/internal/config"
	"github.com/GrayCodeAI/hawk/internal/engine"
)

// syncSessionFromPersistedSelection copies eyrie provider.json selection into the
// live session when the session fields are empty (status bar can show ActiveModel
// while s.model is still unset, which breaks deployment routing).
func syncSessionFromPersistedSelection(sess *engine.Session, settings hawkconfig.Settings) {
	if sess == nil {
		return
	}
	ctx := context.Background()
	hawkconfig.SyncSelectionWithCredentials(ctx)

	if strings.TrimSpace(sess.Model()) == "" {
		model := strings.TrimSpace(hawkconfig.ActiveModel(ctx))
		if model != "" && hawkconfig.DeploymentRoutingEnabled(settings) {
			model = hawkconfig.ResolveCanonicalModel(model)
		}
		if model != "" {
			sess.SetModel(model)
		}
	}

	if strings.TrimSpace(sess.Provider()) == "" {
		if provider := strings.TrimSpace(hawkconfig.ActiveProvider(ctx)); provider != "" {
			sess.SetProvider(hawkconfig.NormalizeProviderForEngine(provider))
		}
	}
}

func (m *chatModel) syncSessionSelection() {
	syncSessionFromPersistedSelection(m.session, m.settings)
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
