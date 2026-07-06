// theme.go — Hawk's theme system.
//
// Every color in the TUI is defined here as a palette struct. To change
// a theme, edit theme_palettes.go. To add a new theme, add a palette and
// register it in themeRegistry.
//
// The Theme struct holds the resolved palette. buildTheme (in theme.go)
// converts a palette to the final Theme with resolved AdaptiveColors.

package theme

import "github.com/charmbracelet/lipgloss"

// Theme holds all resolved theme colors and styles for the TUI.
type Theme struct {
	// Brand identity
	BrandColor lipgloss.Color

	// UI state
	PanelBg         lipgloss.Color
	PromptBg        lipgloss.Color
	LineColor       lipgloss.Color
	LineColor2      lipgloss.Color

	// Semantic feedback
	SuccessColor lipgloss.Color
	WarnColor    lipgloss.Color
	ErrorColor   lipgloss.Color
	InfoColor    lipgloss.Color

	// Tooling & agents
	ToolColor lipgloss.Color
	AgentColor lipgloss.Color
	DoneColor  lipgloss.Color
	ContainerColor lipgloss.Color

	// Autonomy tiers
	TierInspect lipgloss.Color
	TierEdit    lipgloss.Color
	TierRun     lipgloss.Color
	TierTrust   lipgloss.Color

	// HUD overlay
	HudBorderColor lipgloss.Color
	HudLabelColor  lipgloss.Color

	// Status bar
	CostColor  lipgloss.Color
	BranchColor lipgloss.Color
	TokenColor lipgloss.Color
	CwdColor   lipgloss.Color

	// Text hierarchy (AdaptiveColor for dark/light variant)
	TextPrimary   lipgloss.AdaptiveColor
	TextMuted     lipgloss.AdaptiveColor
	TextPlaceholder lipgloss.Color
	TextDisabled  lipgloss.AdaptiveColor
	TextWhite     lipgloss.Color

	// Structure
	BorderColor lipgloss.AdaptiveColor
	BgCode      lipgloss.Color

	// Diff colors
	DiffAddBg   lipgloss.Color
	DiffDelBg   lipgloss.Color
	DiffAddBgWord lipgloss.Color
	DiffDelBgWord lipgloss.Color

	// Permission
	PermBg  lipgloss.Color
	PermBgWord lipgloss.Color
	SelBg   lipgloss.Color

	// On-accent for text on brand backgrounds
	OnBrandColor lipgloss.Color
	OnAccentColor lipgloss.Color

	// Card colors for specialist cards
	CardRun  lipgloss.Color
	CardErr  lipgloss.Color
	CardPerm lipgloss.Color

	// Git colors
	GitAdd  lipgloss.Color
	GitDel  lipgloss.Color
	GitAddBg lipgloss.Color
	GitDelBg lipgloss.Color
	GitAddBgWord lipgloss.Color
	GitDelBgWord lipgloss.Color

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
	Panel       string `json:"panel,omitempty"`
	PromptBg    string `json:"prompt_bg,omitempty"`
	Line        string `json:"line,omitempty"`
	Line2       string `json:"line2,omitempty"`
	Ink         string `json:"ink,omitempty"`
	Muted       string `json:"muted,omitempty"`
	Faint       string `json:"faint,omitempty"`
	Faintest    string `json:"faintest,omitempty"`
	Accent      string `json:"accent,omitempty"`
	Green       string `json:"green,omitempty"`
	Red         string `json:"red,omitempty"`
	Amber       string `json:"amber,omitempty"`
	Blue        string `json:"blue,omitempty"`
	GitAdd      string `json:"git_add,omitempty"`
	GitDel      string `json:"git_del,omitempty"`
	AddBg       string `json:"add_bg,omitempty"`
	DelBg       string `json:"del_bg,omitempty"`
	AddBgWord   string `json:"add_bg_word,omitempty"`
	DelBgWord   string `json:"del_bg_word,omitempty"`
	PermBg      string `json:"perm_bg,omitempty"`
	SelBg       string `json:"sel_bg,omitempty"`
	AddInk      string `json:"add_ink,omitempty"`
	DelInk      string `json:"del_ink,omitempty"`
	OnAccent    string `json:"on_accent,omitempty"`
	CardRun     string `json:"card_run,omitempty"`
	CardErr     string `json:"card_err,omitempty"`
	CardPerm    string `json:"card_perm,omitempty"`
}

