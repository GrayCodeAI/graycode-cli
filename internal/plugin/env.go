package plugin

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/GrayCodeAI/graycode-cli/internal/storage"
)

// Env vars exported to plugin hooks and child processes (Year 0 PACK-04).
const (
	EnvPluginRoot = "GRAYCODE_PLUGIN_ROOT"
	EnvPluginData = "GRAYCODE_PLUGIN_DATA"
	EnvPluginName = "GRAYCODE_PLUGIN_NAME"
)

// PluginDataDir returns the durable data directory for a plugin.
func PluginDataDir(pluginName string) string {
	return filepath.Join(storage.StateDir(), "plugin-data", pluginName)
}

func ensurePluginDataDir(pluginRoot, pluginName string) string {
	data := PluginDataDir(pluginName)
	_ = os.MkdirAll(data, 0o700)
	return data
}

// pluginHookEnv builds the environment for a plugin hook command.
func pluginHookEnv(pluginRoot, pluginName string, data map[string]interface{}) []string {
	env := os.Environ()
	dataDir := ensurePluginDataDir(pluginRoot, pluginName)
	env = append(
		env,
		EnvPluginRoot+"="+pluginRoot,
		EnvPluginData+"="+dataDir,
		EnvPluginName+"="+pluginName,
	)
	for k, v := range data {
		env = append(env, fmt.Sprintf("GRAYCODE_%s=%v", k, v))
	}
	return env
}
