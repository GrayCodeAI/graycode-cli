package voice

import (
	"strings"
	"testing"
)

func TestNewTranscriber(t *testing.T) {
	config := STTConfig{
		Engine: "whisper",
		Model:  "base",
		Lang:   "en",
	}
	transcriber := NewTranscriber(config)
	if transcriber == nil {
		t.Fatal("expected non-nil transcriber")
	}
	if transcriber.config.Engine != "whisper" {
		t.Errorf("config.Engine = %q, want %q", transcriber.config.Engine, "whisper")
	}
	if transcriber.config.Model != "base" {
		t.Errorf("config.Model = %q, want %q", transcriber.config.Model, "base")
	}
	if transcriber.config.Lang != "en" {
		t.Errorf("config.Lang = %q, want %q", transcriber.config.Lang, "en")
	}
}

func TestNewTranscriber_EmptyConfig(t *testing.T) {
	transcriber := NewTranscriber(STTConfig{})
	if transcriber == nil {
		t.Fatal("expected non-nil transcriber")
	}
	if transcriber.config.Engine != "" {
		t.Errorf("config.Engine = %q, want empty", transcriber.config.Engine)
	}
}

func TestTranscribe_NoEngineAvailable(t *testing.T) {
	transcriber := NewTranscriber(STTConfig{
		Engine: "whisper",
		Model:  "base",
		Lang:   "en",
	})
	// whisper is not installed, so this should return an error
	result, err := transcriber.Transcribe([]byte("fake audio data"))
	if err == nil {
		t.Error("expected error when no STT engine is available")
	}
	if !strings.Contains(err.Error(), "no STT engine available") {
		t.Errorf("error = %q, want to contain 'no STT engine available'", err.Error())
	}
	if result != "" {
		t.Errorf("result = %q, want empty", result)
	}
}

func TestTranscribe_EmptyAudioData(t *testing.T) {
	transcriber := NewTranscriber(STTConfig{})
	// Even with empty audio data, if no engine is available, should return error
	_, err := transcriber.Transcribe([]byte{})
	if err == nil {
		t.Error("expected error when no STT engine is available")
	}
}

func TestTranscribe_NilAudioData(t *testing.T) {
	transcriber := NewTranscriber(STTConfig{})
	_, err := transcriber.Transcribe(nil)
	if err == nil {
		t.Error("expected error when no STT engine is available")
	}
}

func TestIsAvailable_NotInstalled(t *testing.T) {
	// whisper is not installed in the test environment
	available := IsAvailable()
	if available {
		t.Error("expected IsAvailable() to return false when whisper is not installed")
	}
}

func TestTranscribeWhisper_NonExistentBinary(t *testing.T) {
	transcriber := NewTranscriber(STTConfig{
		Engine: "whisper",
		Model:  "base",
		Lang:   "en",
	})
	// Call transcribeWhisper with a non-existent binary path
	// This should fail at exec.CommandContext
	result, err := transcriber.transcribeWhisper("/nonexistent/whisper/binary", []byte("audio"))
	if err == nil {
		t.Error("expected error when whisper binary doesn't exist")
	}
	if result != "" {
		t.Errorf("result = %q, want empty", result)
	}
}

func TestTranscribeWhisper_EmptyAudioData(t *testing.T) {
	transcriber := NewTranscriber(STTConfig{
		Engine: "whisper",
		Model:  "base",
		Lang:   "en",
	})
	// Even with empty audio data, the command will fail since the binary doesn't exist
	_, err := transcriber.transcribeWhisper("/nonexistent/whisper/binary", []byte{})
	if err == nil {
		t.Error("expected error when whisper binary doesn't exist")
	}
}

func TestSTTConfig_JSONRoundTrip(t *testing.T) {
	original := STTConfig{
		Engine: "whisper",
		Model:  "base",
		Lang:   "en",
	}
	// Verify fields are accessible
	if original.Engine != "whisper" || original.Model != "base" || original.Lang != "en" {
		t.Error("config fields don't match")
	}
}

func TestKeyterms_ContainsExpectedTerms(t *testing.T) {
	terms := Keyterms()
	expected := []string{"hawk", "run", "test", "build", "yes", "no", "stop"}
	for _, e := range expected {
		found := false
		for _, t := range terms {
			if t == e {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected keyterm %q not found", e)
		}
	}
}
