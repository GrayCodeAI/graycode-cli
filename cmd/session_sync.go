package cmd

import (
	"context"
	"fmt"
	"strings"

	"github.com/GrayCodeAI/eyrie/runtime"
	hawkconfig "github.com/GrayCodeAI/hawk/internal/config"
	"github.com/GrayCodeAI/hawk/internal/engine"
)

func explicitSelection(ctx context.Context) (provider, model string) {
	if ctx == nil {
		ctx = context.Background()
	}
	return strings.TrimSpace(hawkconfig.ActiveGateway(ctx)), strings.TrimSpace(hawkconfig.ActiveModel(ctx))
}

// syncSessionFromPersistedSelection copies explicit eyrie provider.json
// selection into the live session when the session fields are empty.
// It intentionally avoids runtime defaults so Hawk can preserve the
// "gateway selected, model still missing" setup state.
func syncSessionFromPersistedSelection(sess *engine.Session) {
	if sess == nil {
		return
	}
	provider, model := explicitSelection(context.Background())

	if strings.TrimSpace(sess.Model()) == "" {
		if model != "" {
			sess.SetModel(model)
		}
	}

	if strings.TrimSpace(sess.Provider()) == "" {
		if provider != "" {
			sess.SetProvider(provider)
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
	if strings.TrimSpace(m.session.Model()) != "" {
		return nil
	}

	ctx := context.Background()
	explicitProvider, explicitModel := explicitSelection(ctx)
	if explicitProvider != "" && explicitModel == "" {
		return fmt.Errorf("no model selected — open /config, go to Models, and pick one")
	}

	selection := runtime.EffectiveSelection(ctx, runtime.SelectionOpts{
		ProviderOverride: strings.TrimSpace(m.session.Provider()),
		ModelOverride:    strings.TrimSpace(m.session.Model()),
	})
	if strings.TrimSpace(selection.Model) == "" {
		return fmt.Errorf("no model selected — open /config, go to Models, and pick one")
	}
	if strings.TrimSpace(m.session.Provider()) == "" && strings.TrimSpace(selection.Provider) != "" {
		m.session.SetProvider(selection.Provider)
	}
	m.session.SetModel(selection.Model)
	return nil
}
