package cmd

import (
	"os"
	"strings"
	"testing"
	"time"
)

func newTestFormatter(color, unicode bool, width int) *OutputFormatter {
	theme := OutputTheme{}
	if color {
		theme = OutputTheme{
			Primary:   "\033[36m",
			Secondary: "\033[35m",
			Success:   "\033[32m",
			Error:     "\033[31m",
			Warning:   "\033[33m",
			Info:      "\033[34m",
			Muted:     "\033[90m",
			Reset:     "\033[0m",
		}
	}
	return &OutputFormatter{
		Width:          width,
		ColorEnabled:   color,
		UnicodeEnabled: unicode,
		Theme:          theme,
		Verbose:        false,
	}
}

func TestNewOutputFormatter(t *testing.T) {
	f := NewOutputFormatter()
	if f == nil {
		t.Fatal("NewOutputFormatter returned nil")
	}
	if f.Width <= 0 {
		t.Errorf("expected positive width, got %d", f.Width)
	}
}

func TestFormatSuccess(t *testing.T) {
	t.Run("unicode with color", func(t *testing.T) {
		f := newTestFormatter(true, true, 80)
		result := f.FormatSuccess("done")
		if !strings.Contains(result, "✔") {
			t.Errorf("expected unicode checkmark, got: %s", result)
		}
		if !strings.Contains(result, "done") {
			t.Errorf("expected message 'done', got: %s", result)
		}
		if !strings.Contains(result, "\033[32m") {
			t.Errorf("expected green color code, got: %s", result)
		}
	})

	t.Run("ascii no color", func(t *testing.T) {
		f := newTestFormatter(false, false, 80)
		result := f.FormatSuccess("done")
		if !strings.Contains(result, "[ok]") {
			t.Errorf("expected [ok], got: %s", result)
		}
		if !strings.Contains(result, "done") {
			t.Errorf("expected message 'done', got: %s", result)
		}
	})
}

func TestFormatError(t *testing.T) {
	t.Run("unicode with color", func(t *testing.T) {
		f := newTestFormatter(true, true, 80)
		result := f.FormatError("failed")
		if !strings.Contains(result, "✘") {
			t.Errorf("expected unicode X, got: %s", result)
		}
		if !strings.Contains(result, "\033[31m") {
			t.Errorf("expected red color code, got: %s", result)
		}
	})

	t.Run("ascii no color", func(t *testing.T) {
		f := newTestFormatter(false, false, 80)
		result := f.FormatError("failed")
		if !strings.Contains(result, "[X]") {
			t.Errorf("expected [X], got: %s", result)
		}
	})
}

func TestFormatWarning(t *testing.T) {
	t.Run("unicode with color", func(t *testing.T) {
		f := newTestFormatter(true, true, 80)
		result := f.FormatWarning("careful")
		if !strings.Contains(result, "⚠") {
			t.Errorf("expected unicode warning, got: %s", result)
		}
		if !strings.Contains(result, "\033[33m") {
			t.Errorf("expected yellow color code, got: %s", result)
		}
	})

	t.Run("ascii no color", func(t *testing.T) {
		f := newTestFormatter(false, false, 80)
		result := f.FormatWarning("careful")
		if !strings.Contains(result, "[!]") {
			t.Errorf("expected [!], got: %s", result)
		}
	})
}

func TestFormatInfo(t *testing.T) {
	t.Run("unicode with color", func(t *testing.T) {
		f := newTestFormatter(true, true, 80)
		result := f.FormatInfo("note")
		if !strings.Contains(result, "●") {
			t.Errorf("expected unicode circle, got: %s", result)
		}
		if !strings.Contains(result, "\033[34m") {
			t.Errorf("expected blue color code, got: %s", result)
		}
	})

	t.Run("ascii no color", func(t *testing.T) {
		f := newTestFormatter(false, false, 80)
		result := f.FormatInfo("note")
		if !strings.Contains(result, "[i]") {
			t.Errorf("expected [i], got: %s", result)
		}
	})
}

