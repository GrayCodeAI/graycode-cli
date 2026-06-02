package cmd

import (
	"fmt"
	"math/rand"
	"strings"
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
	SpinnerRandom      SpinnerStyle = "random"
)

// hawkQuadBlockGlyphs is the unicode.framer.website QUADBLOCK spinner (4 frames).
var hawkQuadBlockGlyphs = []string{"▛", "▜", "▟", "▙"}

// hawkSpinnerBG is the chat viewport background (chat_view.go) — palette is tuned for this.
var hawkSpinnerBG = [3]int{30, 30, 40}

// hawkRandomPalette — 20 natural colors for spinner + verbs on dark bg. No orange
// (hawk accent #FF5E0E is used elsewhere in the TUI).
var hawkRandomPalette = [][3]int{
	{78, 205, 196},  // teal
	{80, 210, 200},  // aqua
	{100, 225, 200}, // mint
	{120, 210, 185}, // seafoam
	{150, 205, 160}, // sage
	{175, 220, 130}, // lime
	{225, 235, 110}, // lemon
	{235, 205, 90},  // gold
	{110, 190, 240}, // sky
	{140, 160, 235}, // cornflower
	{150, 165, 240}, // periwinkle
	{140, 150, 225}, // indigo
	{190, 165, 240}, // lavender
	{175, 145, 235}, // violet
	{210, 145, 235}, // orchid
	{235, 130, 170}, // rose
	{245, 150, 175}, // blush
	{210, 145, 195}, // mauve
	{235, 115, 195}, // fuchsia
	{70, 200, 165},  // emerald
}

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
}

const (
	hawkSpinnerANSI  = "\033[38;2;255;94;14m"
	hawkSpinnerReset = "\033[0m"
)

func randomHawkColor() [3]int {
	return hawkRandomPalette[rand.Intn(len(hawkRandomPalette))]
}

func colorHawkRGB(rgb [3]int, text string) string {
	if text == "" {
		return ""
	}
	return fmt.Sprintf("\033[38;2;%d;%d;%dm%s\033[0m", rgb[0], rgb[1], rgb[2], text)
}

func colorSpinnerGlyph(glyph string) string {
	if glyph == "" {
		return ""
	}
	return hawkSpinnerANSI + glyph + hawkSpinnerReset
}

// BrailleSpinner renders animated braille spinners with shimmer text.
type BrailleSpinner struct {
	mu         sync.Mutex
	style      SpinnerStyle
	frames     []string
	frame      int
	text       string
	glyphColor [3]int
	labelColor [3]int
	wave       bool // when true, label is colored as a moving wave
	dots       int  // 0..2 — position of the highlighted dot in the trailing animation
	running    bool
	stopCh     chan struct{}
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
	s := &BrailleSpinner{
		style:  style,
		frames: frames,
		text:   text,
		stopCh: make(chan struct{}),
	}
	if style == SpinnerHawk {
		s.glyphColor = randomHawkColor()
		s.labelColor = randomHawkColor()
	}
	return s
}

func (s *BrailleSpinner) refreshGlyphColorLocked() {
	s.glyphColor = randomHawkColor()
}

// SetLabel updates spinner label text and picks a fresh random label color.
func (s *BrailleSpinner) SetLabel(text string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.text = text
	if s.style == SpinnerHawk {
		s.labelColor = randomHawkColor()
	}
}

// SetWave enables or disables color-wave animation on the label. When on,
// each character in the label is colored from a rotating slice of the
// hawk palette, producing a visible wave that moves with each tick.
func (s *BrailleSpinner) SetWave(on bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.wave = on
}

func (s *BrailleSpinner) renderGlyphLocked(glyph string) string {
	if s.style == SpinnerHawk {
		return colorHawkRGB(s.glyphColor, glyph)
	}
	return colorSpinnerGlyph(glyph)
}

// renderWaveLabel colors each character of text with a per-position color
// drawn from hawkRandomPalette, shifted by the current frame so the
// bright peak slides across the word.
func (s *BrailleSpinner) renderWaveLabelLocked(text string) string {
	if text == "" {
		return ""
	}
	const reset = "\033[0m"
	var b strings.Builder
	palette := len(hawkRandomPalette)
	for i, r := range text {
		c := hawkRandomPalette[(i*3+s.frame)%palette]
		fmt.Fprintf(&b, "\033[38;2;%d;%d;%dm%c", c[0], c[1], c[2], r)
	}
	b.WriteString(reset)
	return b.String()
}

// renderAnimatedDots returns three dots where one is filled and the others
// are dim, cycling through positions 0..2 with each tick. Used after the
// verb to show that work is ongoing.
func (s *BrailleSpinner) renderAnimatedDotsLocked() string {
	const reset = "\033[0m"
	dim := "\033[2m"
	// Bright dot uses hawk orange so it pops against the muted dots.
	const peakR, peakG, peakB = 255, 94, 14 // matches hawkColor
	highlightIdx := s.dots % 3
	var b strings.Builder
	for i := 0; i < 3; i++ {
		glyph := "○"
		if i == highlightIdx {
			glyph = "●"
		}
		if i == highlightIdx {
			fmt.Fprintf(&b, "\033[38;2;%d;%d;%dm%s", peakR, peakG, peakB, glyph)
		} else {
			fmt.Fprintf(&b, "%s%s", dim, glyph)
		}
	}
	b.WriteString(reset)
	return b.String()
}

func (s *BrailleSpinner) renderLabelLocked() string {
	if s.text == "" {
		return ""
	}
	var rendered string
	if s.wave {
		rendered = s.renderWaveLabelLocked(s.text)
	} else if s.style == SpinnerHawk {
		rendered = colorHawkRGB(s.labelColor, s.text)
	} else {
		rendered = colorSpinnerGlyph(s.text)
	}
	// Trailing animated dots ride with every Hawk spinner in wave mode.
	if s.wave {
		rendered += " " + s.renderAnimatedDotsLocked()
	}
	return rendered
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

// Tick advances to the next frame and picks a fresh random glyph color.
func (s *BrailleSpinner) Tick() string {
	s.mu.Lock()
	s.frame++
	s.dots = (s.dots + 1) % 3
	if s.style == SpinnerHawk {
		s.refreshGlyphColorLocked()
	}
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

// renderShimmer colors a label with natural welcome RGB.
func renderShimmer(text string, _ int) string {
	if text == "" {
		return ""
	}
	return colorHawkRGB(randomHawkColor(), text)
}
