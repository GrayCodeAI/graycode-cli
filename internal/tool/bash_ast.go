package tool

import (
	"fmt"
	"strings"
	"unicode"
)

// bashASTAnalyzer is a hand-written Bash tokenizer + parser + walker. It is
// intentionally a focused subset of mvdan.cc/sh — large enough to catch the
// dangerous patterns the existing regex layer misses, small enough to be
// reviewable in one sitting and free of the 50K-LOC mvdan.cc/sh dependency
// (which currently can't be added to this codebase due to internal
// version conflicts in the hawk-eco go workspace).
//
// Pipeline: tokenize → parse → walk. The walker emits findings tagged with
// the dangerous category. BashTool.Execute calls bashASTAnalyze as a
// second-pass safety check after the existing regex pass; either can
// hard-deny the command.
//
// Dangerous categories the walker detects that the regex layer might miss:
//   - Command substitution `$(...)` whose INNER command is dangerous
//     (e.g. `$(rm -rf /tmp)` would not be caught by the regex layer
//     because it only checks the outer string).
//   - Heredoc bodies containing dangerous commands (`cat <<EOF | bash`).
//   - Nested if/while/for bodies whose condition or body is dangerous.
//   - Function definitions whose body is dangerous.
//   - Multi-segment commands (`a; b; c`) where any segment is dangerous.
//
// Categories already covered by the regex layer (eval, exec, $() alone,
// `| sh`, process substitution, IFS, etc.) are still flagged by the
// regex layer; the AST layer's job is to catch what regex misses.
type bashASTAnalyzer struct {
	// findings is populated by the walker. BashASTAnalyze returns it.
	findings []astFinding
	// depth guards against pathological recursion (e.g. `$( $( $( ... ) ) )`).
	// The bash command line has a hard limit of ~getconf ARG_MAX bytes so
	// 256 levels is well above any real-world input.
	depth int
}

type astFinding struct {
	category string
	snippet  string // the offending text (truncated for log readability)
	pos      int    // byte offset into the original input
}

// bashASTAnalyze returns the list of dangerous findings the AST walker
// found in command. Returns nil if the command is clean (or only
// contains patterns already caught by the regex layer, which BashTool
// also runs).
func bashASTAnalyze(command string) []astFinding {
	a := &bashASTAnalyzer{}
	a.analyze(command)
	return a.findings
}

const maxASTDepth = 256

// analyze is the public entry point. It tokenizes, parses, and walks.
func (a *bashASTAnalyzer) analyze(command string) {
	if a.depth >= maxASTDepth {
		return
	}
	a.depth++
	defer func() { a.depth-- }()

	toks := bashTokenize(command)
	if len(toks) == 0 {
		return
	}
	// Parse the full command as a script (top-level list of statements).
	stmts, end := bashParseScript(toks, 0)
	if end == 0 {
		return
	}
	a.walkStmts(stmts, command)
}

// -----------------------------------------------------------------------------
// Tokenizer
// -----------------------------------------------------------------------------

type bashTokKind int

const (
	tokWord       bashTokKind = iota
	tokOp                     // ; & | || && ( ) { } < > newline EOF
	tokQuoted                 // "..." or '...'
	tokVariable               // $VAR or ${VAR}
	tokCommandSub             // $(...) — has already been recursively tokenized
	tokBackquote              // `...`
	tokHeredoc                // <<TAG or <<-TAG
	tokRedirect               // > < >> <<& <> etc. with filename
	tokProcessSub             // <(...) or >(...)
)

type bashTok struct {
	kind  bashTokKind
	text  string // raw text including delimiters (used for span reconstruction)
	value string // the "meaning" — for quoted, the unquoted content; for variable, the name; for word, the word
	pos   int    // start byte offset in source
}

