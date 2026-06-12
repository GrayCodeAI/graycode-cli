package cmd

import (
	"context"
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	hawkconfig "github.com/GrayCodeAI/hawk/internal/config"
	"github.com/GrayCodeAI/hawk/internal/engine"
)

func modelStatusMeta(gateway, modelID string) (displayName, contextLabel string) {
	modelID = strings.TrimSpace(modelID)
	if modelID == "" {
		return "", ""
	}
	displayName = shortModelID(modelID)
	if opt, ok := lookupModelOption(gateway, modelID); ok {
		if n := strings.TrimSpace(opt.DisplayName); n != "" {
			displayName = n
		}
		if opt.ContextWindow > 0 {
			contextLabel = formatModelTableContext(opt.ContextWindow)
		}
		return normalizeModelDisplayName(modelID, displayName), contextLabel
	}
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
	gw, modelName := m.sessionGatewayModel()
	creds := strings.Join(hawkconfig.ConfiguredCredentialProviders(), ",")
	api := 0
	if m.session != nil {
		api = m.session.LastPromptTokens()
	}
	used := sessionContextUsedTokens(m.session)
	return gw + "\x00" + modelName + "\x00" + creds + "\x00" + fmt.Sprintf("%d", used) + "\x00" + fmt.Sprintf("%d", api)
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
	gw, modelName, ctxLabel := m.connectionStatusParts()
	if gw == "" && modelName == "" {
		return "pick model"
	}
	if modelName == "" {
		if gw == "" {
			return "pick model"
		}
		return gw + " · pick model"
	}
	if ctxLabel != "" && ctxLabel != "—" {
		ctxText := formatConnectionContextLabel(m, ctxLabel)
		if ctxText != "" {
			return fmt.Sprintf("%s · %s · %s", gw, modelName, ctxText)
		}
	}
	if gw == "" {
		return modelName
	}
	return gw + " · " + modelName
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
	if contextLabel == "" || contextLabel == "—" || contextLabel == "0k" {
		if m.session != nil {
			if w := m.session.ContextWindowCached; w > 0 {
				contextLabel = formatModelTableContext(w)
			} else if w := m.session.ContextWindowSize(); w > 0 && w != engine.DefaultContextWindow {
				contextLabel = formatModelTableContext(w)
			}
		}
		if (contextLabel == "" || contextLabel == "—") && isXiaomiMimoProvider(gw) {
			if w := platformContextForNativeModel(modelID); w > 0 {
				contextLabel = formatModelTableContext(w)
				if m.session != nil {
					m.session.ContextWindowCached = w
					m.session.EnsureAutoCompactor()
				}
			}
		}
		// Omit ctx segment until a real window is known — never show the 128k default.
	}
	return gateway, model, contextLabel
}

// renderConnectionStatusSplit returns gateway/model and context usage as separate
// footer segments so context can sit flush on the right edge.
func (m chatModel) renderConnectionStatusSplit() (modelRendered string, modelVis int, ctxRendered string, ctxVis int) {
	ctx := context.Background()
	if !hawkconfig.HasConfiguredDeploymentCached(ctx) {
		return "", 0, "", 0
	}

	gw, modelName, ctxLabel := m.connectionStatusParts()
	ctxText := formatConnectionContextLabel(m, ctxLabel)
	modelRendered, modelVis = renderChatConnectionModel(gw, modelName)
	if ctxText != "" {
		ctxRendered, ctxVis = renderChatConnectionContext(ctxText, contextUsagePercent(m, ctxLabel))
	}
	return modelRendered, modelVis, ctxRendered, ctxVis
}

func renderChatConnectionModel(gateway, model string) (string, int) {
	muted := configMutedStyle().Inline(true)
	gatewayStyle := configAccentStyle().Inline(true)
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
		return lipgloss.JoinHorizontal(
			lipgloss.Left,
			gatewayStyle.Render(gateway),
			muted.Render(" · pick model"),
		), len(s)
	}

	var b strings.Builder
	vis := 0
	if gateway != "" {
		b.WriteString(gatewayStyle.Render(gateway))
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
	return b.String(), vis
}