func TestFormatTable(t *testing.T) {
	t.Run("unicode table", func(t *testing.T) {
		f := newTestFormatter(false, true, 80)
		headers := []string{"Name", "Age", "City"}
		rows := [][]string{
			{"Alice", "30", "New York"},
			{"Bob", "25", "London"},
		}
		result := f.FormatTable(headers, rows)
		if !strings.Contains(result, "┌") {
			t.Errorf("expected box-drawing top-left, got: %s", result)
		}
		if !strings.Contains(result, "│") {
			t.Errorf("expected box-drawing vertical, got: %s", result)
		}
		if !strings.Contains(result, "Alice") {
			t.Errorf("expected cell content 'Alice', got: %s", result)
		}
		if !strings.Contains(result, "└") {
			t.Errorf("expected box-drawing bottom-left, got: %s", result)
		}
	})

	t.Run("ascii table", func(t *testing.T) {
		f := newTestFormatter(false, false, 80)
		headers := []string{"Name", "Age"}
		rows := [][]string{
			{"Alice", "30"},
		}
		result := f.FormatTable(headers, rows)
		if !strings.Contains(result, "+") {
			t.Errorf("expected '+' in ASCII table, got: %s", result)
		}
		if !strings.Contains(result, "|") {
			t.Errorf("expected '|' in ASCII table, got: %s", result)
		}
	})

	t.Run("empty headers", func(t *testing.T) {
		f := newTestFormatter(false, false, 80)
		result := f.FormatTable([]string{}, nil)
		if result != "" {
			t.Errorf("expected empty string for no headers, got: %s", result)
		}
	})

	t.Run("truncation for narrow terminal", func(t *testing.T) {
		f := newTestFormatter(false, false, 30)
		headers := []string{"Name", "Description"}
		rows := [][]string{
			{"test", "a very long description that should be truncated"},
		}
		result := f.FormatTable(headers, rows)
		// Should not exceed terminal width significantly
		lines := strings.Split(result, "\n")
		for _, line := range lines {
			if len(line) > 40 {
				t.Errorf("line too long for narrow terminal: %d chars: %s", len(line), line)
			}
		}
	})
}

func TestFormatList(t *testing.T) {
	t.Run("numbered list", func(t *testing.T) {
		f := newTestFormatter(false, false, 80)
		items := []string{"first", "second", "third"}
		result := f.FormatList(items, true)
		if !strings.Contains(result, "1. first") {
			t.Errorf("expected numbered item, got: %s", result)
		}
		if !strings.Contains(result, "3. third") {
			t.Errorf("expected numbered item 3, got: %s", result)
		}
	})

	t.Run("bullet list unicode", func(t *testing.T) {
		f := newTestFormatter(false, true, 80)
		items := []string{"alpha", "beta"}
		result := f.FormatList(items, false)
		if !strings.Contains(result, "•") {
			t.Errorf("expected unicode bullet, got: %s", result)
		}
	})

	t.Run("bullet list ascii", func(t *testing.T) {
		f := newTestFormatter(false, false, 80)
		items := []string{"alpha", "beta"}
		result := f.FormatList(items, false)
		if !strings.Contains(result, "- alpha") {
			t.Errorf("expected dash bullet, got: %s", result)
		}
	})
}

func TestFormatProgress(t *testing.T) {
	t.Run("unicode progress", func(t *testing.T) {
		f := newTestFormatter(false, true, 80)
		result := f.FormatProgress(8, 12, "Installing packages...")
		if !strings.Contains(result, "█") {
			t.Errorf("expected filled block, got: %s", result)
		}
		if !strings.Contains(result, "░") {
			t.Errorf("expected empty block, got: %s", result)
		}
		if !strings.Contains(result, "66%") && !strings.Contains(result, "67%") {
			t.Errorf("expected percentage around 66-67%%, got: %s", result)
		}
		if !strings.Contains(result, "(8/12)") {
			t.Errorf("expected (8/12), got: %s", result)
		}
		if !strings.Contains(result, "Installing packages...") {
			t.Errorf("expected label, got: %s", result)
		}
	})

	t.Run("ascii progress", func(t *testing.T) {
		f := newTestFormatter(false, false, 80)
		result := f.FormatProgress(5, 10, "Loading")
		if !strings.Contains(result, "#") {
			t.Errorf("expected '#' fill, got: %s", result)
		}
		if !strings.Contains(result, "-") {
			t.Errorf("expected '-' empty, got: %s", result)
		}
		if !strings.Contains(result, "50%") {
			t.Errorf("expected 50%%, got: %s", result)
		}
	})

	t.Run("zero total", func(t *testing.T) {
		f := newTestFormatter(false, false, 80)
		result := f.FormatProgress(0, 0, "Nothing")
		if !strings.Contains(result, "0%") {
			t.Errorf("expected 0%% for zero total, got: %s", result)
		}
	})

	t.Run("complete", func(t *testing.T) {
		f := newTestFormatter(false, false, 80)
		result := f.FormatProgress(10, 10, "Done")
		if !strings.Contains(result, "100%") {
			t.Errorf("expected 100%%, got: %s", result)
		}
	})
}

