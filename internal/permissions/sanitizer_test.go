package permissions

import (
	"strings"
	"testing"
)

func TestNewInputSanitizer(t *testing.T) {
	s := NewInputSanitizer()
	if s.MaxLength != 100000 {
		t.Errorf("expected MaxLength=100000, got %d", s.MaxLength)
	}
	if !s.StripInvisible {
		t.Error("expected StripInvisible=true")
	}
	if !s.NormalizeUnicode {
		t.Error("expected NormalizeUnicode=true")
	}
}

func TestSanitize_CleanInput(t *testing.T) {
	s := NewInputSanitizer()
	result := s.Sanitize("hello world")
	if result.WasModified {
		t.Error("expected clean input to not be modified")
	}
	if result.Clean != "hello world" {
		t.Errorf("expected 'hello world', got %q", result.Clean)
	}
	if len(result.Changes) != 0 {
		t.Errorf("expected no changes, got %d", len(result.Changes))
	}
}

func TestSanitize_NullBytes(t *testing.T) {
	s := NewInputSanitizer()
	result := s.Sanitize("hello\x00world")
	if !result.WasModified {
		t.Error("expected modified")
	}
	if result.Clean != "helloworld" {
		t.Errorf("expected 'helloworld', got %q", result.Clean)
	}
	foundNull := false
	for _, ch := range result.Changes {
		if ch.Type == "stripped" && ch.Original == "\\x00" {
			foundNull = true
			break
		}
	}
	if !foundNull {
		t.Error("expected a null byte stripped change")
	}
}

func TestSanitize_LineEndings(t *testing.T) {
	s := NewInputSanitizer()
	result := s.Sanitize("line1\r\nline2\r\nline3")
	if result.Clean != "line1\nline2\nline3" {
		t.Errorf("expected normalized line endings, got %q", result.Clean)
	}
	if !result.WasModified {
		t.Error("expected modified")
	}
}

func TestSanitize_LoneCarriageReturn(t *testing.T) {
	s := NewInputSanitizer()
	result := s.Sanitize("line1\rline2")
	if result.Clean != "line1\nline2" {
		t.Errorf("expected \\r replaced with \\n, got %q", result.Clean)
	}
}

func TestSanitize_Truncation(t *testing.T) {
	s := NewInputSanitizer()
	s.MaxLength = 10
	input := "abcdefghijklmnop"
	result := s.Sanitize(input)
	if len(result.Clean) != 10 {
		t.Errorf("expected length 10, got %d", len(result.Clean))
	}
	if result.Clean != "abcdefghij" {
		t.Errorf("expected 'abcdefghij', got %q", result.Clean)
	}
	foundTrunc := false
	for _, ch := range result.Changes {
		if ch.Type == "truncated" {
			foundTrunc = true
			break
		}
	}
	if !foundTrunc {
		t.Error("expected a truncated change")
	}
}

func TestSanitize_InvisibleChars(t *testing.T) {
	s := NewInputSanitizer()
	// Zero-width space (U+200B) between hello and world
	input := "hello" + string(rune(0x200B)) + "world"
	result := s.Sanitize(input)
	if result.Clean != "helloworld" {
		t.Errorf("expected 'helloworld', got %q", result.Clean)
	}
	if !result.WasModified {
		t.Error("expected modified")
	}
}

func TestSanitize_BOM(t *testing.T) {
	s := NewInputSanitizer()
	input := string(rune(0xFEFF)) + "hello"
	result := s.Sanitize(input)
	if result.Clean != "hello" {
		t.Errorf("expected 'hello', got %q", result.Clean)
	}
}

func TestSanitize_BidiOverrides(t *testing.T) {
	s := NewInputSanitizer()
	// Right-to-left override (U+202E)
	input := "normal" + string(rune(0x202E)) + "text"
	result := s.Sanitize(input)
	if result.Clean != "normaltext" {
		t.Errorf("expected 'normaltext', got %q", result.Clean)
	}
}

func TestSanitize_ANSI(t *testing.T) {
	s := NewInputSanitizer()
	input := "\033[31mred text\033[0m"
	result := s.Sanitize(input)
	if result.Clean != "red text" {
		t.Errorf("expected 'red text', got %q", result.Clean)
	}
}

func TestSanitize_HomoglyphMixed(t *testing.T) {
	s := NewInputSanitizer()
	// "hello" but with Cyrillic o (U+043E) after Latin "hell"
	input := "hell" + string(rune(0x043E))
	result := s.Sanitize(input)
	if result.Clean != "hello" {
		t.Errorf("expected 'hello', got %q", result.Clean)
	}
	foundNorm := false
	for _, ch := range result.Changes {
		if ch.Type == "normalized" && ch.Replacement == "o" {
			foundNorm = true
			break
		}
	}
	if !foundNorm {
		t.Error("expected a normalized homoglyph change")
	}
}

