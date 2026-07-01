package cmd

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/GrayCodeAI/eyrie/catalog/xiaomi"
	tea "github.com/charmbracelet/bubbletea"
)

var platformCtxCache struct {
	mu  sync.Mutex
	idx map[string]int
	at  time.Time
}

const platformCtxCacheTTL = 10 * time.Minute

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
	modelID = xiaomi.NativeModelID(strings.TrimSpace(modelID))
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
		idx, err := xiaomi.FetchPlatformModelsIndex(context.Background(), "")
		if err != nil {
			return platformContextIndexMsg{err: err}
		}
		m := make(map[string]int, len(idx))
		for k, pm := range idx {
			if pm.ContextLength > 0 {
				m[xiaomi.NativeModelID(k)] = pm.ContextLength
			}
		}
		return platformContextIndexMsg{idx: m}
	}
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

func seedPlatformContextCacheForTest(idx map[string]int) {
	platformCtxCache.mu.Lock()
	platformCtxCache.idx = idx
	platformCtxCache.at = time.Now()
	platformCtxCache.mu.Unlock()
}
