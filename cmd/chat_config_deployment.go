package cmd

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	hawkconfig "github.com/GrayCodeAI/hawk/internal/config"
	"github.com/GrayCodeAI/hawk/internal/eyrieclient"
)

type configDeploymentsLoadedMsg struct {
	rows []hawkconfig.DeploymentRow
	err  error
}

type configRoutingPreviewMsg struct {
	body string
	err  error
}

type configCatalogRefreshMsg struct {
	summary string
	err     error
}

type configApplyCredentialsMsg struct {
	summary      string
	err          error
	providerID   string
	modelOptions []configModelOption
}

func (m chatModel) configHubChoices() []string {
	return []string{
		"Connect API key → pick model",
		"API keys (eyrie deployments)",
		"Model (eyrie catalog)",
		"View provider.json + routing",
		fmt.Sprintf("Routing preview (%s)", truncateConfig(m.session.Model(), 28)),
		"Refresh catalog (eyrie discover)",
	}
}

func truncateConfig(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}

func fetchDeploymentsAsync() tea.Cmd {
	return func() tea.Msg {
		rows, err := hawkconfig.ListDeploymentRows(context.Background())
		return configDeploymentsLoadedMsg{rows: rows, err: err}
	}
}

func fetchRoutingPreviewAsync(model string) tea.Cmd {
	return func() tea.Msg {
		body, err := hawkconfig.RoutingPreviewJSON(context.Background(), model)
		return configRoutingPreviewMsg{body: body, err: err}
	}
}

func refreshCatalogAsync() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
		defer cancel()
		summary, err := hawkconfig.RefreshModelCatalogV1(ctx)
		return configCatalogRefreshMsg{summary: summary, err: err}
	}
}

func applyEyrieCredentialsAsync(deploymentID string) tea.Cmd {
	return func() tea.Msg {
		result, err := hawkconfig.ApplyEyrieCredentials(context.Background())
		if err != nil {
			return configApplyCredentialsMsg{err: err, providerID: hawkconfig.ProviderIDForDeployment(deploymentID)}
		}
		providerID := hawkconfig.ProviderIDForDeployment(deploymentID)
		opts := hawkconfig.OptionsFromSetupUI(result.Setup, providerID)
		return configApplyCredentialsMsg{
			summary:      hawkconfig.FormatApplyCredentialsSummary(result),
			providerID:   providerID,
			modelOptions: toConfigModelOptions(opts),
		}
	}
}

func toConfigModelOptions(in []hawkconfig.ModelOption) []configModelOption {
	out := make([]configModelOption, len(in))
	for i, o := range in {
		out[i] = configModelOption{ID: o.ID, DisplayName: o.DisplayName}
	}
	return out
}

func (m chatModel) configDeploymentChoiceLabels() []string {
	if len(m.configDeployments) == 0 {
		return []string{"(loading…)"}
	}
	out := make([]string, len(m.configDeployments))
	for i, row := range m.configDeployments {
		mark := "○"
		if row.Configured {
			mark = "●"
		}
		out[i] = fmt.Sprintf("%s %-22s %s", mark, row.ID, row.Status)
	}
	return out
}

func (m chatModel) configHubView() string {
	return m.configListView("⚙ Hawk Config (eyrie)", m.configHubChoices())
}

func (m chatModel) configDeploymentsView() string {
	return m.configListView("🔑 API keys — pick deployment", m.configDeploymentChoiceLabels())
}

