// Package icons is hawk's centralized icon registry.
//
// Hawk never emits emoji or one-off Unicode glyphs in its CLI/TUI output.
// Every glyph hawk renders goes through this package. Each glyph has two
// forms:
//
//   - A Nerd Font PUA (Private Use Area) codepoint (U+E000–U+F8FF). These
//     render as designed icons when the user has any Nerd Font patched
//     font installed (https://www.nerdfonts.com).
//   - An ASCII fallback. The default. Used on any terminal that does not
//     have a patched font, when stdout is not a TTY (e.g. CI logs,
//     captured output), or when the user sets HAWK_ICONS=ascii.
//
// No emoji block codepoints (U+1F300–U+1FAFF) and no symbol/dingbat block
// codepoints (U+2600–U+27BF) are used anywhere. The codepoints defined in
// this file are verified to be in the Unicode PUA by the audit test in
// internal/testaudit.
package icons

// Nerd Font PUA codepoints. Names match the Nerd Fonts cheat sheet
// (https://www.nerdfonts.com/cheat-sheet). All entries are in the basic
// Private Use Area (U+E000–U+F8FF).
const (
	// Material Design Icons
	puaChevronRight   = "\ue5cc" // nf-md-chevron_right
	puaRobot          = "\ue244" // nf-md-robot
	puaCircleMedium   = "\uf444" // nf-md-circle_medium (filled)
	puaCircle         = "\uf4a7" // nf-md-circle         (outline)
	puaAlert          = "\ue002" // nf-md-alert
	puaCheckBold      = "\ue5ca" // nf-md-check_bold
	puaCloseThick     = "\ue5cd" // nf-md-close_thick
	puaArrowRight     = "\ue5c8" // nf-md-arrow_right
	puaArrowLeft      = "\ue5c4" // nf-md-arrow_left
	puaArrowUp        = "\ue5d8" // nf-md-arrow_up
	puaArrowDown      = "\ue5db" // nf-md-arrow_down
	puaSwapHorizontal = "\ue5d5" // nf-md-swap_horizontal
	puaTimerSand      = "\uf51f" // nf-md-timer_sand
	puaReload         = "\uf045" // nf-md-reload
	puaStop           = "\uf04a" // nf-md-stop
	puaBell           = "\uf009" // nf-md-bell
	puaCancel         = "\uf015" // nf-md-cancel
	puaCheckDecagram  = "\uf079" // nf-md-check_decagram
	puaAlertOctagram  = "\ueb27" // nf-md-alert_octagram
	puaArrowUpBold    = "\ue5d9" // nf-md-arrow_up_bold  (return/enter key)

	// Codicons
	puaFile     = "\uea7b" // nf-cod-file
	puaQuestion = "\uea11" // nf-cod-question
	puaMail     = "\uea8c" // nf-cod-mail
)
