package cmd

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"

	graycodeconfig "github.com/GrayCodeAI/graycode-cli/internal/config"
)

// handleConfigCommand handles the /config command and all its subcommands.
func (m *chatModel) handleConfigCommand(parts []string, text string) (tea.Model, tea.Cmd) {
	if len(parts) >= 3 && parts[1] == "provider" {
		value := strings.TrimSpace(strings.Join(parts[2:], " "))
		if err := graycodeconfig.SetGlobalSetting("provider", value); err != nil {
			m.messages = append(m.messages, displayMsg{role: "error", content: err.Error()})
			return m, nil
		}
		m.syncSessionSelection()
		// Use cached model or set first from cache
		modelCacheMu.RLock()
		cached, cacheHit := modelCache[m.session.Provider()]
		modelCacheMu.RUnlock()
		if cacheHit && len(cached) > 0 {
			m.session.SetModel(cached[0].ID)
			_ = graycodeconfig.SetGlobalSetting("model", cached[0].ID)
		}
		m.messages = append(m.messages, displayMsg{role: "system", content: fmt.Sprintf("Provider set to: %s\nModel: %s\nSaved in graycode-router (provider.json).", value, m.session.Model())})
		return m, nil
	}
	if len(parts) >= 3 && parts[1] == "model" {
		value := strings.TrimSpace(strings.Join(parts[2:], " "))
		known := configModelChoices(m.configModelOptions, false)
		if len(known) > 0 {
			found := false
			for i, k := range known {
				if strings.EqualFold(k, value) || strings.EqualFold(m.configModelOptions[i].ID, value) {
					value = m.configModelOptions[i].ID
					found = true
					break
				}
			}
			if !found {
				hint := "Unknown model: " + value + "\nUse /model to browse available models."
				m.messages = append(m.messages, displayMsg{role: "error", content: hint})
				return m, nil
			}
		}
		if err := graycodeconfig.SetGlobalSetting("model", value); err != nil {
			m.messages = append(m.messages, displayMsg{role: "error", content: err.Error()})
			return m, nil
		}
		m.syncSessionSelection()
		m.messages = append(m.messages, displayMsg{role: "system", content: fmt.Sprintf("Model switched to: %s\nSaved in graycode-router (provider.json).", m.session.Model())})
		return m, nil
	}
	if len(parts) >= 2 && parts[1] == "keys" {
		m.messages = append(m.messages, displayMsg{role: "system", content: apiKeyConfigSummary()})
		return m, nil
	}
	if len(parts) >= 3 && parts[1] == "key" && parts[2] == "remove" {
		if len(parts) > 3 {
			m.messages = append(m.messages, displayMsg{role: "error", content: "Usage: /config key remove"})
			return m, nil
		}
		return m.openConfigRemoveKeyPanel()
	}
	if len(parts) >= 3 && parts[1] == "get" {
		settings, err := loadEffectiveSettings()
		if err != nil {
			m.messages = append(m.messages, displayMsg{role: "error", content: err.Error()})
			return m, nil
		}
		value, ok := graycodeconfig.SettingValue(settings, parts[2])
		if !ok {
			m.messages = append(m.messages, displayMsg{role: "error", content: fmt.Sprintf("Unsupported setting key %q", parts[2])})
			return m, nil
		}
		if strings.TrimSpace(value) == "" {
			value = "(empty)"
		}
		m.messages = append(m.messages, displayMsg{role: "system", content: fmt.Sprintf("%s = %s", parts[2], value)})
		return m, nil
	}
	if len(parts) >= 4 && parts[1] == "set" {
		key := parts[2]
		value := strings.TrimSpace(strings.Join(parts[3:], " "))
		if err := graycodeconfig.SetGlobalSetting(key, value); err != nil {
			m.messages = append(m.messages, displayMsg{role: "error", content: err.Error()})
			return m, nil
		}
		// Apply common runtime keys immediately.
		normalizedKey := strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(key, "-", ""), "_", ""))
		switch normalizedKey {
		case "model":
			m.syncSessionSelection()
		case "provider":
			m.syncSessionSelection()
		}
		m.messages = append(m.messages, displayMsg{role: "system", content: fmt.Sprintf("Updated %s = %s", key, value)})
		return m, nil
	}
	settings, err := loadEffectiveSettings()
	if err != nil {
		m.messages = append(m.messages, displayMsg{role: "error", content: err.Error()})
		return m, nil
	}
	m.settings = settings
	next, cmd := m.openConfigPanel()
	*m = next
	return m, cmd
}