func (m chatModel) configDeploymentDetailView() string {
	titleStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#FF5E0E")).Bold(true)
	mutedStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#8D939E"))
	okStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#4ECDC4"))
	warnStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#e05555"))
	style := lipgloss.NewStyle().Foreground(lipgloss.Color("#E6E6E6"))

	var b strings.Builder
	b.WriteString(titleStyle.Render("Deployment: ") + style.Render(m.configDeploymentID) + "\n\n")
	row, ok := m.configDeploymentRow(m.configDeploymentID)
	if !ok {
		b.WriteString(warnStyle.Render("Not found in catalog") + "\n")
		b.WriteString(mutedStyle.Render("esc back"))
		return b.String()
	}
	b.WriteString(mutedStyle.Render(row.Name) + " · " + row.ProviderID + "\n")
	b.WriteString(fmt.Sprintf("Status: %s\n\n", row.Status))
	b.WriteString(style.Render("Environment:") + "\n")
	for _, ev := range row.EnvVars {
		mark := warnStyle.Render("✗")
		if ev.Set {
			mark = okStyle.Render("✓")
		}
		b.WriteString(fmt.Sprintf("  %s %s\n", mark, ev.Name))
	}
	b.WriteString("\n" + mutedStyle.Render("esc back"))
	return b.String()
}

func (m chatModel) configRoutingView() string {
	titleStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#FF5E0E")).Bold(true)
	mutedStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#8D939E"))
	style := lipgloss.NewStyle().Foreground(lipgloss.Color("#E6E6E6"))

	var b strings.Builder
	b.WriteString(titleStyle.Render("Routing preview") + "\n\n")
	if strings.TrimSpace(m.configRoutingJSON) == "" {
		b.WriteString(mutedStyle.Render("Loading…"))
	} else {
		b.WriteString(style.Render(m.configRoutingJSON))
	}
	b.WriteString("\n\n" + mutedStyle.Render("esc back"))
	return b.String()
}

func (m chatModel) configViewProviderJSON() string {
	titleStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#FF5E0E")).Bold(true)
	mutedStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#8D939E"))
	style := lipgloss.NewStyle().Foreground(lipgloss.Color("#E6E6E6"))

	raw, err := hawkconfig.ProviderConfigJSON()
	if err != nil {
		return titleStyle.Render("provider.json") + "\n\n" + err.Error()
	}
	var b strings.Builder
	b.WriteString(titleStyle.Render("provider.json (eyrie)") + "\n\n")
	b.WriteString(style.Render(raw))
	b.WriteString("\n\n" + mutedStyle.Render("esc back"))
	return b.String()
}

func (m chatModel) configListView(title string, opts []string) string {
	titleStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#FF5E0E")).Bold(true)
	selectedStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#FF5E0E")).Bold(true)
	mutedStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#8D939E"))
	style := lipgloss.NewStyle().Foreground(lipgloss.Color("#E6E6E6"))

	var b strings.Builder
	b.WriteString(titleStyle.Render(title) + "\n\n")
	for i, opt := range opts {
		prefix := "  "
		lineStyle := style
		if i == m.configSel {
			prefix = "❯ "
			lineStyle = selectedStyle
		}
		b.WriteString(lineStyle.Render(prefix+opt) + "\n")
	}
	b.WriteString("\n" + mutedStyle.Render("↑/↓ · enter · esc"))
	return b.String()
}

func (m chatModel) configDeploymentRow(id string) (hawkconfig.DeploymentRow, bool) {
	for _, row := range m.configDeployments {
		if row.ID == id {
			return row, true
		}
	}
	return hawkconfig.DeploymentRow{}, false
}

func (m chatModel) handleConfigHubSelect(option string) (chatModel, tea.Cmd) {
	switch {
	case strings.HasPrefix(option, "Connect API key"):
		m.configMenu = "apikeys"
		m.configSel = 0
		m.configScroll = 0
		m.configDeployments = nil
		m.configNotice = "Step 1: pick deployment · paste key · then pick model"
		return m, fetchDeploymentsAsync()
	case strings.HasPrefix(option, "API keys"):
		m.configMenu = "apikeys"
		m.configSel = 0
		m.configScroll = 0
		m.configDeployments = nil
		return m, fetchDeploymentsAsync()
	case strings.HasPrefix(option, "Model"):
		m.configMenu = "model"
		m.configSel = 0
		m.configScroll = 0
		m.configModelProvider = strings.TrimSpace(m.session.Provider())
		m.configModelOptions = loadConfigModelOptions(m.configModelProvider)
		if len(m.configModelOptions) == 0 {
			return m, fetchModelsAsync(m.configModelProvider)
		}
		return m, nil
	case strings.HasPrefix(option, "View provider"):
		m.configMenu = "view-config"
		m.configSel = 0
		return m, nil
	case strings.HasPrefix(option, "Routing preview"):
		m.configMenu = "routing"
		m.configSel = 0
		m.configScroll = 0
		m.configRoutingJSON = ""
		return m, fetchRoutingPreviewAsync(m.session.Model())
	case strings.HasPrefix(option, "Refresh catalog"):
		m.configNotice = "Refreshing via eyrie…"
		return m, applyEyrieCredentialsAsync("")
	}
	return m, nil
}