func TestSanitize_PureCyrillicUntouched(t *testing.T) {
	s := NewInputSanitizer()
	// Pure Cyrillic text should not be modified
	input := "Привет мир" // "Привет мир"
	result := s.Sanitize(input)
	if result.Clean != input {
		t.Errorf("expected pure Cyrillic to be unchanged, got %q", result.Clean)
	}
}

func TestSanitize_StripInvisibleDisabled(t *testing.T) {
	s := NewInputSanitizer()
	s.StripInvisible = false
	zwsp := string(rune(0x200B))
	input := "hello" + zwsp + "world"
	result := s.Sanitize(input)
	if !strings.Contains(result.Clean, zwsp) {
		t.Error("expected zero-width space to remain when StripInvisible is false")
	}
}

func TestSanitize_NormalizeUnicodeDisabled(t *testing.T) {
	s := NewInputSanitizer()
	s.NormalizeUnicode = false
	// Mixed script but normalization disabled
	input := "hell" + string(rune(0x043E)) // Cyrillic o
	result := s.Sanitize(input)
	if result.Clean != input {
		t.Error("expected input unchanged when NormalizeUnicode is false")
	}
}

func TestStripInvisibleChars(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
		changes  int
	}{
		{"no invisible", "hello", "hello", 0},
		{"zero-width space", "hel" + string(rune(0x200B)) + "lo", "hello", 1},
		{"BOM", string(rune(0xFEFF)) + "text", "text", 1},
		{"multiple", string(rune(0x200B)) + string(rune(0x200C)) + string(rune(0x200D)) + "hello", "hello", 3},
		{"bidi override", "a" + string(rune(0x202E)) + "b", "ab", 1},
		{"word joiner", "word" + string(rune(0x2060)) + "joiner", "wordjoiner", 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, changes := StripInvisibleChars(tt.input)
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
			if len(changes) != tt.changes {
				t.Errorf("expected %d changes, got %d", tt.changes, len(changes))
			}
		})
	}
}

func TestStripInvisibleChars_TagChars(t *testing.T) {
	// Tag characters U+E0001 to U+E007F
	input := "hello" + string(rune(0xE0001)) + "world"
	result, changes := StripInvisibleChars(input)
	if result != "helloworld" {
		t.Errorf("expected 'helloworld', got %q", result)
	}
	if len(changes) != 1 {
		t.Errorf("expected 1 change, got %d", len(changes))
	}
}

func TestNormalizeHomoglyphs(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
		changes  int
	}{
		{"pure latin", "hello", "hello", 0},
		{"pure cyrillic", "привет", "привет", 0},
		{"mixed - cyrillic o", "hell" + string(rune(0x043E)), "hello", 1},
		{"mixed - cyrillic c", "a" + string(rune(0x0441)) + "t", "act", 1},
		{"mixed - multiple", string(rune(0x0430)) + string(rune(0x0435)) + "st", "aest", 2},
		{"no homoglyphs in mixed", "aб", "aб", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, changes := NormalizeHomoglyphs(tt.input)
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
			if len(changes) != tt.changes {
				t.Errorf("expected %d changes, got %d", tt.changes, len(changes))
			}
		})
	}
}

