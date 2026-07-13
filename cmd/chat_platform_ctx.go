package cmd

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	tea "charm.land/bubbletea/v2"
	hawkconfig "github.com/GrayCodeAI/hawk/internal/config"
)

var platformCtxCache struct {
	mu  sync.Mutex
	idx map[string]int
	at  time.Time
}

func isXiaomiMimoProvider(provider string) bool {
	p := strings.ToLower(strings.TrimSpace(provider))
	p = strings.ReplaceAll(p, "-", "_")
	switch p {
	case "xiaomi_mimo_token_plan", "xiaomi_mimo_payg", "xiaomi_mimo":
		return true
	default:
		return false
	}
}

// platformContextForNativeModel reads the public MiMo platform catalog (no API key) from cache.
func platformContextForNativeModel(modelID string) int {
	modelID = nativeModelID(modelID)
	if modelID == "" {
		return 0
	}

	platformCtxCache.mu.Lock()
	defer platformCtxCache.mu.Unlock()

	if platformCtxCache.idx == nil {
		return 0
	}
	return platformCtxCache.idx[modelID]
}

type platformContextIndexMsg struct {
	idx map[string]int
	err error
}

func fetchPlatformContextIndexCmd() tea.Cmd {
	return func() tea.Msg {
		models, err := hawkconfig.ListPublicEngineModels(context.Background(), "xiaomi_mimo_payg")
		if err != nil {
			return platformContextIndexMsg{err: err}
		}
		m := make(map[string]int, len(models))
		for _, model := range models {
			if model.ContextWindow > 0 {
				m[nativeModelID(model.ID)] = model.ContextWindow
			}
		}
		if len(m) == 0 {
			return platformContextIndexMsg{err: fmt.Errorf("no Xiaomi model metadata available")}
		}
		return platformContextIndexMsg{idx: m}
	}
}

func nativeModelID(id string) string {
	id = strings.TrimSpace(id)
	if index := strings.LastIndex(id, "/"); index >= 0 {
		return id[index+1:]
	}
	return id
}

func updatePlatformContextCache(msg platformContextIndexMsg) {
	platformCtxCache.mu.Lock()
	defer platformCtxCache.mu.Unlock()
	if msg.err != nil {
		if platformCtxCache.idx == nil {
			platformCtxCache.idx = make(map[string]int)
		}
		platformCtxCache.at = time.Now()
		return
	}
	platformCtxCache.idx = msg.idx
	platformCtxCache.at = time.Now()
}

func invalidatePlatformContextCache() {
	platformCtxCache.mu.Lock()
	platformCtxCache.idx = nil
	platformCtxCache.at = time.Time{}
	platformCtxCache.mu.Unlock()
}
