// quantize.go — Color quantization for terminals.
//
// Maps truecolor (24-bit) to 256-color or 16-color palettes.

package theme

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// ColorLevel indicates terminal color capability.
type ColorLevel int

const (
	ColorBasic   ColorLevel = 1  // 16-color ANSI
	Color256     ColorLevel = 2  // 256-color
	ColorTruecolor ColorLevel = 3 // 24-bit RGB
)

// DetectColorLevel returns the terminal's color capability.
func DetectColorLevel() ColorLevel {
	// Check COLORTERM first (most reliable)
	if ct := os.Getenv("COLORTERM"); strings.Contains(strings.ToLower(ct), "truecolor") {
		return ColorTruecolor
	}
	// Check TERM for 256-color
	if term := os.Getenv("TERM"); strings.Contains(term, "256color") {
		return Color256
	}
	return ColorBasic
}

// HexToRGB parses a hex color to RGB components.
func HexToRGB(hex string) (r, g, b int, err error) {
	hex = strings.TrimPrefix(hex, "#")
	if len(hex) != 6 {
		return 0, 0, 0, fmt.Errorf("invalid hex length")
	}
	ri, _ := strconv.ParseInt(hex[0:2], 16, 0)
	gi, _ := strconv.ParseInt(hex[2:4], 16, 0)
	bi, _ := strconv.ParseInt(hex[4:6], 16, 0)
	return int(ri), int(gi), int(bi), nil
}

// RGBTo256 maps RGB to closest 256-color index.
// Uses the 6x6x6 color cube plus grayscale ramp.
func RGBTo256(r, g, b int) int {
	// Convert to 0-5 range for color cube
	rr := r / 51 // 0-5
	gg := g / 51
	bb := b / 51

	// Color cube: 16-231
	idx := 16 + (36 * rr) + (6 * gg) + bb
	return idx
}

// sq returns the square of x (helper for color distance).
func sq(x int) int {
	return x * x
}

// RGBToANSI maps RGB to closest ANSI color (16-color).
func RGBToANSI(r, g, b int) int {
	colors := []int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15}
	// Find the closest ANSI color by Euclidean distance
	best := 0
	bestDist := 1 << 30
	for _, c := range colors {
		tr, tg, tb := ANSIToRGB(c)
		dist := sq(r-tr) + sq(g-tg) + sq(b-tb)
		if dist < bestDist {
			bestDist = dist
			best = c
		}
	}
	return best
}

// ANSIToRGB returns RGB for standard ANSI color.
func ANSIToRGB(ansi int) (r, g, b int) {
	// Standard 16 ANSI colors
	standard := []struct{ r, g, b int }{
		{0, 0, 0}, {128, 0, 0}, {0, 128, 0}, {128, 128, 0},
		{0, 0, 128}, {128, 0, 128}, {0, 128, 128}, {192, 192, 192},
		{128, 128, 128}, {255, 0, 0}, {0, 255, 0}, {255, 255, 0},
		{0, 0, 255}, {255, 0, 255}, {0, 255, 255}, {255, 255, 255},
	}
	if ansi >= 0 && ansi < 16 {
		return standard[ansi].r, standard[ansi].g, standard[ansi].b
	}
	return 0, 0, 0
}

