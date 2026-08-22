package kitty

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestParseCapabilityResponseSupported(t *testing.T) {
	if !ParseCapabilityResponse("\x1b_Gi=31;OK,s=1,v=1,m=0") {
		t.Fatal("supported response must be recognized")
	}
}

func TestParseCapabilityResponseUnsupported(t *testing.T) {
	cases := []string{
		"\x1b_Gi=31;NO",
		"\x1b_Gi=31;OK,s=0,v=1,m=0",
		"no kitty response",
		"",
	}
	for _, c := range cases {
		if ParseCapabilityResponse(c) {
			t.Fatalf("response %q must not be recognized as supported", c)
		}
	}
}

func TestEncodeFrameSingleChunk(t *testing.T) {
	// A tiny payload fits in one chunk.
	data := []byte{1, 2, 3, 4}
	out := EncodeFrame(FormatRGBA, 2, 2, data)
	if !strings.HasPrefix(out, apcStart) || !strings.HasSuffix(out, apcEnd) {
		t.Fatal("frame must be APC-wrapped")
	}
	if !strings.Contains(out, "a=T") {
		t.Fatal("frame must use transmit action")
	}
	if !strings.Contains(out, "f=32") || !strings.Contains(out, "s=2") || !strings.Contains(out, "v=2") {
		t.Fatalf("frame must carry format/size metadata, got %q", out)
	}
	if !strings.Contains(out, "m=0;") {
		t.Fatal("final chunk must set m=0")
	}
}

func TestEncodeFrameChunks(t *testing.T) {
	// A payload larger than DefaultChunkSize forces multiple chunks.
	data := make([]byte, DefaultChunkSize*2+100)
	out := EncodeFrame(FormatRGB, 10, 10, data)
	if strings.Count(out, apcStart) < 2 {
		t.Fatalf("large frame must be split into multiple chunks, got %d APC starts", strings.Count(out, apcStart))
	}
	if !strings.Contains(out, "m=1;") {
		t.Fatal("intermediate chunks must set m=1")
	}
	if !strings.Contains(out, "m=0;") {
		t.Fatal("final chunk must set m=0")
	}
}

func TestEncodeFrameRoundTripPayload(t *testing.T) {
	// The concatenated base64 chunks must decode back to the original bytes.
	data := []byte("hello kitty graphics payload")
	out := EncodeFrame(FormatPNG, 1, 1, data)
	encoded := extractBase64(out)
	if encoded == "" {
		t.Fatal("no payload found")
	}
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		t.Fatal(err)
	}
	if string(decoded) != string(data) {
		t.Fatalf("round-trip mismatch: got %q want %q", decoded, data)
	}
}

// extractBase64 strips the APC wrappers and metadata controls from an encoded
// frame, returning the concatenated base64 payload.
func extractBase64(frame string) string {
	var out strings.Builder
	rest := frame
	for len(rest) > 0 {
		start := strings.Index(rest, apcStart)
		if start < 0 {
			break
		}
		rest = rest[start+len(apcStart):]
		end := strings.Index(rest, apcEnd)
		if end < 0 {
			break
		}
		chunk := rest[:end]
		rest = rest[end+len(apcEnd):]
		if semi := strings.Index(chunk, ";"); semi >= 0 {
			out.WriteString(chunk[semi+1:])
		}
	}
	return out.String()
}
