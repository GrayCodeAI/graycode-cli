package cmd

import (
	"strings"
	"testing"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
)

func TestConfigVisibleRowsExpandsWithTerminalHeight(t *testing.T) {
	short := chatModel{height: 30, configTab: configTabGateways}.configVisibleRows()
	tall := chatModel{height: 60, configTab: configTabGateways}.configVisibleRows()
	if tall <= short {
		t.Fatalf("expected tall terminal to show more rows: short=%d tall=%d", short, tall)
	}
	want := 60 - (chatModel{configTab: configTabGateways}).configFixedChromeRows()
	if tall != want {
		t.Fatalf("tall gateway rows = %d, want %d", tall, want)
	}
}

func TestConfigUsesFullTerminalHeight(t *testing.T) {
	m := chatModel{height: 42, configOpen: true}
	m.viewport = viewport.New(viewport.WithWidth(100), viewport.WithHeight(5))
	m = m.withSyncedLayout()
	if got := m.viewport.Height(); got != 42 {
		t.Fatalf("config viewport height = %d, want 42", got)
	}
}

func TestConfigMouseClickSelectsVisibleGatewayRow(t *testing.T) {
	rows := []configGatewayRow{
		{ID: "one", DisplayName: "One"},
		{ID: "two", DisplayName: "Two"},
		{ID: "three", DisplayName: "Three"},
	}
	m := chatModel{
		height:                 30,
		width:                  100,
		configOpen:             true,
		configTab:              configTabGateways,
		configGatewayRowsCache: rows,
	}
	m.viewport = viewport.New(viewport.WithWidth(100), viewport.WithHeight(30))
	click := tea.MouseClickMsg{Y: m.configFirstVisibleRowY() + 1, Button: tea.MouseLeft}
	next, handled := m.handleConfigMouse(click)
	if !handled || next.configSel != 1 || next.configGatewayFocus != 1 {
		t.Fatalf("gateway click handled=%v selection=%d focus=%d", handled, next.configSel, next.configGatewayFocus)
	}
}

func TestConfigMouseClickSelectsVisibleModelRowAfterChrome(t *testing.T) {
	m := chatModel{
		height:              30,
		width:               100,
		configOpen:          true,
		configTab:           configTabModels,
		configModelProvider: "poolside",
		configModelOptions: []configModelOption{
			{ID: "poolside/one", DisplayName: "One"},
			{ID: "poolside/two", DisplayName: "Two"},
		},
	}
	m.viewport = viewport.New(viewport.WithWidth(100), viewport.WithHeight(30))
	click := tea.MouseClickMsg{Y: m.configFirstVisibleRowY() + 1, Button: tea.MouseLeft}
	next, handled := m.handleConfigMouse(click)
	if !handled || next.configSel != 1 {
		t.Fatalf("model click handled=%v selection=%d", handled, next.configSel)
	}
}

func TestConfigVisibleRowsAccountsForNoticeLines(t *testing.T) {
	base := chatModel{height: 50, configTab: configTabGateways}.configVisibleRows()
	withNotice := chatModel{
		height:       50,
		configTab:    configTabGateways,
		configNotice: "first line\nsecond line",
	}.configVisibleRows()
	if base-withNotice != 3 {
		t.Fatalf("two-line notice should consume three rows including spacing: base=%d notice=%d", base, withNotice)
	}
}

func TestConfigVisibleRowsReservesMoreModelChrome(t *testing.T) {
	gateways := chatModel{height: 50, configTab: configTabGateways}.configVisibleRows()
	models := chatModel{
		height:              50,
		configTab:           configTabModels,
		configModelProvider: "poolside",
		configModelOptions:  []configModelOption{{ID: "poolside/model"}},
	}.configVisibleRows()
	if models >= gateways {
		t.Fatalf("models should reserve space for gateway/search rows: gateways=%d models=%d", gateways, models)
	}
}

func TestConfigVisibleRowsFallbackAndMinimum(t *testing.T) {
	if got := (chatModel{}).configVisibleRows(); got != configDefaultVisibleRows {
		t.Fatalf("unknown-height rows = %d, want %d", got, configDefaultVisibleRows)
	}
	if got := (chatModel{height: 8}).configVisibleRows(); got != configMinVisibleRows {
		t.Fatalf("small-terminal rows = %d, want %d", got, configMinVisibleRows)
	}
}

func TestConfigPageStepTracksVisibleRows(t *testing.T) {
	m := chatModel{height: 60, configTab: configTabModels}
	if got, want := m.configPageStep(), m.configVisibleRows()-1; got != want {
		t.Fatalf("page step = %d, want %d", got, want)
	}
}

func TestTallGatewayPanelClampsStaleScrollOffset(t *testing.T) {
	rows := make([]configGatewayRow, 22)
	for i := range rows {
		rows[i] = configGatewayRow{ID: "gateway", DisplayName: "Gateway"}
	}
	rows[0].DisplayName = "First Gateway"
	m := chatModel{
		height:                 60,
		width:                  160,
		configTab:              configTabGateways,
		configScroll:           10,
		configSel:              15,
		configGatewayRowsCache: rows,
	}
	view := m.configGatewaysView()
	if !strings.Contains(view, "First Gateway") {
		t.Fatalf("expected stale scroll to clamp and reveal first row, got:\n%s", view)
	}
}

func TestConfigViewportPinsOuterScrollToTop(t *testing.T) {
	vp := viewport.New(viewport.WithWidth(100), viewport.WithHeight(20))
	vp.SetContent(strings.Repeat("old chat\n", 80))
	vp.SetYOffset(30)
	m := chatModel{
		viewport:               vp,
		width:                  100,
		height:                 30,
		contentLines:           200,
		configOpen:             true,
		configTab:              configTabGateways,
		viewDirty:              true,
		configGatewayRowsCache: []configGatewayRow{{ID: "poolside", DisplayName: "Poolside"}},
	}
	m.updateViewportContent()
	if !m.viewport.AtTop() {
		t.Fatalf("config outer viewport retained chat offset %d", m.viewport.YOffset())
	}
	if m.contentLines >= 200 {
		t.Fatalf("config retained stale chat content line count %d", m.contentLines)
	}
	if !strings.Contains(m.viewport.View(), "Gateway") {
		t.Fatalf("expected sticky config header in viewport, got:\n%s", m.viewport.View())
	}
}
