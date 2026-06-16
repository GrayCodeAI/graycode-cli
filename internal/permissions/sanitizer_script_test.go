//nolint:errcheck
package permissions

import (
	"strings"
	"testing"
	"unicode/utf8"
)

// --- isLegitimateScript unit tests ---

func TestIsLegitimateScript_Latin(t *testing.T) {
	if !isLegitimateScript('A') {
		t.Error("expected 'A' (Latin) to be a legitimate script")
	}
	if !isLegitimateScript('z') {
		t.Error("expected 'z' (Latin) to be a legitimate script")
	}
}

func TestIsLegitimateScript_CJK(t *testing.T) {
	// 中 = U+4E2D (Han)
	if !isLegitimateScript('中') {
		t.Error("expected '中' (Han/CJK) to be a legitimate script")
	}
	// あ = U+3042 (Hiragana)
	if !isLegitimateScript('あ') {
		t.Error("expected 'あ' (Hiragana) to be a legitimate script")
	}
	// ア = U+30A2 (Katakana)
	if !isLegitimateScript('ア') {
		t.Error("expected 'ア' (Katakana) to be a legitimate script")
	}
}

func TestIsLegitimateScript_Hangul(t *testing.T) {
	// 한 = U+D55C (Hangul)
	if !isLegitimateScript('한') {
		t.Error("expected '한' (Hangul) to be a legitimate script")
	}
}

func TestIsLegitimateScript_Arabic(t *testing.T) {
	// م = U+0645 (Arabic)
	if !isLegitimateScript('م') {
		t.Error("expected 'م' (Arabic) to be a legitimate script")
	}
}

func TestIsLegitimateScript_Hebrew(t *testing.T) {
	// א = U+05D0 (Hebrew)
	if !isLegitimateScript('א') {
		t.Error("expected 'א' (Hebrew) to be a legitimate script")
	}
}

func TestIsLegitimateScript_CommonNotLegitimate(t *testing.T) {
	// ASCII punctuation is in "Common" script — not in the allow-list.
	if isLegitimateScript(' ') {
		t.Error("expected ' ' (Common) to NOT be a legitimate script")
	}
	if isLegitimateScript('!') {
		t.Error("expected '!' (Common) to NOT be a legitimate script")
	}
	// Zero-width space is in Common — not legitimate per the rule.
	if isLegitimateScript('\u200B') {
		t.Error("expected ZWS (Common) to NOT be a legitimate script")
	}
}

// --- StripInvisibleChars integration tests ---

// TestStripInvisibleChars_LatinStillStrips: the regression guard.
// Latin text with U+200B (zero-width space) is still stripped.
func TestStripInvisibleChars_LatinStillStrips(t *testing.T) {
	in := "He\u200Bllo" // "He" + ZWS + "llo"
	out, changes := StripInvisibleChars(in)
	if out != "Hello" {
		t.Errorf("out = %q, want %q", out, "Hello")
	}
	if len(changes) != 1 {
		t.Errorf("changes = %d, want 1 (ZWS stripped)", len(changes))
	}
}

// TestStripInvisibleChars_KhmerVowelPreserved: U+17B4 was in the
// invisibleRunes list (mislabeled as "Khmer vowel inherent Aq"),
// but it's a visible Khmer vowel. Stripping it mangles Khmer text.
// The allow-list fix keeps it.
func TestStripInvisibleChars_KhmerVowelPreserved(t *testing.T) {
	// ា = U+17B6 (Khmer vowel sign AA) — adjacent in range
	// to the mislabeled 0x17B4. We test with the actual 0x17B4.
	in := string([]rune{0x17B4}) // Khmer vowel inherent AQ
	out, changes := StripInvisibleChars(in)
	if out != in {
		t.Errorf("out = %q, want %q (Khmer vowel should be preserved)", out, in)
	}
	if len(changes) != 0 {
		t.Errorf("changes = %d, want 0 (no strip for legitimate script)", len(changes))
	}
}

// TestStripInvisibleChars_HangulFillerPreserved: U+115F was in the
// invisibleRunes list as "hangul choseong filler", but it's a visible
// Hangul filler character. The allow-list keeps it.
func TestStripInvisibleChars_HangulFillerPreserved(t *testing.T) {
	in := string([]rune{0x115F}) // Hangul choseong filler
	out, changes := StripInvisibleChars(in)
	if out != in {
		t.Errorf("out = %q, want %q (Hangul filler should be preserved)", out, in)
	}
	if len(changes) != 0 {
		t.Errorf("changes = %d, want 0", len(changes))
	}
}

