// Package kitty implements the Kitty graphics protocol for terminal images.
//
// It provides:
//   - capability response parsing (whether the terminal supports Kitty
//     graphics), and
//   - chunked frame encoding that emits the APC graphics transmission sequence.
//
// The package is self-contained and unit-testable; wiring the encoded frames
// into a terminal render loop (e.g. the differential renderer in internal/tui/
// diff) is the documented follow-up.
package kitty

import (
	"encoding/base64"
	"fmt"
	"strings"
)

// Format identifies the raster encoding used in a frame transmission.
type Format int

const (
	// FormatRGB is a raw 24-bit RGB raster.
	FormatRGB Format = 24
	// FormatRGBA is a raw 32-bit RGBA raster.
	FormatRGBA Format = 32
	// FormatPNG is a PNG-compressed image.
	FormatPNG Format = 100
)

// DefaultChunkSize bounds the base64 payload per transmission chunk, keeping
// individual writes small enough for terminals to buffer without truncation.
const DefaultChunkSize = 4096

// responseMarker is the APC wrapper that terminates a capability query/response.
const (
	apcStart = "\x1b_G"
	apcEnd   = "\x1b\\"
)

// ParseCapabilityResponse reports whether a terminal's capability response
// advertises Kitty graphics support. The terminal replies to the query
// `\x1b_Gi=31,s=1,v=1,m=0` with a `\x1b_Gi=31;...` response; support is
// indicated by `OK` plus `s=1` (transmission) and `v=1` (version 1).
func ParseCapabilityResponse(data string) bool {
	if !strings.Contains(data, "i=31") {
		return false
	}
	if !strings.Contains(data, "OK") {
		return false
	}
	// Kitty responds with `s=<supported>`, `v=<version>`, `a=<abort>`. A
	// supported terminal reports s=1 (and usually v=1); a 1 is a positive
	// capability flag regardless of the version number.
	return strings.Contains(data, "s=1")
}

// EncodeFrame renders one image frame as a chunked Kitty graphics transmission
// sequence. data is the raster bytes (raw RGB/RGBA or PNG depending on format),
// width/height its dimensions. The returned string is ready to write to the
// terminal.
func EncodeFrame(f Format, width, height int, data []byte) string {
	payload := base64.StdEncoding.EncodeToString(data)
	var b strings.Builder
	first := true
	for len(payload) > 0 {
		chunk := payload
		if len(chunk) > DefaultChunkSize {
			chunk = chunk[:DefaultChunkSize]
		}
		payload = payload[len(chunk):]

		// m=1 signals more chunks follow; m=0 marks the final chunk.
		m := 0
		if len(payload) > 0 {
			m = 1
		}
		b.WriteString(apcStart)
		if first {
			fmt.Fprintf(&b, "a=T,f=%d,s=%d,v=%d,m=%d;", int(f), width, height, m)
			first = false
		} else {
			fmt.Fprintf(&b, "m=%d;", m)
		}
		b.WriteString(chunk)
		b.WriteString(apcEnd)
	}
	return b.String()
}
