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
func (v *voiceSubcommand) Handle(m *chatModel, args []string, text string) (tea.Model, tea.Cmd) {
	out, err := exec.CommandContext(context.Background(), "which", "whisper").CombinedOutput()
	if err != nil || strings.TrimSpace(string(out)) == "" {
		m.messages = append(m.messages, displayMsg{role: "error", content: "Voice requires whisper.cpp. Install with: brew install whisper-cpp"})
	} else {
		m.messages = append(m.messages, displayMsg{role: "system", content: "Recording audio... (press Enter to stop)"})
		go func() {
			tmpFile := filepath.Join(os.TempDir(), "hawk_voice_input.wav")
			var recordCmd *exec.Cmd
			if _, err := exec.LookPath("sox"); err == nil {
				recordCmd = exec.Command("sox", "-d", tmpFile, "trim", "0", "10") // #nosec G204 -- fixed command 'sox' resolved via exec.LookPath
			} else if _, err := exec.LookPath("ffmpeg"); err == nil {
				recordCmd = exec.Command("ffmpeg", "-y", "-f", "avfoundation", "-i", ":0", "-t", "10", tmpFile) // #nosec G204 -- fixed command 'ffmpeg' resolved via exec.LookPath
			} else {
				m.messages = append(m.messages, displayMsg{role: "system", content: "No audio recorder found. Install sox (brew install sox) or use: whisper --model base -f recording.wav"})
				return
			}
			if err := recordCmd.Run(); err != nil {
				m.messages = append(m.messages, displayMsg{role: "error", content: fmt.Sprintf("Recording failed: %v", err)})
				return
			}
			transcribeCmd := exec.Command("whisper", "--model", "base", "--output_format", "txt", "--output_dir", os.TempDir(), tmpFile) // #nosec G204 -- fixed command 'whisper' with internal args
			if err := transcribeCmd.Run(); err != nil {
				m.messages = append(m.messages, displayMsg{role: "error", content: fmt.Sprintf("Transcription failed: %v", err)})
				return
			}
			txtFile := strings.TrimSuffix(tmpFile, ".wav") + ".txt"
			transcription, err := os.ReadFile(txtFile) // #nosec G304 -- txtFile derived from internally generated tmpFile path
			if err != nil {
				m.messages = append(m.messages, displayMsg{role: "error", content: "Could not read transcription"})
				return
			}
			transcript := strings.TrimSpace(string(transcription))
			if transcript != "" {
				m.input.SetValue(transcript)
				m.input.CursorEnd()
				m.messages = append(m.messages, displayMsg{role: "system", content: fmt.Sprintf("Voice input: %s", transcript)})
			}
		}()
	}
	return m, nil
}

func init() {
	subcommandRegistry.Register(&voiceSubcommand{})
}
