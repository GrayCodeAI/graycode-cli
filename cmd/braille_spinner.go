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
	SpinnerHawkQuad    SpinnerStyle = "hawkquad"
	SpinnerDNA         SpinnerStyle = "dna"
	SpinnerScan        SpinnerStyle = "scan"
	SpinnerPulse       SpinnerStyle = "pulse"
	SpinnerSnake       SpinnerStyle = "snake"
	SpinnerOrbit       SpinnerStyle = "orbit"
	SpinnerWing        SpinnerStyle = "wing"   // ⫷⫸ — two-frame wing flap
	SpinnerTalons      SpinnerStyle = "talons" // ⩤⩥⩦⩧ — four-frame talon cycle
	SpinnerRandom      SpinnerStyle = "random"
)

// hawkSpinnerGlyphs is the default TUI spinner — partial-circle compass (smooth, readable).
var hawkSpinnerGlyphs = []string{"◐", "◓", "◑", "◒"}

// hawkQuadBlockGlyphs is the legacy QUADBLOCK animation (kept for tests / bubbles compat).
var hawkQuadBlockGlyphs = []string{"▛", "▜", "▟", "▙"}

// spinnerFrames maps style names to their animation frames.
var spinnerFrames = map[SpinnerStyle][]string{
	SpinnerBraille:     {"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"},
	SpinnerBrailleWave: {"⠁⠂⠄⡀", "⠂⠄⡀⢀", "⠄⡀⢀⠠", "⡀⢀⠠⠐", "⢀⠠⠐⠈", "⠠⠐⠈⠁", "⠐⠈⠁⠂", "⠈⠁⠂⠄"},
	SpinnerHawk:        hawkSpinnerGlyphs,
	SpinnerHawkQuad:    hawkQuadBlockGlyphs,
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

// BrailleSpinner renders the glyph frame (◐◓◑◒) and a 20-color wave on the
// whole status strip: glyph → verb → ▪▫▫.
type BrailleSpinner struct {
	mu        sync.Mutex
	style     SpinnerStyle
	frames    []string
	frame     int // glyph animation frame (mod len(frames))
	wavePhase int // 0..19 flowing color wave (glyph + verb + dots)
	text      string
	dots      int // 0..hawkTypingDots-1 — position of the highlighted dot
	running   bool
	stopCh    chan struct{}
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

// SetWave is kept for backwards compatibility with existing call sites.
// The verb and typing dots always use the 20-color wave; this is a no-op.
func (s *BrailleSpinner) SetWave(_ bool) {}

// GlyphChar returns the current spinner glyph only (no verb or typing dots).
func (s *BrailleSpinner) GlyphChar() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.frames[s.frame%len(s.frames)]
}

// Frame returns the current rendered frame (glyph + verb + dots, all in the wave).
func (s *BrailleSpinner) Frame() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return renderSpinnerWaveLine(s.frames[s.frame%len(s.frames)], s.text, s.wavePhase, s.dots)
}

// Tick advances to the next frame and cycles the dot highlight.
func (s *BrailleSpinner) Tick() string {
	s.mu.Lock()
	s.frame++
	s.wavePhase = (s.wavePhase + 1) % spinnerWaveLen
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