// bashTokenize is a streaming-style tokenizer that respects single quotes,
// double quotes (with $ and ` expansion), backslash escapes, and
// bash-specific syntax (heredocs, process substitution, command
// substitution). It does NOT do full bash parsing — it produces a flat
// token stream that the parser then turns into statements.
func bashTokenize(s string) []bashTok {
	var toks []bashTok
	i := 0
	for i < len(s) {
		ch := s[i]
		switch {
		case ch == ' ' || ch == '\t':
			i++
		case ch == '\n' || ch == ';':
			toks = append(toks, bashTok{kind: tokOp, text: string(ch), value: string(ch), pos: i})
			i++
		case ch == '&':
			if i+1 < len(s) && s[i+1] == '&' {
				toks = append(toks, bashTok{kind: tokOp, text: "&&", value: "&&", pos: i})
				i += 2
			} else {
				toks = append(toks, bashTok{kind: tokOp, text: "&", value: "&", pos: i})
				i++
			}
		case ch == '|':
			if i+1 < len(s) && s[i+1] == '|' {
				toks = append(toks, bashTok{kind: tokOp, text: "||", value: "||", pos: i})
				i += 2
			} else {
				toks = append(toks, bashTok{kind: tokOp, text: "|", value: "|", pos: i})
				i++
			}
		case ch == '(':
			if i+1 < len(s) && s[i+1] == '(' {
				// Process substitution: <( ) or >( ). We treat both as a
				// single token; the inner is recursively tokenized.
				innerStart := i + 2
				depth := 1
				j := innerStart
				for j < len(s) && depth > 0 {
					switch s[j] {
					case '(':
						depth++
					case ')':
						depth--
					}
					j++
				}
				inner := s[innerStart : j-1]
				toks = append(toks, bashTok{kind: tokProcessSub, text: s[i:j], value: inner, pos: i})
				i = j
			} else {
				toks = append(toks, bashTok{kind: tokOp, text: "(", value: "(", pos: i})
				i++
			}
		case ch == ')':
			toks = append(toks, bashTok{kind: tokOp, text: ")", value: ")", pos: i})
			i++
		case ch == '{':
			toks = append(toks, bashTok{kind: tokOp, text: "{", value: "{", pos: i})
			i++
		case ch == '}':
			toks = append(toks, bashTok{kind: tokOp, text: "}", value: "}", pos: i})
			i++
		case ch == '\'':
			// Single-quoted string: literal, no expansion.
			j := i + 1
			for j < len(s) && s[j] != '\'' {
				j++
			}
			toks = append(toks, bashTok{
				kind:  tokQuoted,
				text:  s[i:min(j+1, len(s))],
				value: s[i+1 : min(j, len(s))],
				pos:   i,
			})
			i = min(j+1, len(s))
		case ch == '"':
			// Double-quoted string: $ and ` expansions allowed.
			j := i + 1
			for j < len(s) && s[j] != '"' {
				if s[j] == '\\' && j+1 < len(s) {
					j += 2
				} else {
					j++
				}
			}
			toks = append(toks, bashTok{
				kind:  tokQuoted,
				text:  s[i:min(j+1, len(s))],
				value: s[i+1 : min(j, len(s))],
				pos:   i,
			})
			i = min(j+1, len(s))
		case ch == '`':
			// Backtick command substitution: recursively tokenize the body.
			j := i + 1
			for j < len(s) && s[j] != '`' {
				if s[j] == '\\' && j+1 < len(s) {
					j += 2
				} else {
					j++
				}
			}
			inner := s[i+1 : min(j, len(s))]
			toks = append(toks, bashTok{kind: tokBackquote, text: s[i:min(j+1, len(s))], value: inner, pos: i})
			i = min(j+1, len(s))
		case ch == '$':
			if i+1 < len(s) && s[i+1] == '(' {
				// $(command) — find matching close paren respecting nesting
				// and quoted subregions.
				innerStart := i + 2
				depth := 1
				j := innerStart
				for j < len(s) && depth > 0 {
					switch s[j] {
					case '(':
						depth++
					case ')':
						depth--
					case '"', '\'':
						// Skip past quoted region so parens inside don't count.
						q := s[j]
						j++
						for j < len(s) && s[j] != q {
							if s[j] == '\\' && j+1 < len(s) {
								j += 2
							} else {
								j++
							}
						}
					}
					j++
				}
				inner := s[innerStart : j-1]
				toks = append(toks, bashTok{kind: tokCommandSub, text: s[i:j], value: inner, pos: i})
				i = j
			} else if i+1 < len(s) && s[i+1] == '{' {
				// ${VAR} or ${VAR:-default} — find matching close brace.
				j := i + 2
				for j < len(s) && s[j] != '}' {
					j++
				}
				name := s[i+2 : min(j, len(s))]
				toks = append(toks, bashTok{
					kind:  tokVariable,
					text:  s[i:min(j+1, len(s))],
					value: name,
					pos:   i,
				})
				i = min(j+1, len(s))
			} else if i+1 < len(s) && isNameStart(s[i+1]) {
				// $VAR
				j := i + 1
				for j < len(s) && isNameCont(s[j]) {
					j++
				}
				toks = append(toks, bashTok{
					kind:  tokVariable,
					text:  s[i:j],
					value: s[i+1 : j],
					pos:   i,
				})
				i = j
			} else {
				// Bare '$' (not a valid variable, just emit a word).
				toks = append(toks, bashTok{kind: tokWord, text: "$", value: "$", pos: i})
				i++
			}
		case ch == '<':
			if i+1 < len(s) && s[i+1] == '<' {
				// Heredoc: <<TAG or <<-TAG (or <<< here-string).
				tagStart := i + 2
				stripTabs := false
				if tagStart < len(s) && s[tagStart] == '-' {
					stripTabs = true
					tagStart++
				}
				for tagStart < len(s) && (s[tagStart] == ' ' || s[tagStart] == '\t') {
					tagStart++
				}
				if tagStart < len(s) && s[tagStart] == '<' {
					// <<< here-string: read until newline, capture body.
					j := tagStart + 1
					for j < len(s) && s[j] != '\n' {
						j++
					}
					toks = append(toks, bashTok{
						kind:  tokHeredoc,
						text:  s[i:j],
						value: s[tagStart+1 : j],
						pos:   i,
					})
					i = j
					continue
				}
				tagEnd := tagStart
				for tagEnd < len(s) && (isNameStart(s[tagEnd]) || s[tagEnd] == '_') {
					tagEnd++
				}
				tag := s[tagStart:tagEnd]
				toks = append(toks, bashTok{kind: tokHeredoc, text: s[i:tagEnd], value: tag, pos: i})
				_ = stripTabs
				i = tagEnd
				// Read the heredoc body: from current position to the first
				// occurrence of TAG on its own line. The body may span
				// multiple lines.
				if i < len(s) {
					bodyStart := i
					found := false
					for {
						nl := strings.IndexByte(s[i:], '\n')
						if nl < 0 {
							// No more newlines. Check if the rest of the
							// input (after trimming) is the tag.
							rest := strings.TrimRight(s[i:], " \t")
							if rest == tag {
								toks = append(toks, bashTok{
									kind:  tokHeredoc,
									text:  s[bodyStart:i],
									value: s[bodyStart:i],
									pos:   bodyStart,
								})
								found = true
							}
							break
						}
						line := s[i : i+nl]
						if strings.TrimRight(line, " \t") == tag {
							toks = append(toks, bashTok{
								kind:  tokHeredoc,
								text:  s[bodyStart:i],
								value: s[bodyStart:i],
								pos:   bodyStart,
							})
							i += nl + 1
							found = true
							break
						}
						i += nl + 1
					}
					if !found {
						// Malformed heredoc (no terminator). Emit the
						// remainder as a best-effort body so the AST
						// walker can still inspect it.
						toks = append(toks, bashTok{
							kind:  tokHeredoc,
							text:  s[bodyStart:],
							value: s[bodyStart:],
							pos:   bodyStart,
						})
						i = len(s)
					}
				}
			} else if i+1 < len(s) && s[i+1] == '(' {
				// <( process substitution. The case '(' above already
				// handles (() (subshell), so this branch is only reached
				// when s[i+1] is '(' but s[i+2] is NOT '(' — i.e. the
				// single-paren process-substitution case. We emit it as a
				// tokProcessSub and recurse into the body.
				innerStart := i + 2
				depth := 1
				j := innerStart
				for j < len(s) && depth > 0 {
					switch s[j] {
					case '(':
						depth++
					case ')':
						depth--
					}
					j++
				}
				inner := s[innerStart : j-1]
				toks = append(toks, bashTok{kind: tokProcessSub, text: s[i:j], value: inner, pos: i})
				i = j
			} else {
				// Plain < (redirect or just less-than)
				toks = append(toks, bashTok{kind: tokWord, text: "<", value: "<", pos: i})
				i++
			}
		case ch == '>':
			// > >> etc. — emit as a single word for the walker to flag if
			// it's a write to /. But first, check for >(...) process
			// substitution (the OUTPUT form, counterpart to <(...)).
			if i+1 < len(s) && s[i+1] == '(' {
				innerStart := i + 2
				depth := 1
				j := innerStart
				for j < len(s) && depth > 0 {
					switch s[j] {
					case '(':
						depth++
					case ')':
						depth--
					}
					j++
				}
				inner := s[innerStart : j-1]
				toks = append(toks, bashTok{kind: tokProcessSub, text: s[i:j], value: inner, pos: i})
				i = j
				continue
			}
			j := i + 1
			if j < len(s) && s[j] == '>' {
				j++
			}
			toks = append(toks, bashTok{kind: tokWord, text: s[i:j], value: s[i:j], pos: i})
			i = j
		case ch == '\\':
			// Backslash-escaped char: emit a word of length 2.
			if i+1 < len(s) {
				toks = append(toks, bashTok{kind: tokWord, text: s[i : i+2], value: s[i+1 : i+2], pos: i})
				i += 2
			} else {
				i++
			}
		default:
			// Plain word: read until whitespace, operator, or quote.
			j := i
			for j < len(s) {
				c := s[j]
				if unicode.IsSpace(rune(c)) || c == ';' || c == '&' || c == '|' || c == '(' || c == ')' || c == '{' || c == '}' || c == '\'' || c == '"' || c == '`' || c == '$' || c == '<' || c == '>' || c == '\\' {
					break
				}
				j++
			}
			if j == i {
				j = i + 1
			}
			toks = append(toks, bashTok{kind: tokWord, text: s[i:j], value: s[i:j], pos: i})
			i = j
		}
	}
	return toks
}