// TestStripInvisibleChars_ArabicLetterMarkPreserved: U+061C is the
// Arabic letter mark. The original list treated it as invisible;
// the allow-list fix preserves it in Arabic text.
func TestStripInvisibleChars_ArabicLetterMarkPreserved(t *testing.T) {
	in := "مرحبا" + string([]rune{0x061C}) + "بالعالم" // "مرحبا" + ALM + "بالعالم"
	out, changes := StripInvisibleChars(in)
	if out != in {
		t.Errorf("out = %q, want %q (Arabic letter mark should be preserved in Arabic text)", out, in)
	}
	if len(changes) != 0 {
		t.Errorf("changes = %d, want 0 (Arabic text preserved)", len(changes))
	}
}

// TestStripInvisibleChars_CJKPreserved: a CJK-only string is
// returned unchanged (no invisible chars to strip).
func TestStripInvisibleChars_CJKPreserved(t *testing.T) {
	in := "中文测试"
	out, changes := StripInvisibleChars(in)
	if out != in {
		t.Errorf("out = %q, want %q", out, in)
	}
	if len(changes) != 0 {
		t.Errorf("changes = %d, want 0 (CJK has no invisible chars)", len(changes))
	}
}

// TestStripInvisibleChars_TagCharactersAlwaysStrip: U+E0001-U+E007F
// (tag block) is always stripped, regardless of script. These are
// pure injection vectors with no legitimate use.
func TestStripInvisibleChars_TagCharactersAlwaysStrip(t *testing.T) {
	in := "before" + string([]rune{0xE0001}) + "after"
	out, changes := StripInvisibleChars(in)
	if out != "beforeafter" {
		t.Errorf("out = %q, want %q", out, "beforeafter")
	}
	if len(changes) != 1 {
		t.Errorf("changes = %d, want 1 (tag char stripped)", len(changes))
	}
}

// TestStripInvisibleChars_LatinWithZWSStripped: regression guard —
// pure zero-width space (Common script) is still stripped.
func TestStripInvisibleChars_LatinWithZWSStripped(t *testing.T) {
	cases := []struct {
		name string
		r    rune
	}{
		{"zero-width space", '\u200B'},
		{"zero-width non-joiner", '\u200C'},
		{"zero-width joiner", '\u200D'},
		{"BOM", '\uFEFF'},
		{"left-to-right mark", '\u200E'},
		{"right-to-left mark", '\u200F'},
		{"word joiner", '\u2060'},
		{"invisible separator", '\u2063'},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			in := "A" + string(tc.r) + "B"
			out, changes := StripInvisibleChars(in)
			if out != "AB" {
				t.Errorf("out = %q, want %q", out, "AB")
			}
			if len(changes) != 1 {
				t.Errorf("changes = %d, want 1 (%s should be stripped)", len(changes), tc.name)
			}
		})
	}
}

// TestStripInvisibleChars_MixedLatinAndCJKWithZWS: ZWS is in Common
// (not in the allow-list) and is stripped. The CJK chars are
// preserved (they're not invisible to begin with).
func TestStripInvisibleChars_MixedLatinAndCJKWithZWS(t *testing.T) {
	in := "Hello\u200B世界"
	out, changes := StripInvisibleChars(in)
	want := "Hello世界"
	if out != want {
		t.Errorf("out = %q, want %q", out, want)
	}
	if len(changes) != 1 {
		t.Errorf("changes = %d, want 1 (only ZWS stripped)", len(changes))
	}
}

// TestStripInvisibleChars_PreservesLength: the position tracking
// in SanitizeChange should match byte offsets in the ORIGINAL text.
func TestStripInvisibleChars_PreservesLength(t *testing.T) {
	in := "He\u200Bllo" // "He" + ZWS(3 bytes UTF-8) + "llo"
	_, changes := StripInvisibleChars(in)
	if len(changes) != 1 {
		t.Fatalf("changes = %d, want 1", len(changes))
	}
	// ZWS is at position 2 (right after "He" which is 2 bytes).
	if changes[0].Position != 2 {
		t.Errorf("Position = %d, want 2 (after 'He')", changes[0].Position)
	}
}

// TestStripInvisibleChars_LegitimateScriptHasNoChanges: a string
// of pure legitimate-script characters returns zero changes.
func TestStripInvisibleChars_LegitimateScriptHasNoChanges(t *testing.T) {
	// "ABC" + 中 + "あ" + 한 + "م" + א — all legitimate scripts.
	in := "ABC中あ한מא"
	_, changes := StripInvisibleChars(in)
	if len(changes) != 0 {
		t.Errorf("changes = %d, want 0 (all scripts are legitimate)", len(changes))
	}
}

