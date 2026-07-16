package theme

import (
	"testing"
)

func TestDetectColorLevel(t *testing.T) {
	// Just ensure it returns a valid value - actual detection depends on environment
	level := DetectColorLevel()
	if level < ColorBasic || level > ColorTruecolor {
		t.Errorf("DetectColorLevel() = %d, want 1-3", level)
	}
}

func TestHexToRGB(t *testing.T) {
	tests := []struct {
		hex     string
		r, g, b int
		wantErr bool
	}{
		{"#FF0000", 255, 0, 0, false},
		{"#00FF00", 0, 255, 0, false},
		{"#0000FF", 0, 0, 255, false},
		{"#FFFFFF", 255, 255, 255, false},
		{"#000000", 0, 0, 0, false},
		{"invalid", 0, 0, 0, true},
		{"#FFF", 0, 0, 0, true}, // invalid length
	}

	for _, tt := range tests {
		t.Run(tt.hex, func(t *testing.T) {
			r, g, b, err := HexToRGB(tt.hex)
			if tt.wantErr {
				if err == nil {
					t.Errorf("HexToRGB(%q) expected error, got none", tt.hex)
				}
			} else {
				if err != nil {
					t.Errorf("HexToRGB(%q) unexpected error: %v", tt.hex, err)
				}
				if r != tt.r || g != tt.g || b != tt.b {
					t.Errorf("HexToRGB(%q) = (%d, %d, %d), want (%d, %d, %d)", tt.hex, r, g, b, tt.r, tt.g, tt.b)
				}
			}
		})
	}
}

func TestRGBTo256(t *testing.T) {
	tests := []struct {
		r, g, b int
		idx     int // expected index range
	}{
		{255, 0, 0, 196},     // Red
		{0, 255, 0, 22},      // Green
		{0, 0, 255, 27},      // Blue
		{255, 255, 255, 231}, // White (top of cube)
	}

	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			idx := RGBTo256(tt.r, tt.g, tt.b)
			if idx < 16 || idx > 231 {
				t.Errorf("RGBTo256(%d, %d, %d) = %d, want 16-231", tt.r, tt.g, tt.b, idx)
			}
		})
	}
}

func TestRGBToANSI(t *testing.T) {
	tests := []struct {
		r, g, b int
	}{
		{255, 0, 0},     // Red - should map to ANSI red
		{0, 255, 0},     // Green
		{0, 0, 255},     // Blue
		{255, 255, 255}, // White
		{0, 0, 0},       // Black
	}

	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			ansi := RGBToANSI(tt.r, tt.g, tt.b)
			if ansi < 0 || ansi > 15 {
				t.Errorf("RGBToANSI(%d, %d, %d) = %d, want 0-15", tt.r, tt.g, tt.b, ansi)
			}
		})
	}
}

func TestANSIToRGB(t *testing.T) {
	tests := []struct {
		ansi    int
		r, g, b int
	}{
		{0, 0, 0, 0},        // Black
		{7, 192, 192, 192},  // Bright gray
		{15, 255, 255, 255}, // Bright white
		{16, 0, 0, 0},       // Out of range
	}

	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			r, g, b := ANSIToRGB(tt.ansi)
			if r != tt.r || g != tt.g || b != tt.b {
				t.Errorf("ANSIToRGB(%d) = (%d, %d, %d), want (%d, %d, %d)", tt.ansi, r, g, b, tt.r, tt.g, tt.b)
			}
		})
	}
}

func TestQuantizePalette(t *testing.T) {
	// Test truecolor - no change
	p := &Palette{
		Panel:  "#0e0e10",
		Accent: "#FF5E0E",
		Ink:    "#ececee",
		Green:  "#5dd1a4",
		Red:    "#ff7a7a",
	}

	result := QuantizePalette(p, ColorTruecolor)
	if result.Panel != "#0e0e10" {
		t.Errorf("Truecolor mode should not change colors, got %s", result.Panel)
	}

	// Test 256-color
	result = QuantizePalette(p, Color256)
	if result.Panel == "" {
		t.Error("256-color mode should produce valid colors")
	}

	// Test 16-color
	result = QuantizePalette(p, ColorBasic)
	if result.Panel == "" {
		t.Error("16-color mode should produce valid colors")
	}
}

func TestIdxToRGB(t *testing.T) {
	tests := []struct {
		idx     int
		r, g, b int
	}{
		{16, 0, 0, 0},        // First color cube entry
		{231, 255, 255, 255}, // Last color cube entry (white)
		{232, 8, 8, 8},       // First grayscale
		{255, 238, 238, 238}, // Last grayscale
		{0, 0, 0, 0},         // Out of range
	}

	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			r, g, b := idxToRGB(tt.idx)
			if r != tt.r || g != tt.g || b != tt.b {
				t.Errorf("idxToRGB(%d) = (%d, %d, %d), want (%d, %d, %d)", tt.idx, r, g, b, tt.r, tt.g, tt.b)
			}
		})
	}
}