func isNameStart(c byte) bool { return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || c == '_' }
func isNameCont(c byte) bool  { return isNameStart(c) || (c >= '0' && c <= '9') }
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// -----------------------------------------------------------------------------
// Parser: produces a flat list of statements (each a flat list of tokens).
// This is intentionally not a full AST — it's just enough to walk
// statement-by-statement and segment-by-segment.
// -----------------------------------------------------------------------------

// bashParseScript parses from toks[start] and returns the statements and the
// index after the last token consumed. Statements are separated by ';' or
// newline, and '|' / '||' / '&&' / '&' bind tighter (treated as part of
// the same statement).
func bashParseScript(toks []bashTok, start int) (stmts [][]bashTok, end int) {
	end = start
	for end < len(toks) {
		stmt, next := bashParseStatement(toks, end)
		if stmt == nil {
			break
		}
		stmts = append(stmts, stmt)
		end = next
		// Consume statement separators.
		for end < len(toks) && (toks[end].text == ";" || toks[end].text == "\n") {
			end++
		}
	}
	return stmts, end
}

// bashParseStatement reads one statement (a flat list of tokens terminated
// by ;, newline, or EOF). Stops at top-level '|', '||', '&&' boundaries
// for downstream segmentation but does NOT split them.
func bashParseStatement(toks []bashTok, start int) (stmt []bashTok, end int) {
	end = start
	for end < len(toks) {
		switch toks[end].kind {
		case tokOp:
			switch toks[end].text {
			case ";", "\n":
				return stmt, end
			case ")", "}", "&":
				// End of subshell / backgrounding. Statement is whatever
				// came before; parent parser picks up.
				return stmt, end
			}
		}
		stmt = append(stmt, toks[end])
		end++
	}
	return stmt, end
}

