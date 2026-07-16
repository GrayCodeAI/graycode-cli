package tool

import (
	"fmt"
	"strings"
	"unicode"
)

// exploreBashAllow is the first-token allowlist for explore/plan (read-only) modes.
// Anything not listed is denied — fail-closed.
var exploreBashAllow = map[string]bool{
	// listing / inspection
	"ls": true, "pwd": true, "dirname": true, "basename": true,
	"cat": true, "head": true, "tail": true, "less": true, "more": true,
	"wc": true, "file": true, "stat": true, "du": true, "df": true,
	"which": true, "type": true, "command": true, "env": true, "printenv": true,
	"echo": true, "printf": true, "true": true, "false": true, "test": true, "[": true,
	// search
	"grep": true, "egrep": true, "fgrep": true, "rg": true, "ag": true, "ack": true,
	"find": true, "locate": true,
	// text utilities (read-side)
	"sort": true, "uniq": true, "cut": true, "tr": true, "awk": true, "sed": true,
	"jq": true, "yq": true, "column": true, "comm": true,
	"diff": true, "cmp": true,
	// VCS (subcommand gated below)
	"git": true, "gh": true,
	// language tooling (read-ish)
	"go": true, "node": true, "python": true, "python3": true, "ruby": true,
	"cargo": true, "rustc": true, "javac": true, "java": true,
	"npm": true, "pnpm": true, "yarn": true, "pip": true, "pip3": true,
	// system info
	"uname": true, "whoami": true, "id": true, "date": true, "hostname": true,
	"sysctl": true, "sw_vers": true,
	// network read
	"curl": true, "wget": true, "dig": true, "nslookup": true, "ping": true,
	// tree / pretty
	"tree": true, "bat": true, "hexdump": true, "od": true, "xxd": true,
}

// git subcommands allowed under explore/plan (no mutation).
var exploreGitAllow = map[string]bool{
	"status": true, "log": true, "show": true, "diff": true, "branch": true,
	"rev-parse": true, "rev-list": true, "describe": true, "ls-files": true,
	"ls-tree": true, "cat-file": true, "blame": true, "shortlog": true,
	"remote": true, "tag": true, "stash": true, // stash list only enforced loosely
	"config": true, "help": true, "version": true, "grep": true,
	"name-rev": true, "symbolic-ref": true, "for-each-ref": true,
}

// go subcommands that are non-mutating (or only write caches).
var exploreGoAllow = map[string]bool{
	"version": true, "env": true, "list": true, "doc": true, "help": true,
	"test": true, "vet": true, "fmt": true, // fmt rewrites — deny separately
	"mod": true, "tool": true,
}

// ExploreBashAllowed returns nil if command is safe for explore/plan modes.
// It is segment-aware: splits on ; && || | and checks each pipeline stage's
// primary command. Fail-closed on empty/unknown tokens, redirects, or
// mutating git/go subcommands.
func ExploreBashAllowed(command string) error {
	cmd := strings.TrimSpace(command)
	if cmd == "" {
		return fmt.Errorf("empty command")
	}
	// Hard deny obvious mutators / redirections even before segment parse.
	if strings.Contains(cmd, ">") || strings.Contains(cmd, "<<") {
		// Allow `2>&1` style redirects only if no file redirect.
		if hasFileRedirect(cmd) {
			return fmt.Errorf("explore bash: file redirects are not allowed")
		}
	}
	segments := splitShellSegments(cmd)
	if len(segments) == 0 {
		return fmt.Errorf("explore bash: no executable segments")
	}
	for _, seg := range segments {
		if err := exploreSegmentAllowed(seg); err != nil {
			return err
		}
	}
	return nil
}

func hasFileRedirect(s string) bool {
	// Detect >file or >>file or > file but not 2>&1
	runes := []rune(s)
	for i := 0; i < len(runes); i++ {
		if runes[i] != '>' {
			continue
		}
		// skip >>
		j := i + 1
		if j < len(runes) && runes[j] == '>' {
			j++
		}
		// skip optional & for 2>&1 — if next is & digit, ok
		for j < len(runes) && unicode.IsSpace(runes[j]) {
			j++
		}
		if j < len(runes) && runes[j] == '&' {
			continue // fd redirect
		}
		if j < len(runes) {
			return true
		}
	}
	return false
}

