package voice

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
)

// STTConfig holds speech-to-text configuration.
type STTConfig struct {
	Engine string `json:"engine"`
	Model  string `json:"model"`
	Lang   string `json:"lang"`
}

// Transcriber handles speech-to-text transcription.
type Transcriber struct {
	config STTConfig
}

// NewTranscriber creates a new transcriber.
func NewTranscriber(config STTConfig) *Transcriber {
	return &Transcriber{config: config}
}

// Transcribe transcribes audio data to text.
func (t *Transcriber) Transcribe(audioData []byte) (string, error) {
	// Try whisper.cpp if available
	if path, err := exec.LookPath("whisper"); err == nil {
		return t.transcribeWhisper(path, audioData)
	}
	// Fallback to other engines
	return "", fmt.Errorf("no STT engine available")
}

func (t *Transcriber) transcribeWhisper(path string, audioData []byte) (string, error) {
	// Write audio to temp file
	tmpFile, err := os.CreateTemp("", "hawk-voice-*.wav")
	if err != nil {
		return "", err
	}
	defer os.Remove(tmpFile.Name())
	if _, err := tmpFile.Write(audioData); err != nil {
		return "", err
	}
	tmpFile.Close()

	// Run whisper
	cmd := exec.Command(path, tmpFile.Name(), "-m", t.config.Model, "-l", t.config.Lang)
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("whisper failed: %w", err)
	}
	return out.String(), nil
}

// IsAvailable checks if voice mode is available.
func IsAvailable() bool {
	_, err := exec.LookPath("whisper")
	return err == nil
}

// Keyterms returns common voice command keyterms.
func Keyterms() []string {
	return []string{
		"hawk",
		"run",
		"test",
		"build",
		"fix",
		"explain",
		"search",
		"find",
		"edit",
		"create",
		"delete",
		"yes",
		"no",
		"stop",
		"continue",
	}
}