func TestFormatTree(t *testing.T) {
	t.Run("unicode tree", func(t *testing.T) {
		f := newTestFormatter(false, true, 80)
		children := []TreeNode{
			{
				Name: "auth/",
				Children: []TreeNode{
					{Name: "token.go"},
					{Name: "middleware.go"},
				},
			},
			{
				Name: "handler/",
				Children: []TreeNode{
					{Name: "api.go"},
				},
			},
			{Name: "main.go"},
		}
		result := f.FormatTree("src/", children)
		if !strings.Contains(result, "├── auth/") {
			t.Errorf("expected tree branch, got: %s", result)
		}
		if !strings.Contains(result, "└── main.go") {
			t.Errorf("expected last branch, got: %s", result)
		}
		if !strings.Contains(result, "│   ├── token.go") {
			t.Errorf("expected nested branch, got: %s", result)
		}
		if !strings.Contains(result, "│   └── middleware.go") {
			t.Errorf("expected nested last branch, got: %s", result)
		}
	})

	t.Run("ascii tree", func(t *testing.T) {
		f := newTestFormatter(false, false, 80)
		children := []TreeNode{
			{Name: "file1.go"},
			{Name: "file2.go"},
		}
		result := f.FormatTree("root/", children)
		if !strings.Contains(result, "|-- file1.go") {
			t.Errorf("expected ASCII branch, got: %s", result)
		}
		if !strings.Contains(result, "`-- file2.go") {
			t.Errorf("expected ASCII last branch, got: %s", result)
		}
	})

	t.Run("tree with icons", func(t *testing.T) {
		f := newTestFormatter(false, true, 80)
		children := []TreeNode{
			{Name: "main.go", Icon: "📄"},
		}
		result := f.FormatTree("project/", children)
		if !strings.Contains(result, "📄 main.go") {
			t.Errorf("expected icon before name, got: %s", result)
		}
	})
}

func TestFormatDiff(t *testing.T) {
	t.Run("with color", func(t *testing.T) {
		f := newTestFormatter(true, false, 80)
		result := f.FormatDiff(234, 89)
		if !strings.Contains(result, "+234") {
			t.Errorf("expected +234, got: %s", result)
		}
		if !strings.Contains(result, "-89") {
			t.Errorf("expected -89, got: %s", result)
		}
		if !strings.Contains(result, "\033[32m") {
			t.Errorf("expected green for added, got: %s", result)
		}
		if !strings.Contains(result, "\033[31m") {
			t.Errorf("expected red for removed, got: %s", result)
		}
	})

	t.Run("without color", func(t *testing.T) {
		f := newTestFormatter(false, false, 80)
		result := f.FormatDiff(10, 5)
		if result != "+10 -5" {
			t.Errorf("expected '+10 -5', got: %s", result)
		}
	})
}

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		input    time.Duration
		expected string
	}{
		{0, "0s"},
		{500 * time.Millisecond, "500ms"},
		{1200 * time.Millisecond, "1.2s"},
		{30 * time.Second, "30.0s"},
		{2*time.Minute + 15*time.Second, "2m 15s"},
		{5 * time.Minute, "5m"},
		{1*time.Hour + 3*time.Minute, "1h 3m"},
		{2 * time.Hour, "2h"},
	}

	f := newTestFormatter(false, false, 80)
	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			result := f.FormatDuration(tt.input)
			if result != tt.expected {
				t.Errorf("FormatDuration(%v) = %s, want %s", tt.input, result, tt.expected)
			}
		})
	}
}

func TestFormatterFormatBytes(t *testing.T) {
	tests := []struct {
		input    int64
		expected string
	}{
		{0, "0B"},
		{512, "512B"},
		{1024, "1.0KB"},
		{1536, "1.5KB"},
		{1048576, "1.0MB"},
		{3670016, "3.5MB"},
		{1073741824, "1.0GB"},
		{-1, "0B"},
	}

	f := newTestFormatter(false, false, 80)
	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			result := f.FormatBytes(tt.input)
			if result != tt.expected {
				t.Errorf("FormatBytes(%d) = %s, want %s", tt.input, result, tt.expected)
			}
		})
	}
}

func TestFormatNumber(t *testing.T) {
	tests := []struct {
		input    int
		expected string
	}{
		{0, "0"},
		{999, "999"},
		{1234, "1,234"},
		{9999, "9,999"},
		{45600, "45.6K"},
		{1200000, "1.2M"},
	}

	f := newTestFormatter(false, false, 80)
	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			result := f.FormatNumber(tt.input)
			if result != tt.expected {
				t.Errorf("FormatNumber(%d) = %s, want %s", tt.input, result, tt.expected)
			}
		})
	}
}

