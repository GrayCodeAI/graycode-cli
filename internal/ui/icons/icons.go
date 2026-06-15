package icons

// ASCII fallback strings. Every glyph in the registry has one. Defaults
// are single ASCII runes or short bracketed tokens ("[ok]", "[!!]") for
// state indicators that need more visibility than a single character.
const (
	ASCIIPrompt        = ">"
	ASCIIRobot         = "*"
	ASCIICircleFilled  = "*"
	ASCIICircleOutline = "."
	ASCIIAlert         = "!"
	ASCIICheck         = "+"
	ASCIICross         = "x"
	ASCIIBallotX       = "x"
	ASCIIBlock         = "#"
	ASCIIReturn        = "<-"
	ASCIIArrowRight    = "->"
	ASCIIArrowLeft     = "<-"
	ASCIIArrowUp       = "^"
	ASCIIArrowDown     = "v"
	ASCIISwap          = "<->"
	ASCIITimer         = "[t]"
	ASCIIReload        = "[r]"
	ASCIIStop          = "[x]"
	ASCIIBell          = "[!]"
	ASCIICheckDecagram = "[ok]"
	ASCIIAlertOctagram = "[!!]"
	ASCIICancel        = "[x]"
	ASCIIFile          = "[f]"
	ASCIIQuestion      = "?"
	ASCIIMail          = "[m]"
)

// registry maps the human-friendly glyph name to (Nerd, ASCII) pair.
// Order is also the order returned by Names() and used by hawk harness
// for its icons section. Keep grouped by family.
var registry = []struct {
	name  string
	nerd  string
	ascii string
}{
	{"chevron_right", puaChevronRight, ASCIIPrompt},
	{"robot", puaRobot, ASCIIRobot},
	{"circle_filled", puaCircleMedium, ASCIICircleFilled},
	{"circle_outline", puaCircle, ASCIICircleOutline},
	{"alert", puaAlert, ASCIIAlert},
	{"check_bold", puaCheckBold, ASCIICheck},
	{"close_thick", puaCloseThick, ASCIICross},
	{"ballot_x", puaCloseThick, ASCIIBallotX},
	{"block", puaCircleMedium, ASCIIBlock},
	{"return", puaArrowUpBold, ASCIIReturn},
	{"arrow_right", puaArrowRight, ASCIIArrowRight},
	{"arrow_left", puaArrowLeft, ASCIIArrowLeft},
	{"arrow_up", puaArrowUp, ASCIIArrowUp},
	{"arrow_down", puaArrowDown, ASCIIArrowDown},
	{"swap", puaSwapHorizontal, ASCIISwap},
	{"timer", puaTimerSand, ASCIITimer},
	{"reload", puaReload, ASCIIReload},
	{"stop", puaStop, ASCIIStop},
	{"bell", puaBell, ASCIIBell},
	{"cancel", puaCancel, ASCIICancel},
	{"check_decagram", puaCheckDecagram, ASCIICheckDecagram},
	{"alert_octagram", puaAlertOctagram, ASCIIAlertOctagram},
	{"file", puaFile, ASCIIFile},
	{"question", puaQuestion, ASCIIQuestion},
	{"mail", puaMail, ASCIIMail},
}

func lookup(name string) (string, string, bool) {
	for _, e := range registry {
		if e.name == name {
			return e.nerd, e.ascii, true
		}
	}
	return "", "", false
}

// Glyph returns the active-mode glyph for the given name. Panics on
// unknown names — that is a programmer error, not a runtime condition.
func Glyph(name string) string {
	nerd, ascii, ok := lookup(name)
	if !ok {
		panic("icons: unknown glyph " + name)
	}
	if Mode() == ModeNerd {
		return nerd
	}
	return ascii
}

// ASCII returns the ASCII fallback for the given name regardless of the
// active mode. Use this in:
//   - golden-file tests (stable byte-for-byte expected output)
//   - log lines that may be shipped to a non-tty destination later
//   - the TUI when it detects it is being rendered to a captured file
func ASCII(name string) string {
	_, ascii, ok := lookup(name)
	if !ok {
		panic("icons: unknown glyph " + name)
	}
	return ascii
}

// Nerd returns the Nerd Font PUA form for the given name regardless of
// the active mode. Used by the harness command's "icons" section.
func Nerd(name string) string {
	nerd, _, ok := lookup(name)
	if !ok {
		panic("icons: unknown glyph " + name)
	}
	return nerd
}

// Names returns all registered glyph names in display order.
func Names() []string {
	out := make([]string, len(registry))
	for i, e := range registry {
		out[i] = e.name
	}
	return out
}

// --- Typed convenience accessors -----------------------------------------
//
// Callers should rarely need Glyph("chevron_right"). These typed
// functions give the audit test a single name to grep for.

func ChevronRight() string  { return Glyph("chevron_right") }
func Robot() string         { return Glyph("robot") }
func CircleFilled() string  { return Glyph("circle_filled") }
func CircleOutline() string { return Glyph("circle_outline") }
func Alert() string         { return Glyph("alert") }
func CheckBold() string     { return Glyph("check_bold") }
func CloseThick() string    { return Glyph("close_thick") }
func BallotX() string       { return Glyph("ballot_x") }
func Block() string         { return Glyph("block") }
func Return() string        { return Glyph("return") }
func ArrowRight() string    { return Glyph("arrow_right") }
func ArrowLeft() string     { return Glyph("arrow_left") }
func ArrowUp() string       { return Glyph("arrow_up") }
func ArrowDown() string     { return Glyph("arrow_down") }
func Swap() string          { return Glyph("swap") }
func Timer() string         { return Glyph("timer") }
func Reload() string        { return Glyph("reload") }
func Stop() string          { return Glyph("stop") }
func Bell() string          { return Glyph("bell") }
func Cancel() string        { return Glyph("cancel") }
func CheckDecagram() string { return Glyph("check_decagram") }
func AlertOctagram() string { return Glyph("alert_octagram") }
func File() string          { return Glyph("file") }
func Question() string      { return Glyph("question") }
func Mail() string          { return Glyph("mail") }
