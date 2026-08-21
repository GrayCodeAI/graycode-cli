package tape

import (
	"io"
	"sync"
)

// Recorder captures a live output stream into an fxtape (fx `--record`
// parity). It wraps the underlying terminal writer so every byte written is
// both forwarded to the terminal and stored as a stdout frame with a
// wall-clock delta; terminal size changes are recorded as resize frames.
type Recorder struct {
	mu  sync.Mutex
	w   *Writer
	out io.Writer
}

// NewRecorder returns a Recorder that forwards to out and writes an fxtape to
// file. Pass nil to use the default wall clock.
func NewRecorder(file io.Writer, out io.Writer, cols, rows uint16, clock Clock) (*Recorder, error) {
	w, err := NewWriter(file, cols, rows, "1", clock)
	if err != nil {
		return nil, err
	}
	return &Recorder{w: w, out: out}, nil
}

// Write implements io.Writer: it forwards p to the underlying output and
// records exactly the bytes written as a stdout frame.
func (r *Recorder) Write(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	n, err := r.out.Write(p)
	if n > 0 {
		_ = r.w.RecordStdout(p[:n])
	}
	return n, err
}

// Resize records a terminal size change.
func (r *Recorder) Resize(cols, rows uint16) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.w.RecordResize(cols, rows)
}

// Marker records a named marker frame.
func (r *Recorder) Marker(label string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.w.RecordMarker(label)
}

// Close finalizes the tape. Safe to call more than once.
func (r *Recorder) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.w.Close()
}
