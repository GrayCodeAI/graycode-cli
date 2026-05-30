// Package lsp provides a Language Server Protocol client for hawk.
// It manages LSP server subprocesses with refcounted connection pooling,
// idle reaping, and typed crash recovery.
package lsp

import (
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// ServerConfig defines how to launch a language server for a given language.
type ServerConfig struct {
	Command    string   `json:"command"`
	Args       []string `json:"args,omitempty"`
	Extensions []string `json:"extensions"`
}

// LSPConfig holds all configured language servers.
type LSPConfig struct {
	Servers map[string]ServerConfig `json:"lsp"`
}

var (
	globalConfig *LSPConfig
	configOnce   sync.Once
)

// LoadConfig merges project-level (.hawk/lsp.json) and global (~/.hawk/lsp.json)
// LSP configuration. Project-level overrides global.
func LoadConfig(projectDir string) *LSPConfig {
	configOnce.Do(func() {
		globalConfig = &LSPConfig{Servers: make(map[string]ServerConfig)}
		// Load global config
		home, _ := os.UserHomeDir()
		if home != "" {
			loadConfigFile(filepath.Join(home, ".hawk", "lsp.json"), globalConfig)
		}
		// Load project config (overrides global)
		if projectDir != "" {
			loadConfigFile(filepath.Join(projectDir, ".hawk", "lsp.json"), globalConfig)
		}
		// Apply built-in defaults for unconfigured languages
		applyDefaults(globalConfig)
	})
	return globalConfig
}

// ResetConfig clears the cached config (for testing).
func ResetConfig() {
	globalConfig = nil
	configOnce = sync.Once{}
}

func loadConfigFile(path string, cfg *LSPConfig) {
	data, err := os.ReadFile(path)
	if err != nil {
		return // file not found is fine
	}
	var loaded LSPConfig
	if err := json.Unmarshal(data, &loaded); err != nil {
		slog.Warn("lsp: failed to parse config", "path", path, "error", err)
		return
	}
	for lang, server := range loaded.Servers {
		cfg.Servers[lang] = server
	}
}

func applyDefaults(cfg *LSPConfig) {
	defaults := map[string]ServerConfig{
		"go": {
			Command:    "gopls",
			Extensions: []string{".go"},
		},
		"typescript": {
			Command:    "typescript-language-server",
			Args:       []string{"--stdio"},
			Extensions: []string{".ts", ".tsx", ".js", ".jsx"},
		},
		"python": {
			Command:    "pylsp",
			Extensions: []string{".py"},
		},
		"rust": {
			Command:    "rust-analyzer",
			Extensions: []string{".rs"},
		},
	}
	for lang, def := range defaults {
		if _, exists := cfg.Servers[lang]; !exists {
			cfg.Servers[lang] = def
		}
	}
}

// LanguageForExtension returns the language name for a file extension,
// or empty string if no LSP server is configured for that extension.
func (c *LSPConfig) LanguageForExtension(ext string) string {
	if !strings.HasPrefix(ext, ".") {
		ext = "." + ext
	}
	for lang, server := range c.Servers {
		for _, e := range server.Extensions {
			if strings.EqualFold(e, ext) {
				return lang
			}
		}
	}
	return ""
}

// ServerForFile returns the language and server config for a file path.
func (c *LSPConfig) ServerForFile(filePath string) (string, ServerConfig, bool) {
	ext := strings.ToLower(filepath.Ext(filePath))
	lang := c.LanguageForExtension(ext)
	if lang == "" {
		return "", ServerConfig{}, false
	}
	server, ok := c.Servers[lang]
	return lang, server, ok
}
