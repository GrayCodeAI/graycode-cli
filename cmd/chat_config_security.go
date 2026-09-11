package cmd

import (
	"strings"
	"sync"

	"github.com/GrayCodeAI/hawk/internal/engine"
)

var (
	configNoticeRedactorOnce sync.Once
	configNoticeRedactor     *engine.OutputRedactor
)

func configNoticeRedact() *engine.OutputRedactor {
	configNoticeRedactorOnce.Do(func() {
		configNoticeRedactor = engine.NewOutputRedactor()
	})
	return configNoticeRedactor
}

// sanitizeConfigNotice redacts API keys and tokens before showing errors in the TUI.
func sanitizeConfigNotice(notice string) string {
	notice = strings.TrimSpace(notice)
	if notice == "" {
		return ""
	}
	return configNoticeRedact().Redact(notice)
}

func (m *chatModel) wipeConfigKeyInput() {
	m.configInput.Reset()
	m.configInput.SetValue("")
}
