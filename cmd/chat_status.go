package cmd

import (
	"context"
	"fmt"
	"strings"

	hawkconfig "github.com/GrayCodeAI/hawk/internal/config"
)

func modelStatusMeta(gateway, modelID string) (displayName, contextLabel string) {
	modelID = strings.TrimSpace(modelID)
	if modelID == "" {
		return "", ""
	}
	displayName = shortModelID(modelID)
	for _, o := range loadConfigModelOptions(gateway) {
		if o.ID != modelID {
			continue
		}
		if n := strings.TrimSpace(o.DisplayName); n != "" {
			displayName = n
		}
		contextLabel = formatModelTableContext(o.ContextWindow)
		break
	}
	return displayName, contextLabel
}

func (m *chatModel) invalidateConnStatus() {
	m.connStatusKey = ""
}

func (m chatModel) connStatusFingerprint() string {
	gw, model := m.sessionGatewayModel()
	creds := strings.Join(hawkconfig.ConfiguredCredentialProviders(), ",")
	return gw + "\x00" + model + "\x00" + creds
}

func (m chatModel) sessionGatewayModel() (gateway, model string) {
	if m.session != nil {
		gateway = strings.TrimSpace(m.session.Provider())
		model = strings.TrimSpace(m.session.Model())
	}
	if gateway == "" || model == "" {
		ctx := context.Background()
		if gateway == "" {
			gateway = hawkconfig.ActiveGateway(ctx)
		}
		if model == "" {
			model = strings.TrimSpace(hawkconfig.ActiveModel(ctx))
		}
	}
	return gateway, model
}

func (m *chatModel) chatConnectionStatus() string {
	ctx := context.Background()
	if !hawkconfig.HasConfiguredDeploymentCached(ctx) {
		return ""
	}
	fp := m.connStatusFingerprint()
	if fp == m.connStatusKey {
		return m.connStatusVal
	}
	status := m.buildConnectionStatus()
	m.connStatusKey = fp
	m.connStatusVal = status
	return status
}

func (m chatModel) buildConnectionStatus() string {
	gw, model := m.sessionGatewayModel()
	gwLabel := hawkconfig.GatewayDisplayName(gw)
	if gwLabel == "" {
		gwLabel = gw
	}

	if model == "" {
		if gwLabel == "" {
			return "pick model"
		}
		return gwLabel + ": pick model"
	}

	displayName, ctxLabel := modelStatusMeta(gw, model)
	if ctxLabel == "" || ctxLabel == "—" {
		ctxLabel = "0k"
	}
	if gwLabel == "" {
		return fmt.Sprintf("%s .%s", displayName, ctxLabel)
	}
	return fmt.Sprintf("%s: %s .%s", gwLabel, displayName, ctxLabel)
}

// chatBottomRightStatus is the deployment line on the input bar.
// No keys: empty (welcome screen carries setup hints).
// With key: Gateway: Model .262k
func (m *chatModel) chatBottomRightStatus() string {
	return m.chatConnectionStatus()
}
