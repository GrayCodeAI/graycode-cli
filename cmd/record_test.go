package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/GrayCodeAI/graycode-cli/internal/terminal/tape"
)

func TestStartRecordingCapturesOutput(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rec.fxtape")
	restore, err := startRecording(path)
	if err != nil {
		t.Fatalf("startRecording: %v", err)
	}
	if _, err := fmt.Fprint(replOut, "live bytes"); err != nil {
		t.Fatalf("Fprint: %v", err)
	}
	restore()

	if replOut != os.Stdout {
		t.Errorf("replOut not restored to os.Stdout after cleanup")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read tape: %v", err)
	}
	parsed, err := tape.Parse(data)
	if err != nil {
		t.Fatalf("parse tape: %v", err)
	}
	if len(parsed.Frames) != 1 || parsed.Frames[0].Kind != tape.KindStdout || string(parsed.Frames[0].Payload) != "live bytes" {
		t.Errorf("frames = %+v, want single stdout frame 'live bytes'", parsed.Frames)
	}
}