func renderChatConnectionContext(ctxText string, ctxPct int) (string, int) {
	// ctxText is already styled by formatConnectionContextLabel (per-part
	// colors). Return as-is; the visible width is computed by stripping
	// ANSI escape sequences (the raw string contains 4 styled segments
	// each wrapped in their own escape).
	if ctxText == "" {
		return "", 0
	}
	return ctxText, visibleLen(ctxText)
}

// visibleLen returns the visible (printed) length of s, ignoring ANSI
// escape sequences. Used by the footer layout to compute column widths.
func visibleLen(s string) int {
	n := 0
	for i := 0; i < len(s); {
		if s[i] == 0x1b && i+1 < len(s) && s[i+1] == '[' {
			// Skip until the final byte of the CSI sequence.
			j := i + 2
			for j < len(s) {
				b := s[j]
				if b >= 0x40 && b <= 0x7e {
					j++
					break
				}
				j++
			}
			i = j
			continue
		}
		n++
		i++
	}
	return n
}

func renderChatConnectionStatus(gateway, model, ctxText string, ctxPct int) (string, int) {
	modelRendered, modelVis := renderChatConnectionModel(gateway, model)
	if ctxText == "" {
		return modelRendered, modelVis
	}
	ctxRendered, ctxVis := renderChatConnectionContext(ctxText, ctxPct)
	sep := configMutedStyle().Inline(true).Render(" · ")
	return modelRendered + sep + ctxRendered, modelVis + 3 + ctxVis
}

func sessionContextUsedTokens(sess *engine.Session) int {
	if sess == nil {
		return 0
	}
	return sess.ContextUsedTokens()
}

func contextUsagePercent(m chatModel, windowLabel string) int {
	window := parseContextWindowLabel(windowLabel)
	if window <= 0 {
		return 0
	}
	used := sessionContextUsedTokens(m.session)
	pct := int(float64(used) / float64(window) * 100)
	if pct > 999 {
		return 999
	}
	return pct
}

func formatConnectionContextLabel(m chatModel, windowLabel string) string {
	windowLabel = strings.TrimSpace(windowLabel)
	if windowLabel == "" || windowLabel == "—" {
		return ""
	}
	window := parseContextWindowLabel(windowLabel)
	if window <= 0 {
		return windowLabel + " ctx"
	}
	usedLabel := formatContextUsedLabel(sessionContextUsedTokens(m.session))
	pct := contextUsagePercent(m, windowLabel)

	// Per-part coloring so the eye can scan the percentage at a glance:
	//   "0k"   → textPrimary (the "value" — primary data, distinct from
	//                        the sage session-token count in the same
	//                        status bar so the two don't visually merge)
	//   "/"    → textMuted  (separator)
	//   "262k" → textMuted  (the "capacity" — secondary information)
	//   "ctx " → textMuted  (label)
	//   "(0%)" → successTeal / warnAmber / errorCoral by threshold
	//           (the "status" — this is the part that changes meaning)
	muted := configMutedStyle().Inline(true)
	valueStyle := lipgloss.NewStyle().Foreground(textPrimary).Inline(true)
	pctStyle := lipgloss.NewStyle().Foreground(contextPercentColor(pct)).Inline(true)

	var b strings.Builder
	b.WriteString(valueStyle.Render(usedLabel))
	b.WriteString(muted.Render("/" + windowLabel))
	b.WriteString(muted.Render(" ctx ("))
	b.WriteString(pctStyle.Render(fmt.Sprintf("%d%%", pct)))
	b.WriteString(muted.Render(")"))
	return b.String()
}

// contextPercentColor returns the appropriate theme color for a
// context-window usage percentage. Exposed so the footer test in
// chat_status_test.go can assert the threshold logic without
// depending on the full format function.
func contextPercentColor(pct int) lipgloss.Color {
	switch {
	case pct >= 95:
		return errorCoral
	case pct >= 80:
		return warnAmber
	default:
		return doneGreen
	}
}

func formatContextUsedLabel(tokens int) string {
	if tokens <= 0 {
		return "0k"
	}
	return formatModelTableContext(tokens)
}

// chatBottomRightStatus is the deployment line on the input bar.
func (m *chatModel) chatBottomRightStatus() string {
	return m.chatConnectionStatus()
}
