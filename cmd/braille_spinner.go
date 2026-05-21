package cmd

import (
	"fmt"
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

func (s *BrailleSpinner) renderGlyphLocked(glyph string) string {
	if s.style == SpinnerHawk {
		return colorHawkRGB(s.glyphColor, glyph)
	}
	return colorSpinnerGlyph(glyph)
}

func (s *BrailleSpinner) renderLabelLocked() string {
	if s.text == "" {
		return ""
	}
	if s.style == SpinnerHawk {
		return colorHawkRGB(s.labelColor, s.text)
	}
	return colorSpinnerGlyph(s.text)
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
