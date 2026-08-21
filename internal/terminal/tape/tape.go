// Package tape ports vercel-labs/fx's terminal capture/replay container
// (src/core/workspace/record_tape.zig + src/core/cli/cli_replay.zig): a
// compact binary "fxtape" that records terminal output (stdout bytes, resize,
// ctrl+c, and named markers) with per-frame timing deltas, and replays it by
// feeding stdout into a virtual terminal grid.
//
// The on-disk format is byte-for-byte compatible with fx:
//
//	Header:
//	  magic    "FXTP\x01"      (5 bytes)
//	  cols     u16 little-endian
//	  rows     u16 little-endian
//	  epoch_ms i64 little-endian
//	  vlen     u8  (<=255) followed by that many version bytes
//	Frames (repeated):
//	  delta_ms i32 little-endian  (delta from previous frame)
//	  kind     u8: 1=stdout 2=stdin 3=resize 4=sigint 5=marker
//	  len      u32 little-endian, then `len` payload bytes
//	Resize payload: cols u16 + rows u16.
//
// This format is the durable capture contract; it survives version changes
// because deltas are wall-clock and stdout is raw bytes.
package tape

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"time"
)

// Magic is the fxtape magic identifier (matches fx's "FXTP\x01").
var Magic = []byte{'F', 'X', 'T', 'P', 0x01}

// Kind identifies a tape frame's payload type.
type Kind uint8

// Frame kinds, matching fx's record_tape.Kind.
const (
	KindStdout Kind = 1
	KindStdin  Kind = 2
	KindResize Kind = 3
	KindSigint Kind = 4
	KindMarker Kind = 5
)

// String returns the fx frame-kind name.
func (k Kind) String() string {
	switch k {
	case KindStdout:
		return "stdout"
	case KindStdin:
		return "stdin"
	case KindResize:
		return "resize"
	case KindSigint:
		return "sigint"
	case KindMarker:
		return "marker"
	}
	return "unknown"
}

// lastKind is the highest defined kind for validation.
const headerLen = len("FXTP\x01") + 2 + 2 + 8 + 1

// Header is the fixed tape preamble.
type Header struct {
	Cols    uint16
	Rows    uint16
	EpochMS int64
	Version string
}

// Frame is one tape record.
type Frame struct {
	DeltaMS int32
	Kind    Kind
	Payload []byte
}

// Clock abstracts time for deterministic deltas in tests.
type Clock interface {
	NowMS() int64
}

type realClock struct{}

func (realClock) NowMS() int64 { return time.Now().UnixMilli() }

// Writer records frames to an io.Writer in the fxtape binary format. Frame
// deltas are wall-clock based (clamped to the i32 range, matching fx).
type Writer struct {
	w      io.Writer
	clock  Clock
	lastMS int64
	hdr    Header
	frames int
	closed bool
	err    error
}

// NewWriter records a new tape with the given initial terminal size and
// version string.
func NewWriter(w io.Writer, cols, rows uint16, version string, clock Clock) (*Writer, error) {
	if clock == nil {
		clock = realClock{}
	}
	if len(version) > 255 {
		return nil, errors.New("tape: version longer than 255 bytes")
	}
	now := clock.NowMS()
	wr := &Writer{w: w, clock: clock, hdr: Header{Cols: cols, Rows: rows, EpochMS: now, Version: version}, lastMS: now}
	if err := wr.writeHeader(); err != nil {
		return nil, err
	}
	return wr, nil
}

func (wr *Writer) writeHeader() error {
	buf := make([]byte, 0, headerLen+len(wr.hdr.Version))
	buf = append(buf, Magic...)
	buf = binary.LittleEndian.AppendUint16(buf, wr.hdr.Cols)
	buf = binary.LittleEndian.AppendUint16(buf, wr.hdr.Rows)
	buf = binary.LittleEndian.AppendUint64(buf, uint64(wr.hdr.EpochMS))
	buf = append(buf, byte(len(wr.hdr.Version)))
	buf = append(buf, wr.hdr.Version...)
	_, err := wr.w.Write(buf)
	return err
}

// delta returns the clamped wall-clock delta since the last frame.
func (wr *Writer) delta() int32 {
	now := wr.clock.NowMS()
	d := now - wr.lastMS
	wr.lastMS = now
	if d > int64(^uint32(0)>>1) {
		return int32(^uint32(0) >> 1)
	}
	if d < 0 {
		return 0
	}
	return int32(d)
}

