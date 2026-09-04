package icons

// ASCII fallback strings. Every glyph in the registry has one. Defaults
// are single ASCII runes or short bracketed tokens ("[ok]", "[!!]") for
// state indicators that need more visibility than a single character.
const (
	ASCIIPullRequest   = "[pr]"
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
	ASCIIRefresh       = "[r]"
	ASCIIHourglass     = "[..]"
	ASCIICheckCircle   = "[ok]"
	ASCIICloseCircle   = "[x]"
	ASCIIImage         = "[img]"
	ASCIIFileDocument  = "[doc]"
	ASCIIKey           = "[key]"
	ASCIICog           = "[*]"
	ASCIIMagnify       = "/"
	ASCIIBolt          = "!"
	ASCIIBrain         = "[think]"
	ASCIIEmail         = "[m]"
	ASCIIHelpCircle    = "?"
	ASCIIBranch        = "(|)"
	ASCIIClockOutline  = "[t]"
	ASCIIPause         = "[||]"
	ASCIIExpandAll     = "[+]"
	ASCIIContainer     = "[ctr]"
	ASCIINetwork       = "[net]"
	ASCIIShield        = "[safe]"
	ASCIITerminal      = ">_"
	ASCIICaretRight    = ">"
	ASCIICaretDown     = "v"
	ASCIITriangleSmall = ">"
	ASCIICircleHalf    = "o"
	ASCIICircleQuarter = "."
	ASCIICircleSlice5  = "."
	ASCIICircleSlice6  = "."
	ASCIIRotateVariant = "[r]"
	ASCIILlama         = "[ollama]"
)

// registry maps the human-friendly glyph name to (Nerd, ASCII) pair.
// Order is also the order returned by Names() and used by graycode harness
// for its icons section. Keep grouped by family.
var registry = []struct {
	name  string
	nerd  string
	ascii string
}{
	{"chevron_right", puaChevronRight, ASCIIPrompt},
	{"robot", puaRobot, ASCIIRobot},
	{"circle_filled", puaCircleFilled, ASCIICircleFilled},
	{"circle_outline", puaCircleOutline, ASCIICircleOutline},
	{"alert", puaAlert, ASCIIAlert},
	{"check_bold", puaCheckBold, ASCIICheck},
	{"close_thick", puaCloseThick, ASCIICross},
	{"ballot_x", puaCloseThick, ASCIIBallotX},
	{"block", puaCircleFilled, ASCIIBlock},
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
	{"refresh", puaRefresh, ASCIIRefresh},
	{"hourglass", puaHourglass, ASCIIHourglass},
	{"check_circle", puaCheckCircle, ASCIICheckCircle},
	{"close_circle", puaCloseCircle, ASCIICloseCircle},
	{"image", puaImage, ASCIIImage},
	{"file_document", puaFileDocument, ASCIIFileDocument},
	{"key", puaKey, ASCIIKey},
	{"cog", puaCog, ASCIICog},
	{"magnify", puaMagnify, ASCIIMagnify},
	{"bolt", puaBolt, ASCIIBolt},
	{"brain", puaBrain, ASCIIBrain},
	{"email", puaEmail, ASCIIEmail},
	{"help_circle", puaHelpCircle, ASCIIHelpCircle},
	{"branch", puaBranch, ASCIIBranch},
	{"pull_request", puaPullRequest, ASCIIPullRequest},
	{"clock_outline", puaClockOutline, ASCIIClockOutline},
	{"pause", puaPause, ASCIIPause},
	{"expand_all", puaExpandAll, ASCIIExpandAll},
	{"container", puaContainer, ASCIIContainer},
	{"network", puaNetwork, ASCIINetwork},
	{"shield", puaShield, ASCIIShield},
	{"terminal", puaTerminal, ASCIITerminal},
	{"caret_right", puaCaretRight, ASCIICaretRight},
	{"caret_down", puaCaretDown, ASCIICaretDown},
	{"triangle_small", puaTriangleSmall, ASCIITriangleSmall},
	{"circle_half", puaCircleHalf, ASCIICircleHalf},
	{"circle_quarter", puaCircleQuarter, ASCIICircleQuarter},
	{"circle_slice_5", puaCircleSlice5, ASCIICircleSlice5},
	{"circle_slice_6", puaCircleSlice6, ASCIICircleSlice6},
	{"rotate_variant", puaRotateVariant, ASCIIRotateVariant},
	{"llama", puaLlama, ASCIILlama},
	{"check", puaCheckBold, ASCIICheck},
	{"close", puaCloseThick, ASCIICross},
	{"pin", puaPin, "[pin]"},
	{"database", puaDatabase, "[db]"},
	{"ruby", puaRuby, "$"},
}

func lookup(name string) (string, string, bool) {
	for _, e := range registry {
		if e.name == name {
			return e.nerd, e.ascii, true
		}
	}
	return "", "", false
}

// Glyph returns the active-mode glyph for the given name. Returns "" for
// unknown names (a programmer error, but not worth crashing the TUI).
func Glyph(name string) string {
	nerd, ascii, ok := lookup(name)
	if !ok {
		return ""
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
		return ""
	}
	return ascii
}

// Nerd returns the Nerd Font PUA form for the given name regardless of
// the active mode. Used by the harness command's "icons" section.
func Nerd(name string) string {
	nerd, _, ok := lookup(name)
	if !ok {
		return ""
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
func Refresh() string       { return Glyph("refresh") }
func Hourglass() string     { return Glyph("hourglass") }
func CheckCircle() string   { return Glyph("check_circle") }
func CloseCircle() string   { return Glyph("close_circle") }
func Image() string         { return Glyph("image") }
func FileDocument() string  { return Glyph("file_document") }
func Key() string           { return Glyph("key") }
func Cog() string           { return Glyph("cog") }
func Magnify() string       { return Glyph("magnify") }
func Bolt() string          { return Glyph("bolt") }
func Brain() string         { return Glyph("brain") }
func Email() string         { return Glyph("email") }
func HelpCircle() string    { return Glyph("help_circle") }
func Branch() string        { return Glyph("branch") }
func PullRequest() string   { return Glyph("pull_request") }
func ClockOutline() string  { return Glyph("clock_outline") }
func Pause() string         { return Glyph("pause") }
func ExpandAll() string     { return Glyph("expand_all") }
func Container() string     { return Glyph("container") }
func Network() string       { return Glyph("network") }
func Shield() string        { return Glyph("shield") }
func Terminal() string      { return Glyph("terminal") }
func CaretRight() string    { return Glyph("caret_right") }
func CaretDown() string     { return Glyph("caret_down") }
func TriangleSmall() string { return Glyph("triangle_small") }
func CircleHalf() string    { return Glyph("circle_half") }
func CircleQuarter() string { return Glyph("circle_quarter") }
func CircleSlice5() string  { return Glyph("circle_slice_5") }
func CircleSlice6() string  { return Glyph("circle_slice_6") }
func Check() string         { return Glyph("check") }
func Close() string         { return Glyph("close") }
func RotateVariant() string { return Glyph("rotate_variant") }
func Llama() string         { return Glyph("llama") }
func Pin() string           { return Glyph("pin") }
func Database() string      { return Glyph("database") }
func Ruby() string          { return Glyph("ruby") }
