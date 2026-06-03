package cmd

// Config panel state constants for the /config TUI.
//
// Fields on chatModel use these values:
//   - configTab       — main tab (Gateways / Models)
//   - configEntry     — input overlay (API key paste, Ollama URL, key view)
//   - configProvider  — gateway id while an entry overlay is open

// Config tabs (configTab).
const (
	configTabGateways = 0
	configTabModels   = 1
)

var configTabLabels = []string{"Gateways", "Models"}

// Config entry overlays (configEntry).
const (
	configEntryNone        = ""
	configEntryAPIKeyPaste = "apikey-paste"
	configEntryOllamaURL   = "ollama-url"
	configEntryKeyView     = "key-view"
)

// Providers referenced by config UI flows.
const (
	configProviderOllama = "ollama"
)

const configDefaultOllamaURL = "http://localhost:11434/v1"