// splitShellSegments splits on top-level ; && || | (not inside quotes).
func splitShellSegments(s string) []string {
	var segs []string
	var b strings.Builder
	inSingle, inDouble := false, false
	escape := false
	for i := 0; i < len(s); i++ {
		ch := s[i]
		if escape {
			b.WriteByte(ch)
			escape = false
			continue
		}
		if ch == '\\' && !inSingle {
			escape = true
			b.WriteByte(ch)
			continue
		}
		if ch == '\'' && !inDouble {
			inSingle = !inSingle
			b.WriteByte(ch)
			continue
		}
		if ch == '"' && !inSingle {
			inDouble = !inDouble
			b.WriteByte(ch)
			continue
		}
		if !inSingle && !inDouble {
			// operators
			if ch == ';' {
				if t := strings.TrimSpace(b.String()); t != "" {
					segs = append(segs, t)
				}
				b.Reset()
				continue
			}
			if ch == '|' || ch == '&' {
				// || or && or |
				op := string(ch)
				if i+1 < len(s) && s[i+1] == ch {
					op = string(ch) + string(ch)
					i++
				}
				if t := strings.TrimSpace(b.String()); t != "" {
					segs = append(segs, t)
				}
				b.Reset()
				_ = op
				continue
			}
		}
		b.WriteByte(ch)
	}
	if t := strings.TrimSpace(b.String()); t != "" {
		segs = append(segs, t)
	}
	return segs
}

func exploreSegmentAllowed(seg string) error {
	seg = strings.TrimSpace(seg)
	if seg == "" {
		return nil
	}
	// strip env assignments: FOO=bar cmd
	tokens := tokenizeShellWords(seg)
	for len(tokens) > 0 && strings.Contains(tokens[0], "=") && !strings.HasPrefix(tokens[0], "=") {
		tokens = tokens[1:]
	}
	if len(tokens) == 0 {
		return fmt.Errorf("explore bash: empty segment after env strip")
	}
	// skip common prefixes
	for len(tokens) > 0 {
		switch tokens[0] {
		case "command", "builtin", "time", "env", "nice", "nohup":
			tokens = tokens[1:]
			continue
		case "sudo", "doas":
			return fmt.Errorf("explore bash: privilege escalation (%s) is not allowed", tokens[0])
		}
		break
	}
	if len(tokens) == 0 {
		return fmt.Errorf("explore bash: no command token")
	}
	base := commandBase(tokens[0])
	if !exploreBashAllow[base] {
		return fmt.Errorf("explore bash: command %q is not on the read-only allowlist", base)
	}
	switch base {
	case "git":
		return exploreGitAllowed(tokens[1:])
	case "go":
		return exploreGoAllowed(tokens[1:])
	case "npm", "pnpm", "yarn":
		return explorePackageManagerAllowed(base, tokens[1:])
	case "sed":
		// sed -i is mutating
		for _, t := range tokens[1:] {
			if t == "-i" || strings.HasPrefix(t, "-i") {
				return fmt.Errorf("explore bash: sed -i is not allowed")
			}
		}
	case "find":
		for _, t := range tokens[1:] {
			if t == "-delete" || t == "-exec" || t == "-execdir" || t == "-ok" {
				return fmt.Errorf("explore bash: find %s is not allowed", t)
			}
		}
	}
	return nil
}