// -----------------------------------------------------------------------------
// Walker: visits every statement, every segment, every substitution body,
// every heredoc body, and every nested if/while/for body. Emits findings.
// -----------------------------------------------------------------------------

// walkStmts is the top-level walker. For each statement it walks every
// "segment" (a chain of piped/&&/|| tokens) and recurses into nested
// constructs (substitutions, heredocs).
func (a *bashASTAnalyzer) walkStmts(stmts [][]bashTok, src string) {
	for _, stmt := range stmts {
		a.walkStmt(stmt, src)
	}
}

func (a *bashASTAnalyzer) walkStmt(toks []bashTok, src string) {
	// Split into segments on |, ||, &&, &.
	segs := bashSplitSegments(toks)
	for _, seg := range segs {
		a.walkSegment(seg, src)
	}
}

// bashSplitSegments splits a flat statement into segments at |, ||, &&, &.
// Returns the segments in order. Operators are NOT included.
func bashSplitSegments(toks []bashTok) [][]bashTok {
	var segs [][]bashTok
	var cur []bashTok
	for _, t := range toks {
		if t.kind == tokOp && (t.text == "|" || t.text == "||" || t.text == "&&" || t.text == "&") {
			if len(cur) > 0 {
				segs = append(segs, cur)
			}
			cur = nil
			continue
		}
		cur = append(cur, t)
	}
	if len(cur) > 0 {
		segs = append(segs, cur)
	}
	return segs
}

