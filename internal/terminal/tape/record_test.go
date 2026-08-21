package tape

import (
	"bytes"
	"testing"
)

func TestRecorderCapturesStdoutAndResize(t *testing.T) {
	clk := &fakeClock{t: 1000}
	var file bytes.Buffer
	var out bytes.Buffer
	var hadWriterErr bool
	rec, err := NewRecorder(&file, &out, 120, 30, clk)
	if err != nil {
		t.Fatalf("NewRecorder: %v", err)
	}
	_ = &hadWriterErr

	if _, err := rec.Write([]byte("hello")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	clk.t += 250
	if _, err := rec.Write([]byte(" world")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	clk.t += 100
	if err := rec.Resize(100, 20); err != nil {
		t.Fatalf("Resize: %v", err)
	}
	clk.t += 50
	if err := rec.Marker("done"); err != nil {
		t.Fatalf("Marker: %v", err)
	}
	if err := rec.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// Underlying output receives every byte verbatim.
	if got := out.String(); got != "hello world" {
		t.Errorf("forwarded output = %q, want %q", got, "hello world")
	}

	parsed, err := Parse(file.Bytes())
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if parsed.Header.Cols != 120 || parsed.Header.Rows != 30 {
		t.Errorf("header size = %dx%d, want 120x30", parsed.Header.Cols, parsed.Header.Rows)
	}
	if len(parsed.Frames) != 4 {
		t.Fatalf("frames = %d, want 4", len(parsed.Frames))
	}
	wantKinds := []Kind{KindStdout, KindStdout, KindResize, KindMarker}
	for i, f := range parsed.Frames {
		if f.Kind != wantKinds[i] {
			t.Errorf("frame[%d] kind = %s, want %s", i, f.Kind.String(), wantKinds[i].String())
		}
	}
	if string(parsed.Frames[0].Payload) != "hello" {
		t.Errorf("frame[0] payload = %q, want %q", parsed.Frames[0].Payload, "hello")
	}
	// Deltas: 0, 250, 100, 50.
	wantDeltas := []int32{0, 250, 100, 50}
	for i, f := range parsed.Frames {
		if f.DeltaMS != wantDeltas[i] {
			t.Errorf("frame[%d] delta = %d, want %d", i, f.DeltaMS, wantDeltas[i])
		}
	}
}

func TestRecorderCloseIdempotent(t *testing.T) {
	var file bytes.Buffer
	rec, err := NewRecorder(&file, &bytes.Buffer{}, 80, 24, &fakeClock{})
	if err != nil {
		t.Fatalf("NewRecorder: %v", err)
	}
	if err := rec.Close(); err != nil {
		t.Fatalf("first Close: %v", err)
	}
	if err := rec.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
}