func exploreGitAllowed(args []string) error {
	if len(args) == 0 {
		return nil // git with no args is help-ish
	}
	sub := args[0]
	if strings.HasPrefix(sub, "-") {
		// git --version etc.
		return nil
	}
	// deny mutators even if they appear as "git -C path commit"
	deny := map[string]bool{
		"add": true, "commit": true, "push": true, "pull": true, "fetch": true,
		"merge": true, "rebase": true, "cherry-pick": true, "reset": true,
		"checkout": true, "switch": true, "restore": true, "clean": true,
		"rm": true, "mv": true, "init": true, "clone": true, "worktree": true,
		"am": true, "apply": true, "revert": true, "tag": false, // tag create vs list
	}
	if deny[sub] {
		return fmt.Errorf("explore bash: git %s is not allowed", sub)
	}
	// stash push/pop/drop
	if sub == "stash" && len(args) > 1 {
		switch args[1] {
		case "list", "show":
			return nil
		default:
			return fmt.Errorf("explore bash: git stash %s is not allowed", args[1])
		}
	}
	// tag without args lists; with -a/-d mutates
	if sub == "tag" {
		for _, a := range args[1:] {
			if a == "-a" || a == "-d" || a == "-m" || a == "-f" {
				return fmt.Errorf("explore bash: mutating git tag flags not allowed")
			}
		}
	}
	if !exploreGitAllow[sub] {
		return fmt.Errorf("explore bash: git %s is not on the read-only allowlist", sub)
	}
	return nil
}

func exploreGoAllowed(args []string) error {
	if len(args) == 0 {
		return nil
	}
	sub := args[0]
	deny := map[string]bool{
		"get": true, "install": true, "generate": true, "work": true,
		"clean": true, "mod": false,
	}
	if sub == "fmt" {
		return fmt.Errorf("explore bash: go fmt rewrites files and is not allowed")
	}
	if sub == "mod" {
		if len(args) > 1 {
			switch args[1] {
			case "download", "tidy", "vendor", "init", "edit":
				return fmt.Errorf("explore bash: go mod %s is not allowed", args[1])
			}
		}
		return nil
	}
	if deny[sub] {
		return fmt.Errorf("explore bash: go %s is not allowed", sub)
	}
	if !exploreGoAllow[sub] {
		return fmt.Errorf("explore bash: go %s is not on the read-only allowlist", sub)
	}
	return nil
}

func explorePackageManagerAllowed(pm string, args []string) error {
	if len(args) == 0 {
		return nil
	}
	sub := args[0]
	allow := map[string]bool{
		"list": true, "ls": true, "view": true, "info": true, "outdated": true,
		"why": true, "explain": true, "pack": false, "test": true, "run": false,
		"help": true, "version": true, "bin": true, "root": true, "prefix": true,
	}
	deny := map[string]bool{
		"install": true, "i": true, "add": true, "uninstall": true, "remove": true,
		"update": true, "upgrade": true, "publish": true, "link": true, "ci": true,
	}
	if deny[sub] {
		return fmt.Errorf("explore bash: %s %s is not allowed", pm, sub)
	}
	if !allow[sub] {
		return fmt.Errorf("explore bash: %s %s is not on the read-only allowlist", pm, sub)
	}
	return nil
}

func commandBase(token string) string {
	token = strings.Trim(token, `"'`)
	// strip path
	if i := strings.LastIndex(token, "/"); i >= 0 {
		token = token[i+1:]
	}
	return strings.ToLower(token)
}

// tokenizeShellWords is a minimal whitespace tokenizer that respects quotes.
func tokenizeShellWords(s string) []string {
	var out []string
	var b strings.Builder
	inSingle, inDouble := false, false
	escape := false
	flush := func() {
		if b.Len() > 0 {
			out = append(out, b.String())
			b.Reset()
		}
	}
	for i := 0; i < len(s); i++ {
		ch := s[i]
		if escape {
			b.WriteByte(ch)
			escape = false
			continue
		}
		if ch == '\\' && !inSingle {
			escape = true
			continue
		}
		if ch == '\'' && !inDouble {
			inSingle = !inSingle
			continue
		}
		if ch == '"' && !inSingle {
			inDouble = !inDouble
			continue
		}
		if !inSingle && !inDouble && (ch == ' ' || ch == '\t' || ch == '\n') {
			flush()
			continue
		}
		b.WriteByte(ch)
	}
	flush()
	return out
}
