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
	SpinnerBraille      SpinnerStyle = "braille"
	SpinnerBrailleWave  SpinnerStyle = "braillewave"
	SpinnerDNA          SpinnerStyle = "dna"
	SpinnerScan         SpinnerStyle = "scan"
	SpinnerPulse        SpinnerStyle = "pulse"
	SpinnerSnake        SpinnerStyle = "snake"
	SpinnerOrbit        SpinnerStyle = "orbit"
	SpinnerRandom       SpinnerStyle = "random"
)

// spinnerFrames maps style names to their animation frames.
var spinnerFrames = map[SpinnerStyle][]string{
	SpinnerBraille:     {"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"},
	SpinnerBrailleWave: {"⠁⠂⠄⡀", "⠂⠄⡀⢀", "⠄⡀⢀⠠", "⡀⢀⠠⠐", "⢀⠠⠐⠈", "⠠⠐⠈⠁", "⠐⠈⠁⠂", "⠈⠁⠂⠄"},
	SpinnerDNA:         {"⠋⠉⠙⠚", "⠉⠙⠚⠒", "⠙⠚⠒⠂", "⠚⠒⠂⠂", "⠒⠂⠂⠒", "⠂⠂⠒⠲", "⠂⠒⠲⠴", "⠒⠲⠴⠤", "⠲⠴⠤⠄", "⠴⠤⠄⠋", "⠤⠄⠋⠉", "⠄⠋⠉⠙"},
	SpinnerScan:        {"⡇⠀⠀⠀", "⣿⠀⠀⠀", "⢸⡇⠀⠀", "⠀⣿⠀⠀", "⠀⢸⡇⠀", "⠀⠀⣿⠀", "⠀⠀⢸⡇", "⠀⠀⠀⣿", "⠀⠀⠀⢸", "⠀⠀⠀⠀"},
	SpinnerPulse:       {"⠀", "⠄", "⠆", "⠇", "⡇", "⣇", "⣧", "⣷", "⣿", "⣷", "⣧", "⣇", "⡇", "⠇", "⠆", "⠄"},
	SpinnerSnake:       {"⠈⠁", "⠈⠑", "⠈⠱", "⠈⡱", "⢁⡱", "⢁⡰", "⢁⡠", "⢁⡀", "⢁⠀", "⠁⠀"},
	SpinnerOrbit:       {"⢄", "⢂", "⢁", "⡁", "⡈", "⡐", "⡠", "⣀", "⢠", "⢐", "⢈", "⢁"},
}

// shimmerColors is a gradient for the text shimmer effect (256-color).
var shimmerColors = []string{"255", "219", "213", "200", "141"}

// BrailleSpinner renders animated braille spinners with shimmer text.
type BrailleSpinner struct {
	mu       sync.Mutex
	style    SpinnerStyle
	frames   []string
	frame    int
	text     string
	running  bool
	stopCh   chan struct{}
}

// NewBrailleSpinner creates a spinner with the given style and label text.
func NewBrailleSpinner(style SpinnerStyle, text string) *BrailleSpinner {
	if style == SpinnerRandom {
		styles := []SpinnerStyle{SpinnerBraille, SpinnerBrailleWave, SpinnerDNA, SpinnerScan, SpinnerPulse, SpinnerSnake, SpinnerOrbit}
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

// Frame returns the current rendered frame (spinner + shimmer text).
func (s *BrailleSpinner) Frame() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	spinner := s.frames[s.frame%len(s.frames)]
	shimmer := renderShimmer(s.text, s.frame)
	return fmt.Sprintf("%s %s", spinner, shimmer)
}

// Tick advances to the next frame. Returns the rendered string.
func (s *BrailleSpinner) Tick() string {
	s.mu.Lock()
	s.frame++
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

// renderShimmer applies a sweeping brightness gradient across text.
func renderShimmer(text string, frame int) string {
	runes := []rune(text)
	if len(runes) == 0 {
		return ""
	}
	var sb strings.Builder
	gradLen := len(shimmerColors)
	for i, r := range runes {
		// Calculate which gradient position this character is at
		pos := (frame + i) % (len(runes) + gradLen)
		var color string
		if pos < gradLen {
			color = shimmerColors[pos]
		} else {
			color = shimmerColors[0] // default bright
		}
		sb.WriteString(fmt.Sprintf("\033[38;5;%sm%c\033[0m", color, r))
	}
	return sb.String()
}