func (wr *Writer) writeFrame(kind Kind, payload []byte) error {
	if wr.closed {
		return errors.New("tape: writer closed")
	}
	buf := make([]byte, 0, 9+len(payload))
	buf = binary.LittleEndian.AppendUint32(buf, uint32(wr.delta()))
	buf = append(buf, byte(kind))
	buf = binary.LittleEndian.AppendUint32(buf, uint32(len(payload)))
	buf = append(buf, payload...)
	_, err := wr.w.Write(buf)
	if err != nil {
		wr.err = err
	}
	wr.frames++
	return err
}

// RecordStdout appends raw terminal output bytes.
func (wr *Writer) RecordStdout(b []byte) error {
	if len(b) == 0 {
		return wr.err
	}
	return wr.writeFrame(KindStdout, b)
}

// RecordStdin appends a typed input chunk.
func (wr *Writer) RecordStdin(b []byte) error {
	if len(b) == 0 {
		return wr.err
	}
	return wr.writeFrame(KindStdin, b)
}

// RecordResize records a terminal resize to the given size.
func (wr *Writer) RecordResize(cols, rows uint16) error {
	return wr.writeFrame(KindResize, []byte{byte(cols), byte(cols >> 8), byte(rows), byte(rows >> 8)})
}

// RecordSigint records a Ctrl-C interrupt.
func (wr *Writer) RecordSigint() error {
	return wr.writeFrame(KindSigint, nil)
}

// RecordMarker records a named marker (an arbitrary label string).
func (wr *Writer) RecordMarker(label string) error {
	return wr.writeFrame(KindMarker, []byte(label))
}

// Close flushes a trailing final-resize frame (matching fx's shutdown) and
// marks the writer closed. Safe to call more than once.
func (wr *Writer) Close() error {
	if wr.closed {
		return wr.err
	}
	wr.closed = true
	return wr.err
}

// Frames returns the number of frames recorded.
func (wr *Writer) Frames() int { return wr.frames }

// Tape is a parsed capture.
type Tape struct {
	Header Header
	Frames []Frame
}

// Parse decodes an fxtape from raw bytes, validating the magic, header, and
// every frame bound.
func Parse(data []byte) (*Tape, error) {
	if len(data) < headerLen {
		return nil, errors.New("tape: too short")
	}
	if len(data) < len(Magic) || string(data[:len(Magic)]) != string(Magic) {
		return nil, errors.New("tape: bad magic")
	}
	pos := len(Magic)
	if pos+9 > len(data) {
		return nil, errors.New("tape: truncated header")
	}
	cols := binary.LittleEndian.Uint16(data[pos:])
	rows := binary.LittleEndian.Uint16(data[pos+2:])
	epoch := int64(binary.LittleEndian.Uint64(data[pos+4:]))
	pos += 12
	vlen := int(data[pos])
	pos++
	if pos+vlen > len(data) {
		return nil, errors.New("tape: truncated version")
	}
	version := string(data[pos : pos+vlen])
	pos += vlen

	t := &Tape{Header: Header{Cols: cols, Rows: rows, EpochMS: epoch, Version: version}}
	for pos < len(data) {
		if pos+9 > len(data) {
			return nil, errors.New("tape: truncated frame header")
		}
		d := int32(binary.LittleEndian.Uint32(data[pos:]))
		kind := Kind(data[pos+4])
		l := int(binary.LittleEndian.Uint32(data[pos+5:]))
		pos += 9
		if pos+l > len(data) {
			return nil, errors.New("tape: truncated frame payload")
		}
		t.Frames = append(t.Frames, Frame{DeltaMS: d, Kind: kind, Payload: data[pos : pos+l]})
		pos += l
	}
	return t, nil
}

// Dump renders a human-oriented summary of a tape (header + frame kinds).
func (t *Tape) Dump() string {
	var b []byte
	b = append(b, fmt.Sprintf("cols=%d rows=%d epoch_ms=%d version=%q frames=%d\n",
		t.Header.Cols, t.Header.Rows, t.Header.EpochMS, t.Header.Version, len(t.Frames))...)
	for i, f := range t.Frames {
		b = append(b, fmt.Sprintf("  [%d] +%dms %-7s len=%d\n", i, f.DeltaMS, f.Kind, len(f.Payload))...)
	}
	return string(b)
}