// QuantizePalette converts a truecolor palette to terminal-safe colors.
// For 256-color terminals, RGB values are mapped to the nearest indexed color.
// For 16-color terminals, colors map to ANSI names.
func QuantizePalette(p *Palette, level ColorLevel) *Palette {
	if level == ColorTruecolor {
		return p // No change needed
	}

	// Quantize each color field in the palette
	quantizeHex := func(hex string) string {
		r, g, b, err := HexToRGB(hex)
		if err != nil {
			return hex
		}
		if level == Color256 {
			// Map to 256-color index and return as hex approximation
			idx := RGBTo256(r, g, b)
			ir, ig, ib := idxToRGB(idx)
			return fmt.Sprintf("#%02X%02X%02X", ir, ig, ib)
		}
		// Map to closest ANSI color
		ansi := RGBToANSI(r, g, b)
		ar, ag, ab := ANSIToRGB(ansi)
		return fmt.Sprintf("#%02X%02X%02X", ar, ag, ab)
	}

	// Quantize all color fields
	quantized := &Palette{}
	*quantized = *p // Copy all fields first

	// Apply quantization to each hex field
	if p.Panel != "" {
		quantized.Panel = quantizeHex(p.Panel)
	}
	if p.PromptBg != "" {
		quantized.PromptBg = quantizeHex(p.PromptBg)
	}
	if p.Line != "" {
		quantized.Line = quantizeHex(p.Line)
	}
	if p.Line2 != "" {
		quantized.Line2 = quantizeHex(p.Line2)
	}
	if p.Ink != "" {
		quantized.Ink = quantizeHex(p.Ink)
	}
	if p.Muted != "" {
		quantized.Muted = quantizeHex(p.Muted)
	}
	if p.Faint != "" {
		quantized.Faint = quantizeHex(p.Faint)
	}
	if p.Faintest != "" {
		quantized.Faintest = quantizeHex(p.Faintest)
	}
	if p.Accent != "" {
		quantized.Accent = quantizeHex(p.Accent)
	}
	if p.Green != "" {
		quantized.Green = quantizeHex(p.Green)
	}
	if p.Red != "" {
		quantized.Red = quantizeHex(p.Red)
	}
	if p.Amber != "" {
		quantized.Amber = quantizeHex(p.Amber)
	}
	if p.Blue != "" {
		quantized.Blue = quantizeHex(p.Blue)
	}
	if p.GitAdd != "" {
		quantized.GitAdd = quantizeHex(p.GitAdd)
	}
	if p.GitDel != "" {
		quantized.GitDel = quantizeHex(p.GitDel)
	}
	if p.AddBg != "" {
		quantized.AddBg = quantizeHex(p.AddBg)
	}
	if p.DelBg != "" {
		quantized.DelBg = quantizeHex(p.DelBg)
	}
	if p.AddBgWord != "" {
		quantized.AddBgWord = quantizeHex(p.AddBgWord)
	}
	if p.DelBgWord != "" {
		quantized.DelBgWord = quantizeHex(p.DelBgWord)
	}
	if p.PermBg != "" {
		quantized.PermBg = quantizeHex(p.PermBg)
	}
	if p.SelBg != "" {
		quantized.SelBg = quantizeHex(p.SelBg)
	}
	if p.AddInk != "" {
		quantized.AddInk = quantizeHex(p.AddInk)
	}
	if p.DelInk != "" {
		quantized.DelInk = quantizeHex(p.DelInk)
	}
	if p.OnAccent != "" {
		quantized.OnAccent = quantizeHex(p.OnAccent)
	}
	if p.CardRun != "" {
		quantized.CardRun = quantizeHex(p.CardRun)
	}
	if p.CardErr != "" {
		quantized.CardErr = quantizeHex(p.CardErr)
	}
	if p.CardPerm != "" {
		quantized.CardPerm = quantizeHex(p.CardPerm)
	}

	return quantized
}

// idxToRGB converts a 256-color index to approximate RGB values.
// Indices 16-231 are the 6x6x6 color cube; 232-255 are grayscale.
func idxToRGB(idx int) (r, g, b int) {
	if idx >= 16 && idx <= 231 {
		// Color cube
		rr := ((idx - 16) / 36) * 51
		gg := (((idx - 16) % 36) / 6) * 51
		bb := ((idx - 16) % 6) * 51
		return rr, gg, bb
	}
	if idx >= 232 && idx <= 255 {
		// Grayscale ramp
		v := (idx - 232) * 10 + 8
		return v, v, v
	}
	return 0, 0, 0
}