package cmd

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"
)

// voiceSubcommand implements the /voice slash command. It
// records audio (via sox or ffmpeg) and transcribes it via
// whisper.cpp. The transcription is then injected into the
// input box.
type voiceSubcommand struct{}

func (v *voiceSubcommand) Name() string        { return "voice" }
func (v *voiceSubcommand) Aliases() []string   { return nil }
func (v *voiceSubcommand) Description() string { return "record audio and transcribe to text input" }
func (v *voiceSubcommand) Usage() string       { return "" }

// voiceResultMsg carries the outcome of the background recording/transcription
// back to the update loop. Delivering the result as a tea.Msg (instead of
// mutating the model from a raw goroutine, as before) keeps all model mutation
// on the Bubble Tea goroutine and avoids a data race on m.messages and m.input.
type voiceResultMsg struct {
	transcript string // non-empty: inject into the input and announce it
	info       string // non-empty: show as a system message (e.g. no recorder found)
	err        string // non-empty: show as an error message
}

func (v *voiceSubcommand) Handle(m *chatModel, args []string, text string) (tea.Model, tea.Cmd) {
	out, err := exec.CommandContext(context.Background(), "which", "whisper").CombinedOutput()
	if err != nil || strings.TrimSpace(string(out)) == "" {
		m.messages = append(m.messages, displayMsg{role: "error", content: "Voice requires whisper.cpp. Install with: brew install whisper-cpp"})
		return m, nil
	}
	m.messages = append(m.messages, displayMsg{role: "system", content: "Recording audio... (press Enter to stop)"})
	return m, v.recordAndTranscribe()
}

// recordAndTranscribe returns a tea.Cmd that records audio and transcribes it on
// a background goroutine managed by the Bubble Tea runtime, reporting the
// outcome as a voiceResultMsg handled in the update loop.
func (v *voiceSubcommand) recordAndTranscribe() tea.Cmd {
	return func() tea.Msg {
		tmpFile := filepath.Join(os.TempDir(), "graycode_voice_input.wav")
		var recordCmd *exec.Cmd
		if _, err := exec.LookPath("sox"); err == nil {
			recordCmd = exec.Command("sox", "-d", tmpFile, "trim", "0", "10") // #nosec G204 -- fixed command 'sox' resolved via exec.LookPath
		} else if _, err := exec.LookPath("ffmpeg"); err == nil {
			recordCmd = exec.Command("ffmpeg", "-y", "-f", "avfoundation", "-i", ":0", "-t", "10", tmpFile) // #nosec G204 -- fixed command 'ffmpeg' resolved via exec.LookPath
		} else {
			return voiceResultMsg{info: "No audio recorder found. Install sox (brew install sox) or use: whisper --model base -f recording.wav"}
		}
		if err := recordCmd.Run(); err != nil {
			return voiceResultMsg{err: fmt.Sprintf("Recording failed: %v", err)}
		}
		transcribeCmd := exec.Command("whisper", "--model", "base", "--output_format", "txt", "--output_dir", os.TempDir(), tmpFile) // #nosec G204 -- fixed command 'whisper' with internal args
		if err := transcribeCmd.Run(); err != nil {
			return voiceResultMsg{err: fmt.Sprintf("Transcription failed: %v", err)}
		}
		txtFile := strings.TrimSuffix(tmpFile, ".wav") + ".txt"
		transcription, err := os.ReadFile(txtFile) // #nosec G304 -- txtFile derived from internally generated tmpFile path
		if err != nil {
			return voiceResultMsg{err: "Could not read transcription"}
		}
		return voiceResultMsg{transcript: strings.TrimSpace(string(transcription))}
	}
}

func init() {
	subcommandRegistry.Register(&voiceSubcommand{})
}