// TestStripInvisibleChars_PurelyVisibleTextNoChanges: the most
// common case — plain text with no invisible chars returns zero
// changes. (Regression guard for the early-exit path.)
func TestStripInvisibleChars_PurelyVisibleTextNoChanges(t *testing.T) {
	in := "Hello, world! 123."
	out, changes := StripInvisibleChars(in)
	if out != in {
		t.Errorf("out = %q, want %q", out, in)
	}
	if len(changes) != 0 {
		t.Errorf("changes = %d, want 0 (no invisible chars)", len(changes))
	}
}

// TestStripInvisibleChars_Empty: empty string returns empty.
func TestStripInvisibleChars_Empty(t *testing.T) {
	out, changes := StripInvisibleChars("")
	if out != "" {
		t.Errorf("out = %q, want empty", out)
	}
	if len(changes) != 0 {
		t.Errorf("changes = %d, want 0", len(changes))
	}
}

// TestLegitimateScriptTables_ContainCoreScripts: sanity check on
// the allow-list itself. If a script is removed, this test catches
// the unintended removal at the test layer.
func TestLegitimateScriptTables_ContainCoreScripts(t *testing.T) {
	required := []struct {
		name string
		r    rune
	}{
		{"Latin", 'A'},
		{"Cyrillic", 'Б'},
		{"Greek", 'Ω'},
		{"Han", '中'},
		{"Hiragana", 'あ'},
		{"Katakana", 'ア'},
		{"Hangul", '한'},
		{"Arabic", 'م'},
		{"Hebrew", 'א'},
		{"Devanagari", 'अ'},
		{"Thai", 'ก'},
		{"Khmer", 'ក'},
	}
	for _, tc := range required {
		if !isLegitimateScript(tc.r) {
			t.Errorf("isLegitimateScript(%q = %U) is false; %s script is missing from the allow-list", tc.r, tc.r, tc.name)
		}
	}
}

// TestStripInvisibleChars_BytesValid: every output must be valid UTF-8
// (no partial codepoints). Catches accidental mid-rune splits.
func TestStripInvisibleChars_BytesValid(t *testing.T) {
	cases := []string{
		"He\u200Bllo",
		"Hello\u200B",
		"\u200BHello",
		"中\u200B文",
		"中\u17B4文",  // Khmer vowel in CJK context — preserved
		"مرحبا\u061Cبالعالم", // Arabic with ALM — preserved
		"mixed\u200E\u200Ftext",
	}
	for _, in := range cases {
		out, _ := StripInvisibleChars(in)
		if !utf8.ValidString(out) {
			t.Errorf("StripInvisibleChars(%q) = %q is not valid UTF-8", in, out)
		}
	}
}

// TestStripInvisibleChars_ChangesAreDeterministic: same input →
// same output and same changes. Regression guard.
func TestStripInvisibleChars_ChangesAreDeterministic(t *testing.T) {
	in := "He\u200Bllo\u200Cworld\u200D\u17B4"
	out1, ch1 := StripInvisibleChars(in)
	out2, ch2 := StripInvisibleChars(in)
	if out1 != out2 {
		t.Errorf("non-deterministic output: %q vs %q", out1, out2)
	}
	if len(ch1) != len(ch2) {
		t.Errorf("non-deterministic change count: %d vs %d", len(ch1), len(ch2))
	}
	for i := range ch1 {
		if ch1[i] != ch2[i] {
			t.Errorf("non-deterministic change %d: %+v vs %+v", i, ch1[i], ch2[i])
		}
	}
}

// TestStripInvisibleChars_OnlyModified: changes list contains
// only entries for chars that were actually stripped.
func TestStripInvisibleChars_OnlyModified(t *testing.T) {
	in := "He\u200Bllo"
	_, changes := StripInvisibleChars(in)
	for _, ch := range changes {
		if ch.Type != "stripped" {
			t.Errorf("change %+v has unexpected type %q", ch, ch.Type)
		}
	}
}

// TestStripInvisibleChars_ChangeOrigIsSingleChar: the Original
// field is a single formatted codepoint, not the surrounding text.
func TestStripInvisibleChars_ChangeOrigIsSingleChar(t *testing.T) {
	in := "A\u200BB"
	_, changes := StripInvisibleChars(in)
	if len(changes) != 1 {
		t.Fatalf("changes = %d, want 1", len(changes))
	}
	if !strings.HasPrefix(changes[0].Original, "U+") {
		t.Errorf("Original = %q, want U+... format", changes[0].Original)
	}
	if changes[0].Original != "U+200B" {
		t.Errorf("Original = %q, want U+200B", changes[0].Original)
	}
}
