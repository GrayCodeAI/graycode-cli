package cmd

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/GrayCodeAI/eyrie/credentials"
	hawkconfig "github.com/GrayCodeAI/hawk/internal/config"
	tea "github.com/charmbracelet/bubbletea"
)

func TestFinishConfigEntry_APIKeyPaste_SavesBeforeProbe(t *testing.T) {
	hawkconfig.InvalidateConfigUICache()
	store := &credentials.MapStore{}
	credentials.SetDefaultStore(store)
	t.Cleanup(func() {
		credentials.SetDefaultStore(nil)
		hawkconfig.InvalidateConfigUICache()
	})

	oldTransport := http.DefaultTransport
	http.DefaultTransport = configSaveTestTransport(http.StatusUnauthorized)
	t.Cleanup(func() { http.DefaultTransport = oldTransport })

	if _, err := hawkconfig.CredentialInferenceForProvider("xiaomi_mimo_payg"); err != nil {
		t.Fatalf("CredentialInferenceForProvider: %v", err)
	}

	secret := "mimo-token-no-prefix-abcdef01"
	m := chatModelForConfigPasteTest()
	m.configOpen = true
	m.configTab = configTabGateways
	next, _ := m.startConfigKeyForProvider("xiaomi_mimo_payg")
	next.configInput.SetValue(secret)
	if got := strings.TrimSpace(next.configInput.Value()); got != secret {
		t.Fatalf("input value = %q, want %q", got, secret)
	}

	next, saveCmd := next.finishConfigEntry()
	if saveCmd == nil {
		t.Fatal("expected async save command")
	}
	if !next.configSaving {
		t.Fatalf("expected saving state, notice=%q entry=%q provider=%q", next.configNotice, next.configEntry, next.configProvider)
	}

	msg := saveCmd()
	applyMsg, ok := msg.(configApplyCredentialsMsg)
	if !ok {
		t.Fatalf("expected configApplyCredentialsMsg, got %T", msg)
	}
	if applyMsg.err == nil {
		t.Fatal("expected probe failure from mocked transport")
	}
	if !strings.Contains(applyMsg.err.Error(), "key saved in keychain") {
		t.Fatalf("expected persisted key hint: %v", applyMsg.err)
	}
	if !credentials.HasSecret(context.Background(), "XIAOMI_MIMO_PAYG_API_KEY") {
		t.Fatal("key should be in store after save")
	}

	final, _ := m.handleConfigApplyCredentialsMsg(applyMsg)
	if !strings.Contains(strings.ToLower(final.configNotice), "key saved") {
		t.Fatalf("expected key-saved notice, got %q", final.configNotice)
	}
	rows := final.configGatewayRows()
	for _, row := range rows {
		if row.ID == "xiaomi_mimo_payg" && !row.HasKey {
			t.Fatalf("xiaomi_mimo_payg row should show HasKey after save, row=%+v", row)
		}
	}
	if final.configTab != configTabGateways {
		t.Fatalf("tab = %d, want gateways", final.configTab)
	}
}

func TestFinishConfigEntry_APIKeyPaste_EmptyCancels(t *testing.T) {
	m := chatModelForConfigPasteTest()
	m.configOpen = true
	next, _ := m.startConfigKeyForProvider("openrouter")
	next, cmd := next.finishConfigEntry()
	if cmd != nil {
		t.Fatal("expected no cmd for empty key")
	}
	if next.configEntry != configEntryNone {
		t.Fatalf("entry = %q, want none", next.configEntry)
	}
	if !strings.Contains(next.configNotice, "No API key entered") {
		t.Fatalf("expected empty-key notice, got %q", next.configNotice)
	}
}

func TestConfigGatewaysSelect_AddKeyOpensPaste(t *testing.T) {
	m := chatModelForConfigPasteTest()
	m.configTab = configTabGateways
	m.configSel = 0
	rows := m.configGatewayRows()
	if len(rows) == 0 {
		t.Fatal("no gateway rows")
	}
	// Find first gateway without a key.
	sel := -1
	for i, row := range rows {
		if !row.HasKey && row.ID != configProviderOllama {
			sel = i
			break
		}
	}
	if sel < 0 {
		t.Skip("all gateways already have keys in this environment") // TODO: https://github.com/GrayCodeAI/hawk/issues/28
	}
	m.configSel = sel
	next, _ := m.handleConfigGatewaysSelect()
	if next.configEntry != configEntryAPIKeyPaste {
		t.Fatalf("expected paste entry, got %q", next.configEntry)
	}
	if next.configProvider != rows[sel].ID {
		t.Fatalf("provider = %q, want %q", next.configProvider, rows[sel].ID)
	}
}

func configSaveTestTransport(status int) http.RoundTripper {
	return roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: status,
			Body:       http.NoBody,
			Header:     make(http.Header),
			Request:    req,
		}, nil
	})
}

// roundTripFunc is defined in eyrie/runtime tests; duplicate here for hawk cmd tests.
type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestHandleConfigKey_EnterOnPasteSubmits(t *testing.T) {
	hawkconfig.InvalidateConfigUICache()
	store := &credentials.MapStore{}
	credentials.SetDefaultStore(store)
	t.Cleanup(func() {
		credentials.SetDefaultStore(nil)
		hawkconfig.InvalidateConfigUICache()
	})
	oldTransport := http.DefaultTransport
	http.DefaultTransport = configSaveTestTransport(http.StatusUnauthorized)
	t.Cleanup(func() { http.DefaultTransport = oldTransport })

	m := chatModelForConfigPasteTest()
	m.configOpen = true
	m.configTab = configTabGateways
	next, _ := m.startConfigKeyForProvider("openrouter")
	next.configInput.SetValue("sk-or-test-key-12345678901234567890")
	next, cmd := next.handleConfigKey(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected save cmd on enter")
	}
	if next.configEntry != configEntryNone {
		t.Fatalf("entry should close on submit, got %q", next.configEntry)
	}
}
