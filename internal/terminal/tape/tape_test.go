package tape

import (
	"bytes"
	"encoding/binary"
	"testing"
)

// fakeClock returns a caller-controlled millisecond timestamp.
type fakeClock struct{ t int64 }

func (c *fakeClock) NowMS() int64 { return c.t }

// appendFrameBytes serializes one frame the same way Writer does, for tests
// that craft tapes directly.
func appendFrameBytes(dst []byte, delta int32, kind byte, payload []byte) []byte {
	dst = binary.LittleEndian.AppendUint32(dst, uint32(delta))
	dst = append(dst, kind)
	dst = binary.LittleEndian.AppendUint32(dst, uint32(len(payload)))
	return append(dst, payload...)
}

func newTestWriter(t *testing.T) (*Writer, *bytes.Buffer, *fakeClock) {
	t.Helper()
	var buf bytes.Buffer
	clk := &fakeClock{t: 1000}
	w, err := NewWriter(&buf, 80, 24, "1.2", clk)
	if err != nil {
		t.Fatalf("new writer: %v", err)
	}
	return w, &buf, clk
}

// Header layout matches fx exactly: 5-byte magic + LE cols + LE rows + LE
// epoch + vlen + version.
func TestWriterHeaderByteExact(t *testing.T) {
	w, buf, _ := newTestWriter(t)
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	got := buf.Bytes()
	want := []byte{'F', 'X', 'T', 'P', 0x01, 80, 0, 24, 0, 0xe8, 0x03, 0, 0, 0, 0, 0, 0, 3, '1', '.', '2'}
	if !bytes.Equal(got, want) {
		t.Fatalf("\n got: %v\nwant: %v", got, want)
	}
}

// A frame is 9-byte header (LE delta i32, kind, LE len u32) + payload.
func TestWriterFramesByteExact(t *testing.T) {
	w, buf, clk := newTestWriter(t)
	clk.t += 25
	if err := w.RecordStdout([]byte("hi")); err != nil {
		t.Fatal(err)
	}
	clk.t += 10
	if err := w.RecordResize(60, 20); err != nil {
		t.Fatal(err)
	}
	clk.t += 5
	if err := w.RecordSigint(); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}

	data := buf.Bytes()
	tp, err := Parse(data)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if tp.Header.Cols != 80 || tp.Header.Rows != 24 || tp.Header.Version != "1.2" {
		t.Fatalf("bad header: %+v", tp.Header)
	}
	if len(tp.Frames) != 3 {
		t.Fatalf("expected 3 frames, got %d", len(tp.Frames))
	}
	f0 := tp.Frames[0]
	if f0.Kind != KindStdout || string(f0.Payload) != "hi" || f0.DeltaMS != 25 {
		t.Fatalf("bad stdout frame: %+v", f0)
	}
	f1 := tp.Frames[1]
	if f1.Kind != KindResize || len(f1.Payload) != 4 || f1.DeltaMS != 10 {
		t.Fatalf("bad resize frame: %+v", f1)
	}
	if cols := int(f1.Payload[0]) | int(f1.Payload[1])<<8; cols != 60 {
		t.Fatalf("resize cols=%d", cols)
	}
	if f2 := tp.Frames[2]; f2.Kind != KindSigint || f2.DeltaMS != 5 {
		t.Fatalf("bad sigint frame: %+v", f2)
	}
}

// Round-trip: a recorded sequence parses back byte-identical and replays to
// the expected snapshot.
func TestWriterParserReplayRoundTrip(t *testing.T) {
	w, buf, clk := newTestWriter(t)
	clk.t += 5
	_ = w.RecordMarker("start")
	clk.t += 3
	_ = w.RecordStdout([]byte("line one\n"))
	clk.t += 4
	_ = w.RecordStdout([]byte("\x1b[32mgreen\x1b[0m\n"))
	_ = w.Close()

	tp, err := Parse(buf.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if len(tp.Frames) != 3 {
		t.Fatalf("expected 3 frames, got %d", len(tp.Frames))
	}
	replay, snap := ReplayTape(tp)
	// "line one\n" (9) + "\x1b[32mgreen\x1b[0m\n" (15).
	if replay.Stdout != 24 {
		t.Fatalf("stdout bytes=%d, want 24", replay.Stdout)
	}
	if len(replay.Markers) != 1 || replay.Markers[0] != "start" {
		t.Fatalf("markers=%v", replay.Markers)
	}
	// SGR codes are stripped, leaving "green".
	want := "line one\ngreen"
	if snap != want {
		t.Fatalf("snapshot:\n%q\nwant:\n%q", snap, want)
	}
}

// Resize frames are honored during replay.
func TestReplayHonorsResize(t *testing.T) {
	w, buf, clk := newTestWriter(t)
	clk.t += 1
	_ = w.RecordResize(40, 10)
	_ = w.Close()
	tp, _ := Parse(buf.Bytes())
	r, _ := ReplayTape(tp)
	if r.Grid.Cols != 40 || r.Grid.Rows != 10 {
		t.Fatalf("grid not resized: %dx%d", r.Grid.Cols, r.Grid.Rows)
	}
}

// Unknown kinds and malformed resize payloads are tolerated, not fatal.
func TestParseToleratesUnknownAndMalformed(t *testing.T) {
	var buf bytes.Buffer
	// magic(5) + cols(2) + rows(2) + epoch(8) + vlen(1)=18 bytes, version empty.
	_ = binary.Write(&buf, binary.LittleEndian, []byte{'F', 'X', 'T', 'P', 0x01, 80, 0, 24, 0, 0x01, 0, 0, 0, 0, 0, 0, 0, 0})
	// unknown kind 200 with payload "x"; resize with bad (1-byte) payload.
	buf.Write(appendFrameBytes(nil, int32(0), 200, []byte("x")))
	buf.Write(appendFrameBytes(nil, int32(0), byte(KindResize), []byte("x")))

	tp, err := Parse(buf.Bytes())
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(tp.Frames) != 2 {
		t.Fatalf("expected 2 frames, got %d", len(tp.Frames))
	}
	if tp.Frames[0].Kind != Kind(200) {
		t.Fatalf("kind=%d", tp.Frames[0].Kind)
	}
	// Replay must not panic on the malformed resize.
	_, _ = ReplayTape(tp)
}

func TestParseRejectsBadMagicAndTruncation(t *testing.T) {
	if _, err := Parse([]byte("NOTA\x01garbage")); err == nil {
		t.Fatal("bad magic must error")
	}
	if _, err := Parse([]byte("FXTP")); err == nil {
		t.Fatal("too-short tape must error")
	}
	// Valid header then a truncated frame header.
	var buf bytes.Buffer
	buf.Write([]byte{'F', 'X', 'T', 'P', 0x01, 80, 0, 24, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0})
	buf.Write([]byte{1, 2, 3})
	if _, err := Parse(buf.Bytes()); err == nil {
		t.Fatal("truncated frame header must error")
	}
}