func TestStripANSI(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"no ansi", "hello world", "hello world"},
		{"red text", "\033[31mhello\033[0m", "hello"},
		{"bold", "\033[1mbold\033[0m", "bold"},
		{"multiple", "\033[1;31mred bold\033[0m normal \033[32mgreen\033[0m", "red bold normal green"},
		{"cursor move", "\033[2Ahello", "hello"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := StripANSI(tt.input)
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestDetectEncoding(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"empty", "", "ascii"},
		{"ascii", "hello world 123", "ascii"},
		{"utf8", "hello мир", "utf8"},
		{"binary with null", "hello\x00world", "binary"},
		{"mixed invalid", "hello\xff\xfeworld", "mixed"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := DetectEncoding(tt.input)
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestSanitizeFilePath(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{"simple", "file.txt", "file.txt", false},
		{"subdir", "dir/file.txt", "dir/file.txt", false},
		{"backslash", "dir\\file.txt", "dir/file.txt", false},
		{"traversal", "../etc/passwd", "", true},
		{"embedded traversal", "foo/../../etc/passwd", "", true},
		{"null byte", "file\x00.txt", "", true},
		{"empty", "", "", true},
		{"dot dot", "..", "", true},
		{"current dir", "./file.txt", "file.txt", false},
		{"redundant slashes", "dir//file.txt", "dir/file.txt", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := SanitizeFilePath(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("SanitizeFilePath(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
				return
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("SanitizeFilePath(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestSanitizeJSON(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			"no dangerous keys",
			`{"name": "test", "value": 42}`,
			`{"name": "test", "value": 42}`,
		},
		{
			"__proto__",
			`{"name": "test", "__proto__": {"admin": true}}`,
			`{"name": "test"}`,
		},
		{
			"constructor",
			`{"constructor": "evil", "name": "ok"}`,
			`{"name": "ok"}`,
		},
		{
			"prototype",
			`{"prototype": "bad", "data": "good"}`,
			`{"data": "good"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := SanitizeJSON(tt.input)
			// Normalize spaces for comparison
			result = strings.TrimSpace(result)
			expected := strings.TrimSpace(tt.expected)
			if result != expected {
				t.Errorf("SanitizeJSON(%q)\ngot:  %q\nwant: %q", tt.input, result, expected)
			}
		})
	}
}

func TestFormatChanges_NoChanges(t *testing.T) {
	result := &SanitizeResult{
		Changes: nil,
	}
	output := FormatChanges(result)
	if output != "Input clean, no changes needed." {
		t.Errorf("unexpected output: %q", output)
	}
}

func TestFormatChanges_WithChanges(t *testing.T) {
	result := &SanitizeResult{
		Changes: []SanitizeChange{
			{Type: "stripped", Position: 15, Original: "U+200B", Replacement: ""},
			{Type: "stripped", Position: 42, Original: "U+200C", Replacement: ""},
			{Type: "normalized", Position: 28, Original: "о", Replacement: "o"},
			{Type: "stripped", Position: 5, Original: "\033[31m", Replacement: ""},
		},
	}
	output := FormatChanges(result)
	if !strings.Contains(output, "4 changes") {
		t.Errorf("expected '4 changes' in output: %q", output)
	}
	if !strings.Contains(output, "zero-width") {
		t.Errorf("expected 'zero-width' in output: %q", output)
	}
	if !strings.Contains(output, "Cyrillic homoglyph") {
		t.Errorf("expected 'Cyrillic homoglyph' in output: %q", output)
	}
	if !strings.Contains(output, "ANSI") {
		t.Errorf("expected 'ANSI' in output: %q", output)
	}
}

func TestSanitize_CombinedAttack(t *testing.T) {
	s := NewInputSanitizer()
	// Simulate a combined attack: invisible chars + homoglyphs + ANSI
	// ZWS + \033[31m + "hell" + Cyrillic o + " w" + Cyrillic o + "rld" + \033[0m + ZWNJ
	input := string(rune(0x200B)) + "\033[31mhell" + string(rune(0x043E)) + " w" + string(rune(0x043E)) + "rld\033[0m" + string(rune(0x200C))
	result := s.Sanitize(input)
	if result.Clean != "hello world" {
		t.Errorf("expected 'hello world', got %q", result.Clean)
	}
	if !result.WasModified {
		t.Error("expected modified")
	}
}

func TestSanitize_ConcurrentAccess(t *testing.T) {
	s := NewInputSanitizer()
	done := make(chan struct{})

	input := "hello" + string(rune(0x200B)) + "world\033[31m test\033[0m"
	expected := "helloworld test"

	for i := 0; i < 100; i++ {
		go func() {
			defer func() { done <- struct{}{} }()
			result := s.Sanitize(input)
			if result.Clean != expected {
				t.Errorf("concurrent sanitize failed: got %q", result.Clean)
			}
		}()
	}

	for i := 0; i < 100; i++ {
		<-done
	}
}

func TestSanitize_EmptyInput(t *testing.T) {
	s := NewInputSanitizer()
	result := s.Sanitize("")
	if result.WasModified {
		t.Error("expected empty input to not be modified")
	}
	if result.Clean != "" {
		t.Errorf("expected empty string, got %q", result.Clean)
	}
}

func TestSanitize_PreservesValidUnicode(t *testing.T) {
	s := NewInputSanitizer()
	// Emoji and CJK should be preserved
	input := "Hello \U0001F30D 世界"
	result := s.Sanitize(input)
	if result.Clean != input {
		t.Errorf("expected valid unicode preserved, got %q", result.Clean)
	}
	if result.WasModified {
		t.Error("expected no modification for valid unicode")
	}
}

func BenchmarkSanitize_Clean(b *testing.B) {
	s := NewInputSanitizer()
	input := strings.Repeat("hello world ", 100)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s.Sanitize(input)
	}
}

func BenchmarkSanitize_Dirty(b *testing.B) {
	s := NewInputSanitizer()
	input := strings.Repeat("hel"+string(rune(0x200B))+"l"+string(rune(0x043E))+" \033[31mworld\033[0m ", 100)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s.Sanitize(input)
	}
}

func BenchmarkStripInvisibleChars(b *testing.B) {
	input := strings.Repeat("hello"+string(rune(0x200B))+string(rune(0x200C))+string(rune(0x200D))+"world", 100)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		StripInvisibleChars(input)
	}
}

func BenchmarkNormalizeHomoglyphs(b *testing.B) {
	input := strings.Repeat("hell"+string(rune(0x043E))+" w"+string(rune(0x043E))+"rld ", 100)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		NormalizeHomoglyphs(input)
	}
}