// resolve converts a Palette to a Theme with resolved colors.
func (p Palette) resolve(brandColor lipgloss.Color) Theme {
	return Theme{
		BrandColor:     brandColor,
		PanelBg:        lipgloss.Color(p.Panel),
		PromptBg:       lipgloss.Color(p.PromptBg),
		LineColor:      lipgloss.Color(p.Line),
		LineColor2:     lipgloss.Color(p.Line2),
		SuccessColor:   lipgloss.Color(p.Green),
		ErrorColor:     lipgloss.Color(p.Red),
		WarnColor:      lipgloss.Color(p.Amber),
		InfoColor:      lipgloss.Color(p.Blue),
		ToolColor:      lipgloss.Color(p.GitAdd),
		AgentColor:     lipgloss.Color(p.Accent),
		DoneColor:      lipgloss.Color(p.Green),
		ContainerColor: lipgloss.Color(p.Blue),
		TierInspect:    lipgloss.Color(p.Blue),
		TierEdit:       lipgloss.Color(p.Green),
		TierRun:        lipgloss.Color(p.Amber),
		TierTrust:      lipgloss.Color(p.Red),
		HudBorderColor: lipgloss.Color(p.Accent),
		HudLabelColor:  lipgloss.Color(p.Accent),
		CostColor:      lipgloss.Color(p.Accent),
		BranchColor:    lipgloss.Color(p.Amber),
		TokenColor:     lipgloss.Color(p.Green),
		CwdColor:       lipgloss.Color(p.Blue),
		TextPrimary:    lipgloss.AdaptiveColor{Light: p.Ink, Dark: "#F0F0F0"},
		TextMuted:      lipgloss.AdaptiveColor{Light: p.Muted, Dark: p.Muted},
		TextPlaceholder: lipgloss.Color(p.Faint),
		TextDisabled:   lipgloss.AdaptiveColor{Light: p.Faint, Dark: p.Faint},
		TextWhite:      lipgloss.Color("#FFFFFF"),
		BorderColor:    lipgloss.AdaptiveColor{Light: p.Faint, Dark: p.Faint},
		BgCode:         lipgloss.Color(p.Panel),
		DiffAddBg:      lipgloss.Color(p.AddBg),
		DiffDelBg:      lipgloss.Color(p.DelBg),
		DiffAddBgWord:  lipgloss.Color(p.AddBgWord),
		DiffDelBgWord:  lipgloss.Color(p.DelBgWord),
		PermBg:         lipgloss.Color(p.PermBg),
		SelBg:          lipgloss.Color(p.SelBg),
		OnAccentColor:  lipgloss.Color(p.OnAccent),
		CardRun:        lipgloss.Color(p.CardRun),
		CardErr:        lipgloss.Color(p.CardErr),
		CardPerm:       lipgloss.Color(p.CardPerm),
		GitAdd:         lipgloss.Color(p.GitAdd),
		GitDel:         lipgloss.Color(p.GitDel),
		GitAddBg:       lipgloss.Color(p.AddBg),
		GitDelBg:       lipgloss.Color(p.DelBg),
		GitAddBgWord:   lipgloss.Color(p.AddBgWord),
		GitDelBgWord:   lipgloss.Color(p.DelBgWord),
		AnsiBrand:      "\033[38;2;255;94;14m",
		AnsiSuccess:    "\033[38;2;78;205;196m",
		AnsiWarn:       "\033[38;2;255;179;71m",
		AnsiError:      "\033[38;2;255;107;107m",
		AnsiInfo:       "\033[38;2;117;177;226m",
		AnsiDim:        "\033[2m",
		AnsiReset:      "\033[0m",
	}
}
