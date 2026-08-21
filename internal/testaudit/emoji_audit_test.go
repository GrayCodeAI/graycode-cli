package testaudit

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// emojiAuditExempt files in cmd/ that are allowed to contain non-ASCII
// glyph literals for legitimate reasons:
//
//   - braille_spinner.go / spinner_wave.go — TUI spinner frames are
//     intentionally a small alphabet of U+25xx/U+28xx block characters.
//     The icons package is not used here because the spinner is rendered
//     inside a per-frame ANSI wave that is repainted 20 times a second.
//   - welcome_banner.go — HAWK block-letter logo, U+2588 block elements.
//   - *_test.go — tests are allowed to assert on rendered output.
var emojiAuditExempt = map[string]bool{
	"braille_spinner.go":      true,
	"braille_spinner_test.go": true,
	"spinner_wave.go":         true,
	"spinner_wave_test.go":    true,
	"welcome_banner.go":       true,
}

// emojiAuditPathExempt covers internal packages that are NOT part of
// the cmd/ user-facing surface and may legitimately need raw glyphs
// (e.g. markdown.go parses markdown and emits "─", "│", "•" as syntax).
var emojiAuditPathExempt = []string{
	"/internal/markdown/",
	"/internal/multiagent/agents/",
	"/internal/ui/icons/", // The icon registry itself holds the PUA codepoints.
	"/internal/testaudit/",
	// test_loop.go scans external compiler / test-runner stdout for
	// status glyphs (✓/✗/✔/✕/✅/❌/⚠) as part of its test-result
	// parser vocabulary. The literals are not hawk's own output; they
	// appear in compiler messages the parser has to recognise.
	"/internal/engine/validation/test_loop.go",
	// test_fixtures.go contains canned compiler / linter output used
	// as fixture data for parser tests; the emoji are part of the
	// captured external tool output, not hawk's own rendering.
	"/internal/tool/test_fixtures.go",
	// framesdir.go matches the literal "❯" prompt glyph inside replayed fx
	// snapshots to classify input rows when exporting frame artifacts. The
	// glyph is fx's own prompt character embedded in captured output, not
	// hawk's rendering, so it is recognised rather than produced.
	"/internal/terminal/tape/",
}

// isEmojiOrDingbat reports whether r is a glyph that should never appear
// in user-facing code without going through internal/ui/icons. It blocks:
//
//   - U+1F300–U+1FAFF — Emoji & Supplemental Symbols and Pictographs
//   - U+2600–U+27BF   — Miscellaneous Symbols / Dingbats
//   - U+2300–U+23FF   — Miscellaneous Technical (used by some emoji-style icons)
//   - U+2700–U+27BF   — Dingbats (overlap with above, kept for clarity)
//
// Allowed: PUA (U+E000–U+F8FF), ASCII, Latin punctuation, arrows
// (U+2190–U+21FF), mathematical operators (U+2200–U+22FF), and general
// punctuation (U+2000–U+206F) including em-dash, ellipsis, middle dot.
func isEmojiOrDingbat(r rune) bool {
	switch {
	case r >= 0x1F300 && r <= 0x1FAFF:
		return true
	case r >= 0x2600 && r <= 0x27BF:
		return true
	case r >= 0x2300 && r <= 0x23FF:
		return true
	}
	return false
}