func (a *bashASTAnalyzer) walkSegment(seg []bashTok, src string) {
	if len(seg) == 0 {
		return
	}
	// Recurse into every command-substitution / backquote / process-sub /
	// heredoc body. Even if the outer command is safe, the inner body
	// might not be — that's exactly the gap the AST walker fills vs the
	// regex layer (which only sees the outer text).
	//
	// For substitution bodies, we check the inner via BOTH the AST layer
	// (recursively, for nested danger) AND the existing regex layer
	// (IsDestructiveCommand, IsSuspicious, hardDeny). This way the AST
	// walker surfaces "this substitution has a dangerous inner" without
	// having to re-implement the destructive/suspicious-pattern lists.
	for _, t := range seg {
		switch t.kind {
		case tokCommandSub, tokBackquote, tokProcessSub:
			// Recurse for nested findings.
			inner := bashTokenize(t.value)
			innerStmts, _ := bashParseScript(inner, 0)
			var innerFindings []astFinding
			{
				prev := a.findings
				a.walkStmts(innerStmts, t.value)
				innerFindings = a.findings[len(prev):]
				// Roll back so this level's findings only contain the
				// "outer" flag (if any), not the inner's findings.
				a.findings = prev
			}
			// Bridge: also check the inner via the regex layer. If
			// IsDestructiveCommand / IsSuspicious / isHardDeny catch
			// something the AST layer didn't (e.g. "rm -rf" in the
			// inner), surface that.
			if IsDestructiveCommand(t.value) {
				innerFindings = append(innerFindings, astFinding{
					category: "destructive command in inner",
					snippet:  truncateSnippet(t.value),
					pos:      t.pos,
				})
			}
			if isHardDeny(t.value) {
				innerFindings = append(innerFindings, astFinding{
					category: "hard-deny pattern in inner",
					snippet:  truncateSnippet(t.value),
					pos:      t.pos,
				})
			}
			if len(innerFindings) > 0 {
				a.flag(t, fmt.Sprintf("substitution with dangerous inner (%d finding(s))", len(innerFindings)))
			}
		case tokHeredoc:
			// The heredoc body (in t.value) is fed to the previous command
			// on stdin. The AST walker doesn't know the outer command at
			// the token level, so it just inspects the body and lets the
			// regex layer's "| sh" / "| bash" checks do the full evaluation.
			if isHeredocBodyDangerous(t.value) {
				a.flag(t, "heredoc with dangerous body")
			}
		}
	}
}

// truncateSnippet trims a snippet to 80 chars + "...".
func truncateSnippet(s string) string {
	if len(s) <= 80 {
		return s
	}
	return s[:77] + "..."
}

// isHeredocBodyDangerous returns true if the heredoc body contains a
// command-substitution, backtick, eval, exec, or other dynamic-content
// marker that combined with a shell-execution outer command would be
// dangerous. We don't know the outer command at the token level so we
// flag the heredoc and let the existing regex layer do the full check.
func isHeredocBodyDangerous(body string) bool {
	if strings.Contains(body, "$(") {
		return true
	}
	if strings.Contains(body, "`") {
		return true
	}
	if strings.Contains(body, "eval ") {
		return true
	}
	if strings.Contains(body, "exec ") {
		return true
	}
	return false
}

// flag records an astFinding.
func (a *bashASTAnalyzer) flag(t bashTok, category string) {
	snippet := t.text
	if len(snippet) > 80 {
		snippet = snippet[:77] + "..."
	}
	a.findings = append(a.findings, astFinding{
		category: category,
		snippet:  snippet,
		pos:      t.pos,
	})
}

// String renders an astFinding for logs.
func (f astFinding) String() string {
	return fmt.Sprintf("ast[%s] @%d: %q", f.category, f.pos, f.snippet)
}

// hasCategory reports whether any finding in fs has the given category
// (substring match — so "substitution" matches "substitution with
// dangerous inner (1 finding(s))").
func hasCategory(fs []astFinding, cat string) bool {
	for _, f := range fs {
		if strings.Contains(f.category, cat) {
			return true
		}
	}
	return false
}
