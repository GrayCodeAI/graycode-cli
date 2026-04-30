package voice

import "testing"

func TestKeyterms(t *testing.T) {
	terms := Keyterms()
	if len(terms) == 0 {
		t.Fatal("expected keyterms")
	}
	seen := make(map[string]bool)
	for _, term := range terms {
		if seen[term] {
			t.Fatalf("duplicate keyterm: %s", term)
		}
		seen[term] = true
	}
}

func TestSTTConfig(t *testing.T) {
	config := STTConfig{
		Engine: "whisper",
		Model:  "base",
		Lang:   "en",
	}
	if config.Engine != "whisper" {
		t.Fatal("engine mismatch")
	}
}
