package cmd

import (
	"math/rand"
	"sync"
	"time"
)

// SpinnerStyle names an animation style.
type SpinnerStyle string

const (
	SpinnerBraille     SpinnerStyle = "braille"
	SpinnerBrailleWave SpinnerStyle = "braillewave"
	SpinnerHawk        SpinnerStyle = "hawk"
	SpinnerDNA         SpinnerStyle = "dna"
	SpinnerScan        SpinnerStyle = "scan"
	SpinnerPulse       SpinnerStyle = "pulse"
	SpinnerSnake       SpinnerStyle = "snake"
	SpinnerOrbit       SpinnerStyle = "orbit"
	SpinnerWing        SpinnerStyle = "wing"   // ⫷⫸ — two-frame wing flap
	SpinnerTalons      SpinnerStyle = "talons" // ⩤⩥⩦⩧ — four-frame talon cycle
	SpinnerRandom      SpinnerStyle = "random"
)

// hawkQuadBlockGlyphs is the unicode.framer.website QUADBLOCK spinner (4 frames).
var hawkQuadBlockGlyphs = []string{"▛", "▜", "▟", "▙"}

// hawkSpinnerBG is the chat viewport background (chat_view.go).
var hawkSpinnerBG = [3]int{30, 30, 40}

// spinnerFrames maps style names to their animation frames.
var spinnerFrames = map[SpinnerStyle][]string{
	SpinnerBraille:     {"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"},
	SpinnerBrailleWave: {"⠁⠂⠄⡀", "⠂⠄⡀⢀", "⠄⡀⢀⠠", "⡀⢀⠠⠐", "⢀⠠⠐⠈", "⠠⠐⠈⠁", "⠐⠈⠁⠂", "⠈⠁⠂⠄"},
	SpinnerHawk:        hawkQuadBlockGlyphs,
	SpinnerDNA:         {"⠋⠉⠙⠚", "⠉⠙⠚⠒", "⠙⠚⠒⠂", "⠚⠒⠂⠂", "⠒⠂⠂⠒", "⠂⠂⠒⠲", "⠂⠒⠲⠴", "⠒⠲⠴⠤", "⠲⠴⠤⠄", "⠴⠤⠄⠋", "⠤⠄⠋⠉", "⠄⠋⠉⠙"},
	SpinnerScan:        {"⡇⠀⠀⠀", "⣿⠀⠀⠀", "⢸⡇⠀⠀", "⠀⣿⠀⠀", "⠀⢸⡇⠀", "⠀⠀⣿⠀", "⠀⠀⢸⡇", "⠀⠀⠀⣿", "⠀⠀⠀⢸", "⠀⠀⠀⠀"},
	SpinnerPulse:       {"⠀", "⠄", "⠆", "⠇", "⡇", "⣇", "⣧", "⣷", "⣿", "⣷", "⣧", "⣇", "⡇", "⠇", "⠆", "⠄"},
	SpinnerSnake:       {"⠈⠁", "⠈⠑", "⠈⠱", "⠈⡱", "⢁⡱", "⢁⡰", "⢁⡠", "⢁⡀", "⢁⠀", "⠁⠀"},
	SpinnerOrbit:       {"⢄", "⢂", "⢁", "⡁", "⡈", "⡐", "⡠", "⣀", "⢠", "⢐", "⢈", "⢁"},
	SpinnerWing:        {"⫷", "⫸"},
	SpinnerTalons:      {"⩤", "⩥", "⩦", "⩧"},
}

// hawkTypingDots is the number of trailing typing-indicator dots.
const hawkTypingDots = 3

// colorSpinnerGlyph renders a single glyph in hawk brand orange — the
// spinner is the visual hero of the line and the brand color. The ANSI
// escape constants (ansiOrange, ansiReset, etc.) and the icon glyphs
// (iconDotFilled, iconDotEmpty) live in theme.go so the entire palette
// is editable from one place.
func colorSpinnerGlyph(glyph string) string {
	if glyph == "" {
		return ""
	}
	return ansiOrange + glyph + ansiReset
}