func TestTruncate(t *testing.T) {
	f := newTestFormatter(false, false, 80)

	tests := []struct {
		input    string
		width    int
		expected string
	}{
		{"hello", 10, "hello"},
		{"hello world", 5, "he..."},
		{"hello world", 8, "hello..."},
		{"hi", 2, "hi"},
		{"hello", 3, "hel"},
		{"hello", 0, ""},
		{"", 5, ""},
	}

	for _, tt := range tests {
		t.Run(tt.input+"_"+tt.expected, func(t *testing.T) {
			result := f.Truncate(tt.input, tt.width)
			if result != tt.expected {
				t.Errorf("Truncate(%q, %d) = %q, want %q", tt.input, tt.width, result, tt.expected)
			}
		})
	}
}

func TestDetectTerminalWidth(t *testing.T) {
	t.Run("default width", func(t *testing.T) {
		origColumns := os.Getenv("COLUMNS")
		os.Unsetenv("COLUMNS")
		defer os.Setenv("COLUMNS", origColumns)

		width := DetectTerminalWidth()
		if width != 80 {
			t.Errorf("expected default 80, got %d", width)
		}
	})

	t.Run("from COLUMNS env", func(t *testing.T) {
		origColumns := os.Getenv("COLUMNS")
		os.Setenv("COLUMNS", "120")
		defer os.Setenv("COLUMNS", origColumns)

		width := DetectTerminalWidth()
		if width != 120 {
			t.Errorf("expected 120, got %d", width)
		}
	})
}

func TestDetectColorSupport(t *testing.T) {
	t.Run("NO_COLOR disables", func(t *testing.T) {
		origNoColor := os.Getenv("NO_COLOR")
		origForceColor := os.Getenv("FORCE_COLOR")
		origTerm := os.Getenv("TERM")

		os.Setenv("NO_COLOR", "1")
		os.Unsetenv("FORCE_COLOR")
		os.Setenv("TERM", "xterm-256color")
		defer func() {
			os.Setenv("NO_COLOR", origNoColor)
			os.Setenv("FORCE_COLOR", origForceColor)
			os.Setenv("TERM", origTerm)
		}()

		if DetectColorSupport() {
			t.Error("expected no color when NO_COLOR is set")
		}
	})

	t.Run("FORCE_COLOR enables", func(t *testing.T) {
		origNoColor := os.Getenv("NO_COLOR")
		origForceColor := os.Getenv("FORCE_COLOR")
		origTerm := os.Getenv("TERM")

		os.Unsetenv("NO_COLOR")
		os.Setenv("FORCE_COLOR", "1")
		os.Setenv("TERM", "dumb")
		defer func() {
			os.Setenv("NO_COLOR", origNoColor)
			os.Setenv("FORCE_COLOR", origForceColor)
			os.Setenv("TERM", origTerm)
		}()

		if !DetectColorSupport() {
			t.Error("expected color when FORCE_COLOR is set")
		}
	})

	t.Run("dumb terminal", func(t *testing.T) {
		origNoColor := os.Getenv("NO_COLOR")
		origForceColor := os.Getenv("FORCE_COLOR")
		origTerm := os.Getenv("TERM")

		os.Unsetenv("NO_COLOR")
		os.Unsetenv("FORCE_COLOR")
		os.Setenv("TERM", "dumb")
		defer func() {
			os.Setenv("NO_COLOR", origNoColor)
			os.Setenv("FORCE_COLOR", origForceColor)
			os.Setenv("TERM", origTerm)
		}()

		if DetectColorSupport() {
			t.Error("expected no color for dumb terminal")
		}
	})
}

func TestDetectUnicodeSupport(t *testing.T) {
	t.Run("UTF-8 lang", func(t *testing.T) {
		origLang := os.Getenv("LANG")
		origLcAll := os.Getenv("LC_ALL")
		origLcCtype := os.Getenv("LC_CTYPE")

		os.Setenv("LANG", "en_US.UTF-8")
		os.Unsetenv("LC_ALL")
		os.Unsetenv("LC_CTYPE")
		defer func() {
			os.Setenv("LANG", origLang)
			os.Setenv("LC_ALL", origLcAll)
			os.Setenv("LC_CTYPE", origLcCtype)
		}()

		if !DetectUnicodeSupport() {
			t.Error("expected Unicode support with UTF-8 LANG")
		}
	})

	t.Run("no unicode", func(t *testing.T) {
		origLang := os.Getenv("LANG")
		origLcAll := os.Getenv("LC_ALL")
		origLcCtype := os.Getenv("LC_CTYPE")

		os.Setenv("LANG", "C")
		os.Unsetenv("LC_ALL")
		os.Unsetenv("LC_CTYPE")
		defer func() {
			os.Setenv("LANG", origLang)
			os.Setenv("LC_ALL", origLcAll)
			os.Setenv("LC_CTYPE", origLcCtype)
		}()

		if DetectUnicodeSupport() {
			t.Error("expected no Unicode support with LANG=C")
		}
	})
}
