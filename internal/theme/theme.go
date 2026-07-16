// theme.go — Hawk's theme system.
//
// Every color in the TUI is defined here as a palette struct. To change
// a theme, edit theme_palettes.go. To add a new theme, add a palette and
// register it in themeRegistry.
//
// The Theme struct holds the resolved palette. buildTheme (in theme.go)
// converts a palette to the final Theme with resolved AdaptiveColors.

package theme

import (
	lipgloss "charm.land/lipgloss/v2"
	"charm.land/lipgloss/v2/compat"
)

// Theme holds all resolved theme colors and styles for the TUI.
type Theme struct {
	// Brand identity
	BrandColor lipgloss.Style

	// UI state
	PanelBg    lipgloss.Style
	PromptBg   lipgloss.Style
	LineColor  lipgloss.Style
	LineColor2 lipgloss.Style

	// Semantic feedback
	SuccessColor lipgloss.Style
	WarnColor    lipgloss.Style
	ErrorColor   lipgloss.Style
	InfoColor    lipgloss.Style

	// Tooling & agents
	ToolColor      lipgloss.Style
	AgentColor     lipgloss.Style
	DoneColor      lipgloss.Style
	ContainerColor lipgloss.Style

	// Autonomy tiers
	TierInspect lipgloss.Style
	TierEdit    lipgloss.Style
	TierRun     lipgloss.Style
	TierTrust   lipgloss.Style

	// HUD overlay
	HudBorderColor lipgloss.Style
	HudLabelColor  lipgloss.Style

	// Status bar
	CostColor   lipgloss.Style
	BranchColor lipgloss.Style
	TokenColor  lipgloss.Style
	CwdColor    lipgloss.Style

	// Text hierarchy (AdaptiveColor for dark/light variant)
	TextPrimary     compat.AdaptiveColor
	TextMuted       compat.AdaptiveColor
	TextPlaceholder lipgloss.Style
	TextDisabled    compat.AdaptiveColor
	TextWhite       lipgloss.Style

	// Structure
	BorderColor compat.AdaptiveColor
	BgCode      lipgloss.Style

	// Diff colors
	DiffAddBg     lipgloss.Style
	DiffDelBg     lipgloss.Style
	DiffAddBgWord lipgloss.Style
	DiffDelBgWord lipgloss.Style

	// Permission
	PermBg     lipgloss.Style
	PermBgWord lipgloss.Style
	SelBg      lipgloss.Style

	// On-accent for text on brand backgrounds
	OnBrandColor  lipgloss.Style
	OnAccentColor lipgloss.Style

	// Card colors for specialist cards
	CardRun  lipgloss.Style
	CardErr  lipgloss.Style
	CardPerm lipgloss.Style

	// Git colors
	GitAdd       lipgloss.Style
	GitDel       lipgloss.Style
	GitAddBg     lipgloss.Style
	GitDelBg     lipgloss.Style
	GitAddBgWord lipgloss.Style
	GitDelBgWord lipgloss.Style

	// Status line colors (raw ANSI for hot path)
	AnsiBrand   string
	AnsiSuccess string
	AnsiWarn    string
	AnsiError   string
	AnsiInfo    string
	AnsiDim     string
	AnsiReset   string
}

// Palette holds raw hex color values for a theme.
// All palettes must be hex strings (e.g., "#FF5E0E").
type Palette struct {
	Panel     string `json:"panel,omitempty"`
	PromptBg  string `json:"prompt_bg,omitempty"`
	Line      string `json:"line,omitempty"`
	Line2     string `json:"line2,omitempty"`
	Ink       string `json:"ink,omitempty"`
	Muted     string `json:"muted,omitempty"`
	Faint     string `json:"faint,omitempty"`
	Faintest  string `json:"faintest,omitempty"`
	Accent    string `json:"accent,omitempty"`
	Green     string `json:"green,omitempty"`
	Red       string `json:"red,omitempty"`
	Amber     string `json:"amber,omitempty"`
	Blue      string `json:"blue,omitempty"`
	GitAdd    string `json:"git_add,omitempty"`
	GitDel    string `json:"git_del,omitempty"`
	AddBg     string `json:"add_bg,omitempty"`
	DelBg     string `json:"del_bg,omitempty"`
	AddBgWord string `json:"add_bg_word,omitempty"`
	DelBgWord string `json:"del_bg_word,omitempty"`
	PermBg    string `json:"perm_bg,omitempty"`
	SelBg     string `json:"sel_bg,omitempty"`
	AddInk    string `json:"add_ink,omitempty"`
	DelInk    string `json:"del_ink,omitempty"`
	OnAccent  string `json:"on_accent,omitempty"`
	CardRun   string `json:"card_run,omitempty"`
	CardErr   string `json:"card_err,omitempty"`
	CardPerm  string `json:"card_perm,omitempty"`
}

// ScrollConfig holds scroll behavior settings.
type ScrollConfig struct {
	Speed          int // 1-100 (default 50)
	Mode           string // auto, wheel, trackpad
	Invert         bool // natural scrolling
	LinesPerTick   int // lines per scroll event (default 5)
	ScrollbackLines int // max buffer lines (0 = unlimited)
}

// PagerConfig holds pager/scrollback settings.
type PagerConfig struct {
	// Scrollback buffer settings
	BufferLines int // 0 = use viewport default
	// Layout settings
	Margins         PageMargins
	ShowLineNumbers bool
	LineNumbers     string // ANSI color code for line numbers
}

// PageMargins controls layout spacing.
type PageMargins struct {
	Top    int // top margin/padding
	Right  int // right margin
	Bottom int // bottom margin
	Left   int // left margin
}

// DisplayConfig holds display mode settings.
type DisplayConfig struct {
	CompactMode bool
	VimMode     bool
}

// UISettings holds user-configurable UI preferences (persisted in settings.json).
type UISettings struct {
	Theme       string `json:"theme,omitempty"`
	ScrollSpeed int    `json:"scroll_speed,omitempty"`
	ScrollMode  string `json:"scroll_mode,omitempty"`
	InvertScroll bool  `json:"invert_scroll,omitempty"`
	CompactMode bool   `json:"compact_mode,omitempty"`
	VimMode     bool   `json:"vim_mode,omitempty"`
	AutoDarkTheme string `json:"auto_dark_theme,omitempty"`
	AutoLightTheme string `json:"auto_light_theme,omitempty"`
}