// BrailleSpinner renders animated spinners with a single accent color
// (cyan) for both the glyph and the label.
type BrailleSpinner struct {
	mu      sync.Mutex
	style   SpinnerStyle
	frames  []string
	frame   int
	text    string
	dots    int // 0..hawkTypingDots-1 — position of the highlighted dot
	running bool
	stopCh  chan struct{}
}

// NewBrailleSpinner creates a spinner with the given style and label text.
func NewBrailleSpinner(style SpinnerStyle, text string) *BrailleSpinner {
	if style == SpinnerRandom {
		styles := []SpinnerStyle{SpinnerHawk, SpinnerBraille, SpinnerBrailleWave, SpinnerDNA, SpinnerScan, SpinnerPulse, SpinnerSnake, SpinnerOrbit}
		style = styles[rand.Intn(len(styles))]
	}
	frames := spinnerFrames[style]
	if frames == nil {
		frames = spinnerFrames[SpinnerBraille]
	}
	return &BrailleSpinner{
		style:  style,
		frames: frames,
		text:   text,
		stopCh: make(chan struct{}),
	}
}

// SetLabel updates spinner label text.
func (s *BrailleSpinner) SetLabel(text string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.text = text
}

// SetWave is kept for backwards compatibility with existing call sites —
// the line no longer uses a color wave so this is a no-op.
func (s *BrailleSpinner) SetWave(_ bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
}

func (s *BrailleSpinner) renderGlyphLocked(glyph string) string {
	return colorSpinnerGlyph(glyph)
}

// renderLabelLocked returns the label in green followed by the trailing
// animated dots (one yellow dot, two dim dots). The whole group is the
// "alive" part of the spinner line.
func (s *BrailleSpinner) renderLabelLocked() string {
	if s.text == "" {
		return ""
	}
	out := ansiGreen + s.text + ansiReset
	out += " " + s.renderAnimatedDotsLocked()
	return out
}

// renderAnimatedDotsLocked returns hawkTypingDots plain circles, with the
// current position rendered in yellow and the rest dim. The filled and
// empty glyphs come from theme.go (iconDotFilled / iconDotEmpty) so they
// stay in sync with the rest of the TUI.
func (s *BrailleSpinner) renderAnimatedDotsLocked() string {
	idx := s.dots % hawkTypingDots
	out := ""
	for i := 0; i < hawkTypingDots; i++ {
		if i == idx {
			out += ansiYellow + iconDotFilled + ansiReset
		} else {
			out += ansiDim + iconDotEmpty + ansiReset
		}
	}
	return out
}

// Frame returns the current rendered frame (spinner + label).
func (s *BrailleSpinner) Frame() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	glyph := s.frames[s.frame%len(s.frames)]
	spinner := s.renderGlyphLocked(glyph)
	label := s.renderLabelLocked()
	if label == "" {
		return spinner
	}
	return spinner + "  " + label
}

// Tick advances to the next frame and cycles the dot highlight.
func (s *BrailleSpinner) Tick() string {
	s.mu.Lock()
	s.frame++
	s.dots = (s.dots + 1) % hawkTypingDots
	s.mu.Unlock()
	return s.Frame()
}

// Start begins auto-advancing the spinner. Call Stop() to end.
// NOTE: Not used in TUI mode (Tick() is called manually per frame).
// Kept for non-TUI contexts (daemon, CLI progress bars).
func (s *BrailleSpinner) Start(interval time.Duration, render func(string)) {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return
	}
	s.running = true
	s.mu.Unlock()

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-s.stopCh:
				return
			case <-ticker.C:
				render(s.Tick())
			}
		}
	}()
}

// Stop halts the spinner.
func (s *BrailleSpinner) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.running {
		close(s.stopCh)
		s.running = false
	}
}
