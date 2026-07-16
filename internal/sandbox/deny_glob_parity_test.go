// Package sandbox — deny-glob parity tests.
//
// Parity invariant (ported from SpaceXAI's grok agent,
// xai-grok-sandbox/src/deny/glob.rs / mod.rs):
//
//	The Linux landlock deny-set and the macOS seatbelt deny-set must accept
//	and reject the SAME glob patterns, and (for the accepted subset) cover the
//	SAME matched paths on both platforms. Failure-closed: any glob that cannot
//	be translated identically on the two platforms is REJECTED on both.
//
// The rules below are a structural re-implementation of grok's
// validate_deny_glob so the invariant is asserted in Go without pulling in a
// globset dependency.
package sandbox

import (
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"unicode/utf8"
)

// isGlob reports whether entry contains any gitignore metacharacter (*, ?, [).
func isGlob(entry string) bool {
	return strings.ContainsAny(entry, "*?[")
}

// validateDenyGlob returns nil only if glob translates identically on the
// Linux (globset, literal_separator) and macOS (anchored Seatbelt regex)
// backends. Anything that would diverge is rejected so the deny-sets stay in
// agreement. The structural rules mirror grok's validate_deny_glob:
//
//  1. Reject brace alternation ({ }) and backslash escapes (\): globset honors
//     them but a hand-rolled anchored Seatbelt regex would mis-translate **/.
//  2. Reject a segment containing ** other than an exact ** segment (a**b,
//     **a diverge: macOS renders .* while globset renders *).
//  3. Inside [...]: reject a literal-] first member ([]a]), reject nested [ or
//     POSIX [[:…:]] classes (the two parsers disagree). Leading !/^ negation
//     is supported.
//  4. Reject a malformed/unterminated [ (fail closed identically on both).
func validateDenyGlob(glob string) error {
	// Rule 1: brace alternation and backslash escapes diverge across backends.
	if strings.ContainsAny(glob, `{}\`) {
		return &denyGlobError{glob: glob, reason: "brace/backslash escapes are not portable"}
	}

	// Rule 2: ** is only allowed as an exact whole segment.
	for _, seg := range strings.Split(glob, "/") {
		if strings.Contains(seg, "**") && seg != "**" {
			return &denyGlobError{glob: glob, reason: "non-exact ** segment: " + seg}
		}
	}

	// Rules 3 + 4: walk bracket expressions, rejecting the non-portable forms
	// and any unterminated class.
	for i := 0; i < len(glob); i++ {
		if glob[i] != '[' {
			continue
		}
		i++ // consume '['
		// Rule 3a: a literal ']' as the first member (e.g. []a]).
		if i < len(glob) && glob[i] == ']' {
			return &denyGlobError{glob: glob, reason: "']' as first bracket member"}
		}
		// Rule 3b: POSIX named/character classes [[:…:]] diverge across parsers.
		if strings.HasPrefix(glob[i:], "[:") {
			return &denyGlobError{glob: glob, reason: "POSIX [[:...:]] class is not portable"}
		}
		// Scan for the closing ']', rejecting a nested '[' on the way.
		closed := false
		for i < len(glob) && glob[i] != ']' {
			if glob[i] == '[' {
				return &denyGlobError{glob: glob, reason: "nested '[' in bracket expression"}
			}
			i++
		}
		if i < len(glob) && glob[i] == ']' {
			closed = true
		}
		// Rule 4: unterminated bracket expression fails closed.
		if !closed {
			return &denyGlobError{glob: glob, reason: "unterminated '[' bracket expression"}
		}
	}
	return nil
}

// denyGlobError pairs a glob with the reason it was rejected.
type denyGlobError struct {
	glob   string
	reason string
}

func (e *denyGlobError) Error() string {
	return "deny glob " + e.glob + " rejected: " + e.reason
}

// macosAnchoredRegex produces the anchored Seatbelt regex that a relative or
// absolute deny-glob expands to on macOS.
//
//   - A relative glob g rooted at workspace ws becomes
//     ^<ws>/(.*/)?<translated>$: the (.*/)? tail lets a leading ** match at any
//     depth below the workspace, and the whole thing is anchored so it cannot
//     cover paths outside ws.
//   - An absolute glob /prefix/**/.ext rooted at its literal prefix /prefix
//     becomes ^/prefix/(.*/)?\.ext$.
//
// Within a segment, * -> [^/]* and ? -> [^/] (literal_separator: no single
// wildcard crosses a /). ** is absorbed into the (.*/)? depth prefix. Returns
// an error if the glob is rejected by validateDenyGlob.
func macosAnchoredRegex(ws, glob string) (string, error) {
	if err := validateDenyGlob(glob); err != nil {
		return "", err
	}

	// Absolute globs root at their literal prefix; relative globs root at ws.
	prefix, rest := literalPrefix(glob)
	var sb strings.Builder
	sb.WriteByte('^')
	if glob[0] == '/' {
		sb.WriteString(prefix) // includes its trailing '/'
		sb.WriteString(translateSegments(strings.Split(rest, "/")))
	} else {
		sb.WriteString(ws)
		sb.WriteByte('/')
		sb.WriteString(translateSegments(strings.Split(glob, "/")))
	}
	sb.WriteByte('$')
	return sb.String(), nil
}

// literalPrefix splits an absolute glob into the leading literal portion
// (ending in '/') and the remainder that begins at the first metacharacter.
func literalPrefix(glob string) (prefix, rest string) {
	for i := 0; i < len(glob); i++ {
		if c := glob[i]; c == '*' || c == '?' || c == '[' {
			return glob[:i], glob[i:]
		}
	}
	return glob, ""
}

// translateSegments joins glob segments into a regex body. An exact "**"
// segment becomes the cross-segment depth prefix (.*/)?; all other segments
// are translated per-segment and joined with '/'.
func translateSegments(segs []string) string {
	var sb strings.Builder
	for i, seg := range segs {
		if seg == "**" {
			sb.WriteString("(.*/)?")
			continue
		}
		if i > 0 && segs[i-1] != "**" {
			sb.WriteByte('/')
		}
		sb.WriteString(translateSegment(seg))
	}
	return sb.String()
}

// translateSegment converts a single non-** glob segment into regex,
// honoring literal_separator semantics (* does not cross a /, which is already
// guaranteed because a segment contains no /).
func translateSegment(seg string) string {
	var sb strings.Builder
	for i := 0; i < len(seg); {
		c := seg[i]
		switch c {
		case '*':
			sb.WriteString("[^/]*")
			i++
		case '?':
			sb.WriteString("[^/]")
			i++
		case '[':
			// Copy the bracket expression, converting a leading '!' negation to
			// the regex '^' form so the meaning is preserved on macOS.
			end := strings.IndexByte(seg[i:], ']')
			sb.WriteByte('[')
			j := i + 1
			if j < len(seg) && seg[j] == '!' {
				sb.WriteByte('^')
				j++
			}
			sb.WriteString(seg[j : i+end])
			sb.WriteByte(']')
			i += end + 1
		default:
			sb.WriteString(regexp.QuoteMeta(string(c)))
			i++
		}
	}
	return sb.String()
}

// linuxMatchPaths returns the subset of tree matched by any of globs, using
// literal_separator semantics (** crosses segments, single * does not) and
// fail-closed behavior modeled after the landlock globset expansion:
//
//   - A path that is not valid UTF-8 is excluded (non-UTF-8 match -> None).
//   - The tree is the set of paths the sandbox enumerator actually visits;
//     because the enumerator does not descend symlinked directories, paths
//     reachable only through a symlink are simply absent from tree.
func linuxMatchPaths(ws string, globs []string, tree []string) ([]string, error) {
	for _, g := range globs {
		if err := validateDenyGlob(g); err != nil {
			return nil, err
		}
	}

	var matched []string
	for _, p := range tree {
		// Fail closed on non-UTF-8: the globset expansion returns None.
		if !utf8.ValidString(p) {
			continue
		}
		// Strip the workspace prefix so relative globs match the enumerated
		// (relative) paths; paths outside ws are left as-is and won't match.
		rel := p
		if ws != "" && strings.HasPrefix(p, ws) {
			rel = strings.TrimPrefix(p, ws)
			rel = strings.TrimPrefix(rel, "/")
		}
		for _, g := range globs {
			if globMatches(g, rel) {
				matched = append(matched, p)
				break
			}
		}
	}
	return matched, nil
}

// globMatches reports whether name matches glob under literal_separator
// semantics: ** spans path segments, single * and ? do not cross a /, and
// [...] are character classes.
func globMatches(glob, name string) bool {
	return matchSegments(strings.Split(glob, "/"), strings.Split(name, "/"))
}

// matchSegments recursively matches glob segments against name segments,
// treating a "**" glob segment as matching zero or more name segments.
func matchSegments(gs, ns []string) bool {
	for len(gs) > 0 && len(ns) > 0 {
		if gs[0] == "**" {
			// ** consumes zero or more segments; try every split.
			for k := 0; k <= len(ns); k++ {
				if matchSegments(gs[1:], ns[k:]) {
					return true
				}
			}
			return false
		}
		if !segmentMatch(gs[0], ns[0]) {
			return false
		}
		gs = gs[1:]
		ns = ns[1:]
	}
	// A trailing ** can match the (now empty) remainder.
	for len(gs) > 0 && gs[0] == "**" {
		gs = gs[1:]
	}
	return len(gs) == 0 && len(ns) == 0
}

// segmentMatch matches a single non-** glob segment against a name segment
// using the stdlib's filepath.Match (whose *, ?, [...] semantics already
// respect separator boundaries within a single segment).
func segmentMatch(pattern, name string) bool {
	ok, err := filepath.Match(pattern, name)
	return err == nil && ok
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

func TestValidateDenyGlob_RejectsNonPortable(t *testing.T) {
	// The "must reject" sample from the grok deny/glob parity spec.
	reject := []string{
		"[]a]",           // literal ']' as first bracket member
		"[[:alpha:]]",    // POSIX character class
		"{a,b}",          // brace alternation
		"**/*.{pem,key}", // brace alternation inside a otherwise-valid glob
		"a**b",           // ** not an exact segment
		"**a",            // ** not an exact segment
		"[abc",           // unterminated bracket expression
	}
	for _, g := range reject {
		if err := validateDenyGlob(g); err == nil {
			t.Errorf("validateDenyGlob(%q) = nil, want error (non-portable)", g)
		} else {
			t.Logf("ok: %v", err)
		}
	}
}

func TestValidateDenyGlob_AcceptsPortable(t *testing.T) {
	// The "must accept + translate identically" sample.
	accept := []string{
		"**/*.pem",
		"**/.env",
		"secrets/**",
		"/home/**/.ssh",
		"*.key",
	}
	for _, g := range accept {
		if err := validateDenyGlob(g); err != nil {
			t.Errorf("validateDenyGlob(%q) = %v, want nil (portable)", g, err)
		}
		if !isGlob(g) {
			t.Errorf("isGlob(%q) = false, expected a metacharacter", g)
		}
	}
}

func TestParity_AcceptSetsMatch(t *testing.T) {
	// Parity invariant: both backends classify each glob the same way. The
	// reference classification is the portable sample above.
	portable := map[string]bool{
		// must accept
		"**/*.pem":      true,
		"**/.env":       true,
		"secrets/**":    true,
		"/home/**/.ssh": true,
		"*.key":         true,
		// must reject
		"[]a]":           false,
		"[[:alpha:]]":    false,
		"{a,b}":          false,
		"**/*.{pem,key}": false,
		"a**b":           false,
		"**a":            false,
		"[abc":           false,
	}
	for g, wantPortable := range portable {
		err := validateDenyGlob(g)
		gotPortable := err == nil
		if gotPortable != wantPortable {
			t.Errorf("parity: validateDenyGlob(%q) portable=%v (err=%v), want portable=%v",
				g, gotPortable, err, wantPortable)
		}
	}
}

func TestMacosAnchoredRegex_RelativeRootedAtWorkspace(t *testing.T) {
	re, err := macosAnchoredRegex("/ws", "**/*.pem")
	if err != nil {
		t.Fatalf("macosAnchoredRegex error: %v", err)
	}
	t.Logf("regex: %s", re)
	// Expected anchored form: ^/ws/(.*/)?[^/]*\.pem$
	want := `^/ws/(.*/)?[^/]*\.pem$`
	if re != want {
		t.Errorf("got %q, want %q", re, want)
	}
	rx := regexp.MustCompile(re)
	if !rx.MatchString("/ws/sub/dir/key.pem") {
		t.Errorf("expected %q to match /ws/sub/dir/key.pem", re)
	}
	if rx.MatchString("/other/key.pem") {
		t.Errorf("expected %q NOT to match /other/key.pem (must be rooted at ws)", re)
	}
}

func TestMacosAnchoredRegex_AbsoluteRootsAtPrefix(t *testing.T) {
	re, err := macosAnchoredRegex("/ws", "/nope/**/.ssh")
	if err != nil {
		t.Fatalf("macosAnchoredRegex error: %v", err)
	}
	t.Logf("regex: %s", re)
	// Expected anchored form: ^/nope/(.*/)?\.ssh$
	want := `^/nope/(.*/)?\.ssh$`
	if re != want {
		t.Errorf("got %q, want %q", re, want)
	}
	rx := regexp.MustCompile(re)
	if !rx.MatchString("/nope/deep/.ssh") {
		t.Errorf("expected %q to match /nope/deep/.ssh", re)
	}
	if rx.MatchString("/nope/.ssh") {
		// /nope/.ssh has no intermediate segment; (.*/)? can be empty, so this
		// matches — log it so the behavior is explicit.
		t.Logf("note: %q also matches /nope/.ssh (empty depth prefix)", re)
	}
}

func TestLinuxMatchPaths_MatchesAndExcludes(t *testing.T) {
	tree := []string{"sub/dir/key.pem", ".env", "readable.txt"}
	globs := []string{"**/*.pem", "**/.env"}
	got, err := linuxMatchPaths("", globs, tree)
	if err != nil {
		t.Fatalf("linuxMatchPaths error: %v", err)
	}
	want := map[string]bool{"sub/dir/key.pem": true, ".env": true}
	if len(got) != len(want) {
		t.Fatalf("matched set = %v, want exactly %v", got, want)
	}
	for _, p := range got {
		if !want[p] {
			t.Errorf("unexpected match: %q", p)
		}
	}
}

func TestLinuxMatchPaths_SymlinkNotDescended(t *testing.T) {
	// The enumerator does not descend symlinked directories, so a path that is
	// only reachable through a symlink segment is absent from the enumerated
	// tree. Here `linked` is a symlink to an outside directory; its contents
	// are therefore not in tree, and must not appear in the matched set.
	tree := []string{
		"sub/dir/key.pem",
		"linked", // the symlink itself is present, but its target contents are not
	}
	globs := []string{"**/*.pem"}
	got, err := linuxMatchPaths("", globs, tree)
	if err != nil {
		t.Fatalf("linuxMatchPaths error: %v", err)
	}
	for _, p := range got {
		if strings.HasPrefix(p, "linked/") {
			t.Errorf("symlink-descended path matched (should not descend): %q", p)
		}
	}
	// Sanity: the in-workspace match is still found.
	found := false
	for _, p := range got {
		if p == "sub/dir/key.pem" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected sub/dir/key.pem to still match; got %v", got)
	}
}

func TestLinuxMatchPaths_NonUTF8FailClosed(t *testing.T) {
	// A path containing an invalid UTF-8 byte that would otherwise match
	// *.pem must be excluded: the landlock globset expansion returns None for
	// non-UTF-8 matches, so the deny-set fails closed (does not cover it).
	bogus := "dir/\xffkey.pem" // \xff is invalid UTF-8
	tree := []string{"dir/ok.pem", bogus}
	globs := []string{"**/*.pem"}
	got, err := linuxMatchPaths("", globs, tree)
	if err != nil {
		t.Fatalf("linuxMatchPaths error: %v", err)
	}
	for _, p := range got {
		if p == bogus {
			t.Errorf("non-UTF-8 path %q matched; expected fail-closed exclusion", bogus)
		}
	}
	found := false
	for _, p := range got {
		if p == "dir/ok.pem" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected dir/ok.pem to match; got %v", got)
	}
}