// TestNoEmojiInCmd enforces hawk's "no emoji in CLI" rule. Every glyph
// that hawk renders must come from internal/ui/icons. This test scans
// every non-test .go file in cmd/, parses it, and reports any literal
// rune in the emoji or dingbat blocks.
//
// The test FAILS CI when violations are present; the output names each
// file:line and the offending runes. It runs in <500ms for the entire
// cmd/ tree.
func TestNoEmojiInCmd(t *testing.T) {
	root := repoRoot(t)
	cmdDir := filepath.Join(root, "cmd")
	if _, err := os.Stat(cmdDir); err != nil {
		t.Skipf("cmd/ not found at %s: %v", cmdDir, err)
	}

	var files []parsedFile
	err := filepath.WalkDir(cmdDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		name := filepath.Base(path)
		if !strings.HasSuffix(name, ".go") {
			return nil
		}
		if emojiAuditExempt[name] {
			return nil
		}
		if strings.HasSuffix(name, "_test.go") {
			return nil
		}
		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
		if err != nil {
			t.Logf("warning: cannot parse %s: %v", path, err)
			return nil
		}
		files = append(files, parsedFile{Path: path, File: f, FSet: fset})
		return nil
	})
	if err != nil {
		t.Fatalf("walk cmd/: %v", err)
	}

	violations := 0
	for _, pf := range files {
		rel := relPath(root, pf.Path)
		ast.Inspect(pf.File, func(n ast.Node) bool {
			bl, ok := n.(*ast.BasicLit)
			if !ok {
				return true
			}
			if bl.Kind != token.STRING {
				return true
			}
			// Strip Go string quotes.
			raw := bl.Value
			if len(raw) < 2 {
				return true
			}
			inner := raw[1 : len(raw)-1]
			decoded, err := strconvUnquote(inner)
			if err != nil {
				// Raw-string literals (backticks) are returned as-is
				// by strconvUnquote returning an error; fall through
				// to byte-level scan.
				decoded = stripRawString(inner)
			}
			for i, r := range decoded {
				if !isEmojiOrDingbat(r) {
					continue
				}
				pos := pf.FSet.Position(bl.Pos())
				// Offset by literal start within the line.
				pos.Offset += i
				lineCol := pf.FSet.Position(token.Pos(pos.Offset))
				violations++
				t.Run(
					strings.ReplaceAll(rel, "/", "_")+"_"+lineCol.String(),
					func(t *testing.T) {
						t.Errorf("Emoji/dingbat rune %U %q at %s:%d — must go through internal/ui/icons",
							r, string(r), rel, lineCol.Line)
					},
				)
			}
			return true
		})
	}

	if violations == 0 {
		t.Logf("scanned %d files in cmd/; 0 emoji violations", len(files))
	}
}

// TestNoEmojiInInternalExceptIcons is a softer audit for internal/ — it
// fails only for the dingbat/emoji ranges, and allows packages on the
// emojiAuditPathExempt list (markdown, multiagent prompts, icons itself).
func TestNoEmojiInInternalExceptIcons(t *testing.T) {
	root := repoRoot(t)
	internalDir := filepath.Join(root, "internal")
	files := parseInternalPackages(t, internalDir)

	violations := 0
	for _, pf := range files {
		rel := relPath(root, pf.Path)
		skip := false
		for _, ex := range emojiAuditPathExempt {
			if strings.Contains(pf.Path, ex) {
				skip = true
				break
			}
		}
		if skip {
			continue
		}

		ast.Inspect(pf.File, func(n ast.Node) bool {
			bl, ok := n.(*ast.BasicLit)
			if !ok {
				return true
			}
			if bl.Kind != token.STRING {
				return true
			}
			raw := bl.Value
			if len(raw) < 2 {
				return true
			}
			inner := raw[1 : len(raw)-1]
			decoded, err := strconvUnquote(inner)
			if err != nil {
				decoded = stripRawString(inner)
			}
			for _, r := range decoded {
				if !isEmojiOrDingbat(r) {
					continue
				}
				pos := pf.FSet.Position(bl.Pos())
				violations++
				t.Errorf("emoji/dingbat rune %U at %s:%d — use internal/ui/icons", r, rel, pos.Line)
			}
			return true
		})
	}

	if violations == 0 {
		t.Logf("scanned %d files in internal/; 0 emoji violations outside exempt packages", len(files))
	}
}

// strconvUnquote wraps strconv.Unquote so the rest of the audit code can
// stay free of strconv imports.
func strconvUnquote(s string) (string, error) {
	return strconv.Unquote("\"" + s + "\"")
}

// stripRawString returns the body of a Go raw string literal (`...`) with
// surrounding backticks already stripped. Since raw strings cannot contain
// backticks, no further processing is needed.
func stripRawString(s string) string {
	return s
}
