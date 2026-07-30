package config

import (
	"strings"
)

// ThinkingPrefForModel returns the stored per-model thinking preference, or nil
// when the model has no explicit entry in Settings.ModelThinking.
func ThinkingPrefForModel(s Settings, modelID string) *bool {
	modelID = strings.TrimSpace(modelID)
	if modelID == "" || len(s.ModelThinking) == 0 {
		return nil
	}
	if enabled, ok := s.ModelThinking[modelID]; ok {
		v := enabled
		return &v
	}
	if alt := alternateModelThinkingKey(modelID); alt != "" {
		if enabled, ok := s.ModelThinking[alt]; ok {
			v := enabled
			return &v
		}
	}
	// Bare id → find provider/bare entry.
	if !strings.Contains(modelID, "/") {
		suffix := "/" + modelID
		for key, enabled := range s.ModelThinking {
			if strings.HasSuffix(key, suffix) {
				v := enabled
				return &v
			}
		}
	}
	return nil
}

func alternateModelThinkingKey(modelID string) string {
	if i := strings.IndexByte(modelID, '/'); i > 0 && i+1 < len(modelID) {
		return modelID[i+1:]
	}
	return ""
}

// SetModelThinking persists an on/off thinking preference for a catalog model id.
func SetModelThinking(modelID string, enabled bool) error {
	modelID = strings.TrimSpace(modelID)
	if modelID == "" {
		return nil
	}
	s := LoadGlobalSettings()
	if s.ModelThinking == nil {
		s.ModelThinking = make(map[string]bool, 1)
	}
	s.ModelThinking[modelID] = enabled
	return SaveGlobal(s)
}

// ClearModelThinking removes a per-model thinking preference so the provider
// default / global fallback applies again.
func ClearModelThinking(modelID string) error {
	modelID = strings.TrimSpace(modelID)
	if modelID == "" {
		return nil
	}
	s := LoadGlobalSettings()
	if len(s.ModelThinking) == 0 {
		return nil
	}
	delete(s.ModelThinking, modelID)
	if alt := alternateModelThinkingKey(modelID); alt != "" {
		delete(s.ModelThinking, alt)
	}
	if len(s.ModelThinking) == 0 {
		s.ModelThinking = nil
	}
	return SaveGlobal(s)
}

// ModelCapabilitySupportsThinking reports whether catalog capability tags include reasoning/thinking.
func ModelCapabilitySupportsThinking(capabilities []string) bool {
	for _, capability := range capabilities {
		switch strings.ToLower(strings.TrimSpace(capability)) {
		case "reasoning", "thinking", "adaptive_thinking", "explicit_thinking_budget", "effort":
			return true
		}
	}
	return false
}

// ResolveThinkingForModel picks the thinking toggle for a session:
//  1. explicit ModelThinking[modelID] when present
//  2. providers that default thinking ON when omitted → false (safe chat default)
//  3. otherwise global GLMThinkingEnabled / thinking fallback (including nil)
func ResolveThinkingForModel(s Settings, modelID, providerID string) *bool {
	if pref := ThinkingPrefForModel(s, modelID); pref != nil {
		return pref
	}
	switch strings.ToLower(strings.TrimSpace(providerID)) {
	case "longcat", "kimi", "deepseek",
		"xiaomi_mimo", "xiaomi_mimo_payg", "xiaomi_mimo_token_plan",
		"minimax_payg", "minimax_token_plan":
		disabled := false
		return &disabled
	}
	return s.GLMThinkingEnabled
}

// ResolveGLMThinkingForModel is a deprecated alias of ResolveThinkingForModel.
func ResolveGLMThinkingForModel(s Settings, modelID, providerID string) *bool {
	return ResolveThinkingForModel(s, modelID, providerID)
}

// FormatModelThinkingLabel returns the Models-table Think cell: "on", "off", or "—".
func FormatModelThinkingLabel(supportsThinking bool, pref *bool, providerID string) string {
	if !supportsThinking {
		return "—"
	}
	if pref != nil {
		if *pref {
			return "on"
		}
		return "off"
	}
	// Unset: LongCat (and conservative UI default) display as off.
	_ = providerID
	return "off"
}
