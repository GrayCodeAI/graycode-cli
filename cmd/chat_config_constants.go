package cmd

// Config panel state constants for the /config TUI.
//
// Fields on chatModel use these values:
//   - configTab       — main tab (Keys / Gateways / Models)
//   - configEntry     — input overlay (API key paste, Ollama URL)
//   - configMenu      — list overlay (gateway pick after paste)
//   - configProvider  — provider id while an entry overlay is open

// Config tabs (configTab).
const (
	configTabKeys     = 0
	configTabGateways = 1
	configTabModels   = 2
)

var configTabLabels = []string{"Keys", "Gateways", "Models"}

// Config entry overlays (configEntry).
const (
	configEntryNone      = ""
	configEntryAPIKeyPaste = "apikey-paste"
	configEntryOllamaURL   = "ollama-url"
)

// Config menu overlays (configMenu).
const (
	configMenuNone      = ""
	configMenuProviders = "providers"
)

// Keys tab row kinds (configKeysRow.kind).
const (
	configKeysRowCredential = "credential"
	configKeysActionAdd     = "add"
	configKeysActionOllama  = "ollama"
)

// Providers referenced by config UI flows.
const (
	configProviderOllama = "ollama"
)

const configDefaultOllamaURL = "http://localhost:11434/v1"
