package zstdz

import (
	"bytes"
	"testing"
)

func TestCompressAndDecompressFrame(t *testing.T) {
	input := []byte(`{"type":"session","id":"s1","version":1}` + "\n" +
		`{"type":"user/message","seq":1,"at":"2026-01-01T00:00:00Z"}` + "\n")

	out, err := CompressFrame(input)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) == 0 {
		t.Fatal("compressed frame is empty")
	}

	dec, err := DecompressFrame(out)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(input, dec) {
		t.Fatalf("round-trip mismatch:\ninput:  %q\ngot:    %q", input, dec)
	}
}

func TestScanFramesSingle(t *testing.T) {
	input := []byte(`{"type":"session","id":"s1","version":1}` + "\n")
	out, err := CompressFrame(input)
	if err != nil {
		t.Fatal(err)
	}

	scan := ScanFrames(out, 0)
	if len(scan.Frames) != 1 {
		t.Fatalf("expected 1 frame, got %d", len(scan.Frames))
	}
	if scan.TornStart != nil {
		t.Fatal("unexpected torn start")
	}
}

func TestScanFramesTwoAndTorn(t *testing.T) {
	frame1, _ := CompressFrame([]byte("hello\n"))
	frame2, _ := CompressFrame([]byte("world\n"))

	// Two complete frames
	combined := append(frame1, frame2...)
	scan := ScanFrames(combined, 0)
	if len(scan.Frames) != 2 {
		t.Fatalf("expected 2 frames, got %d", len(scan.Frames))
	}
	if scan.TornStart != nil {
		t.Fatal("unexpected torn start")
	}

	// Torn final frame (first 2 bytes of frame2 appended)
	torn := append(frame1, frame2[:2]...)
	scan = ScanFrames(torn, 0)
	if len(scan.Frames) != 1 {
		t.Fatalf("expected 1 complete frame, got %d", len(scan.Frames))
	}
	if scan.TornStart == nil {
		t.Fatal("expected torn start")
	}
}

func TestScanFramesInvalidMagic(t *testing.T) {
	// Not a valid zstd frame
	buf := []byte("garbage data here")
	scan := ScanFrames(buf, 0)
	if len(scan.Frames) != 0 {
		t.Fatalf("expected 0 frames for invalid magic, got %d", len(scan.Frames))
	}
	if scan.TornStart == nil {
		t.Fatal("expected torn start for invalid magic")
	}
}

func TestDecompressPrefix(t *testing.T) {
	input := []byte(`{"type":"session","id":"s1","version":1}` + "\n" +
		`{"type":"user/message","seq":1}`)

	out, err := CompressFrame(input)
	if err != nil {
		t.Fatal(err)
	}

	// Decompress partial — should recover what it can
	dec, err := DecompressPrefix(out)
	if err != nil {
		t.Fatal(err)
	}
	if len(dec) == 0 {
		t.Fatal("expected partial output")
	}
}