func (m chatModel) handleConfigDeploymentSelect(option string) (chatModel, tea.Cmd) {
	parts := strings.Fields(option)
	if len(parts) < 2 {
		return m, nil
	}
	deploymentID := parts[1]
	row, ok := m.configDeploymentRow(deploymentID)
	if !ok {
		return m, nil
	}
	m.configDeploymentID = deploymentID
	if row.Configured {
		m.configMenu = "deployment-detail"
		return m, nil
	}
	envKey := hawkconfig.PrimaryAPIKeyEnvForDeployment(deploymentID)
	if envKey == "" {
		m.configNotice = deploymentID + ": set base URL in environment (local deployment)"
		return m, nil
	}
	m.configProvider = deploymentID
	return m.startConfigEntry("deployment-apikey", deploymentID)
}

func (m chatModel) handleConfigApplyCredentialsMsg(msg configApplyCredentialsMsg) (chatModel, tea.Cmd) {
	if msg.err != nil {
		m.configNotice = msg.err.Error()
		return m, fetchDeploymentsAsync()
	}
	m.configNotice = msg.summary
	modelCache = make(map[string][]configModelOption)
	m.configModelProvider = msg.providerID
	if len(msg.modelOptions) > 0 {
		modelCache[msg.providerID] = msg.modelOptions
	}
	next, cmd := m.rebuildSessionTransport()
	if m.configGuideAfterKey {
		m.configGuideAfterKey = false
		m.configMenu = "model"
		m.configSel = 0
		m.configScroll = 0
		m.configModelOptions = msg.modelOptions
		if len(m.configModelOptions) == 0 {
			m.configModelOptions = loadConfigModelOptions(msg.providerID)
		}
		if len(m.configModelOptions) > 0 {
			m.configNotice = "Step 2: pick a model (" + msg.providerID + ")"
			return next, cmd
		}
	}
	return next, tea.Batch(cmd, fetchDeploymentsAsync())
}

func (m chatModel) rebuildSessionTransport() (chatModel, tea.Cmd) {
	if err := eyrieclient.RebuildSessionTransport(context.Background(), m.session, m.settings, m.session.Provider()); err != nil {
		m.configNotice = err.Error()
	}
	return m, nil
}

func (m chatModel) handleConfigDeploymentMsg(msg configDeploymentsLoadedMsg) (chatModel, tea.Cmd) {
	if msg.err != nil {
		m.configNotice = msg.err.Error()
		return m, nil
	}
	m.configDeployments = msg.rows
	return m, nil
}

func (m chatModel) handleConfigRoutingMsg(msg configRoutingPreviewMsg) (chatModel, tea.Cmd) {
	if msg.err != nil {
		m.configNotice = msg.err.Error()
		return m, nil
	}
	m.configRoutingJSON = msg.body
	return m, nil
}

func (m chatModel) handleConfigCatalogRefreshMsg(msg configCatalogRefreshMsg) (chatModel, tea.Cmd) {
	if msg.err != nil {
		m.configNotice = msg.err.Error()
		return m, fetchDeploymentsAsync()
	}
	m.configNotice = msg.summary
	delete(modelCache, m.session.Provider())
	provider := m.session.Provider()
	cmds := []tea.Cmd{fetchDeploymentsAsync()}
	if m.configMenu == "model" {
		cmds = append(cmds, fetchModelsAsync(provider))
	}
	return m, tea.Batch(cmds...)
}
