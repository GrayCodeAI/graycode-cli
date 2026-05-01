package cmd

import (
	"runtime"
	"testing"
)

func TestCopyToClipboard_SignatureExists(t *testing.T) {
	var _ func(string) error = copyToClipboard
}

func TestPasteFromClipboard_SignatureExists(t *testing.T) {
	var _ func() (string, error) = pasteFromClipboard
}

func TestClipboardRoundTrip(t *testing.T) {
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" && runtime.GOOS != "windows" {
		t.Skip("clipboard not supported on " + runtime.GOOS)
	}

	// Only run on macOS in CI-like environments where pbcopy is available
	if runtime.GOOS == "linux" {
		t.Skip("skipping clipboard test on linux (requires xclip/xsel)")
	}

	text := "hawk clipboard test"
	if err := copyToClipboard(text); err != nil {
		t.Skipf("clipboard not available: %v", err)
	}

	got, err := pasteFromClipboard()
	if err != nil {
		t.Fatalf("paste failed: %v", err)
	}
	if got != text {
		t.Fatalf("clipboard round-trip: got %q, want %q", got, text)
	}
}
