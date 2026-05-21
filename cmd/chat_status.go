package cmd

import (
	"context"
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

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
	return normalizeModelDisplayName(modelID, displayName), contextLabel
}

// normalizeModelDisplayName prefers a short label when the catalog returns a slug.
func normalizeModelDisplayName(modelID, displayName string) string {
	displayName = strings.TrimSpace(displayName)
	if displayName == "" {
		return shortModelID(modelID)
	}
	if strings.Contains(displayName, "/") {
		if short := shortModelID(modelID); short != "" {
			return short
		}
		return shortModelID(displayName)
	}
	return displayName
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
	m.syncSessionSelection()
	fp := m.connStatusFingerprint()
	if fp == m.connStatusKey {
		return m.connStatusVal
	}
	status := m.buildConnectionStatusPlain()
	m.connStatusKey = fp
	m.connStatusVal = status
	return status
}

func (m chatModel) buildConnectionStatusPlain() string {
	gw, model, ctxLabel := m.connectionStatusParts()
	if gw == "" && model == "" {
		return "pick model"
	}
	if model == "" {
		if gw == "" {
			return "pick model"
		}
		return gw + " · pick model"
	}
	if ctxLabel != "" && ctxLabel != "—" {
		return fmt.Sprintf("%s · %s · %s ctx", gw, model, ctxLabel)
	}
	if gw == "" {
		return model
	}
	return gw + " · " + model
}

func (m chatModel) connectionStatusParts() (gateway, model, contextLabel string) {
	gw, modelID := m.sessionGatewayModel()
	gateway = hawkconfig.GatewayDisplayName(gw)
	if gateway == "" {
		gateway = gw
	}

	if modelID == "" {
		return gateway, "", ""
	}

	model, contextLabel = modelStatusMeta(gw, modelID)
	if contextLabel == "" || contextLabel == "—" {
		contextLabel = "0k"
	}
	return gateway, model, contextLabel
}

// renderConnectionStatus returns styled status text and its visible width for layout.
func (m chatModel) renderConnectionStatus() (string, int) {
	ctx := context.Background()
	if !hawkconfig.HasConfiguredDeploymentCached(ctx) {
		return "", 0
	}

	gw, model, ctxLabel := m.connectionStatusParts()
	return renderChatConnectionStatus(gw, model, ctxLabel)
}

func renderChatConnectionStatus(gateway, model, ctxLabel string) (string, int) {
	muted := configMutedStyle().Inline(true)
	accent := configAccentStyle().Inline(true)
	active := configActiveStyle().Inline(true)
	sep := muted.Render(" · ")
	const sepVis = 3

	if gateway == "" && model == "" {
		s := "pick model"
		return muted.Render(s), len(s)
	}
	if model == "" {
		if gateway == "" {
			s := "pick model"
			return muted.Render(s), len(s)
		}
		s := gateway + " · pick model"
		return lipgloss.JoinHorizontal(lipgloss.Left,
			accent.Render(gateway),
			muted.Render(" · pick model"),
		), len(s)
	}

	var b strings.Builder
	vis := 0
	if gateway != "" {
		b.WriteString(accent.Render(gateway))
		vis += len(gateway)
	}
	if model != "" {
		if vis > 0 {
			b.WriteString(sep)
			vis += sepVis
		}
		b.WriteString(active.Render(model))
		vis += len(model)
	}
	if ctxLabel != "" && ctxLabel != "—" {
		if vis > 0 {
			b.WriteString(sep)
			vis += sepVis
		}
		ctxText := ctxLabel + " ctx"
		b.WriteString(muted.Render(ctxText))
		vis += len(ctxText)
	}
	return b.String(), vis
}

// chatBottomRightStatus is the deployment line on the input bar.
func (m *chatModel) chatBottomRightStatus() string {
	return m.chatConnectionStatus()
}
