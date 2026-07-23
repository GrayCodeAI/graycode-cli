// Package audit provides waste-detection detectors for agent transcripts.
//
// Ported from failproofai (https://github.com/FailproofAI/failproofai)
// with permission. These detectors catch "stupid behaviors" that waste tokens
// and wall-clock time but aren't dangerous enough to block at runtime.
package audit

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	contracts "github.com/GrayCodeAI/hawk-core-contracts/events"
)

// DetectorHit represents one detected occurrence of wasteful behavior.
type DetectorHit struct {
	DetectorName string
	Example      string
	Severity     string // "info" or "warn"
	Category     string // "Wasteful" or "Risky"
}

// ToolEvent represents a normalized tool event from a transcript.
type ToolEvent = contracts.ToolEvent

// DetectorSessionState holds per-session state for stateful detectors.
type DetectorSessionState = map[string]interface{}

// Detector is a pure function that detects wasteful behavior in tool events.
type Detector struct {
	Name        string
	Description string
	Category    string
	Severity    string
	Detect      func(event ToolEvent, state DetectorSessionState) *DetectorHit
}

// AllDetectors returns all registered audit detectors.
func AllDetectors() []Detector {
	return []Detector{
		RedundantCdCwd,
		PreferEditOverReadCat,
		PreferEditOverSedAwk,
		PreferWriteOverHeredoc,
		SleepPollingLoop,
		FindFromRoot,
		GitCommitNoVerify,
		RereadAfterEdit,
	}
}

// --- Detector Implementations ---

var cdCwdRe = regexp.MustCompile(`^cd\s+(?:"([^"]+)"|'([^']+)'|(\S+))\s*&&\s*([\s\S]+)$`)

// RedundantCdCwd detects `cd <cwd> && command` where the cd is unnecessary.
var RedundantCdCwd = Detector{
	Name:        "redundant-cd-cwd",
	Description: "Bash commands prefixed with `cd <cwd> && ...` even though commands already run in cwd.",
	Category:    "Wasteful",
	Severity:    "info",
	Detect: func(event ToolEvent, _ DetectorSessionState) *DetectorHit {
		if event.ToolName != "Bash" {
			return nil
		}
		command, _ := event.ToolInput["command"].(string)
		if command == "" || event.CWD == "" {
			return nil
		}
		trimmed := strings.TrimLeft(command, " \t")
		match := cdCwdRe.FindStringSubmatch(trimmed)
		if match == nil {
			return nil
		}
		path := strings.TrimRight(match[1]+match[2]+match[3], "/")
		cwd := strings.TrimRight(event.CWD, "/")
		if path != cwd {
			return nil
		}
		rest := strings.TrimSpace(match[4])
		return &DetectorHit{
			DetectorName: "redundant-cd-cwd",
			Example:      fmt.Sprintf("cd %s && %s", path, rest),
			Severity:     "info",
			Category:     "Wasteful",
		}
	},
}

var sourceExtRe = regexp.MustCompile(`\.(?:ts|tsx|js|jsx|mjs|cjs|py|go|rs|java|kt|swift|rb|php|c|h|cc|cpp|hpp|cs|scala|sh|bash|zsh|json|yaml|yml|toml|md|txt|sql|html|css|scss|sass)$`)

// PreferEditOverReadCat detects `cat/head/tail/less/more` on source files.
var PreferEditOverReadCat = Detector{
	Name:        "prefer-edit-over-read-cat",
	Description: "Bash `cat`/`head`/`tail`/`less`/`more` on a single source file.",
	Category:    "Wasteful",
	Severity:    "info",
	Detect: func(event ToolEvent, _ DetectorSessionState) *DetectorHit {
		if event.ToolName != "Bash" {
			return nil
		}
		command, _ := event.ToolInput["command"].(string)
		if command == "" {
			return nil
		}
		cmd := strings.TrimSpace(command)
		// Reject shell pipelines, redirections, etc.
		if strings.ContainsAny(cmd, "|<>;&`$()") {
			return nil
		}
		// Match: cat/head/tail/less/more [flags] <path>
		parts := strings.Fields(cmd)
		if len(parts) < 2 {
			return nil
		}
		bin := parts[0]
		if bin != "cat" && bin != "head" && bin != "tail" && bin != "less" && bin != "more" {
			return nil
		}
		// Get the last non-flag argument as the path
		path := ""
		for _, p := range parts[1:] {
			if !strings.HasPrefix(p, "-") {
				path = strings.Trim(p, "\"'")
			}
		}
		if path == "" {
			return nil
		}
		// Skip .env files (covered by block-env-files)
		if strings.HasSuffix(path, ".env") || strings.Contains(path, ".env.") {
			return nil
		}
		if !sourceExtRe.MatchString(path) {
			return nil
		}
		return &DetectorHit{
			DetectorName: "prefer-edit-over-read-cat",
			Example:      cmd,
			Severity:     "info",
			Category:     "Wasteful",
		}
	},
}

var sedAwkRe = regexp.MustCompile(`\b(sed|awk)\s`)

// PreferEditOverSedAwk detects sed/awk on source files instead of Edit tool.
var PreferEditOverSedAwk = Detector{
	Name:        "prefer-edit-over-sed-awk",
	Description: "Bash `sed`/`awk` on source files instead of using Edit tool.",
	Category:    "Wasteful",
	Severity:    "info",
	Detect: func(event ToolEvent, _ DetectorSessionState) *DetectorHit {
		if event.ToolName != "Bash" {
			return nil
		}
		command, _ := event.ToolInput["command"].(string)
		if command == "" {
			return nil
		}
		if !sedAwkRe.MatchString(command) {
			return nil
		}
		// Check if it's operating on a source file
		if sourceExtRe.MatchString(command) {
			return &DetectorHit{
				DetectorName: "prefer-edit-over-sed-awk",
				Example:      truncate(command, 80),
				Severity:     "info",
				Category:     "Wasteful",
			}
		}
		return nil
	},
}

var heredocRe = regexp.MustCompile(`cat\s*<<\s*['"]?\w+['"]?\s*>`)

// PreferWriteOverHeredoc detects heredoc usage instead of Write tool.
var PreferWriteOverHeredoc = Detector{
	Name:        "prefer-write-over-heredoc",
	Description: "Bash heredoc (`cat << EOF > file`) instead of Write tool.",
	Category:    "Wasteful",
	Severity:    "info",
	Detect: func(event ToolEvent, _ DetectorSessionState) *DetectorHit {
		if event.ToolName != "Bash" {
			return nil
		}
		command, _ := event.ToolInput["command"].(string)
		if command == "" {
			return nil
		}
		if heredocRe.MatchString(command) {
			return &DetectorHit{
				DetectorName: "prefer-write-over-heredoc",
				Example:      truncate(command, 80),
				Severity:     "info",
				Category:     "Wasteful",
			}
		}
		return nil
	},
}

var (
	sleepRe      = regexp.MustCompile(`\bsleep\s+(\d+(?:\.\d+)?)(m|h|d)?\b`)
	whileSleepRe = regexp.MustCompile(`\bwhile\b[\s\S]*?\bsleep\b[\s\S]*?\bdone\b`)
)

const sleepThresholdSeconds = 30

// SleepPollingLoop detects long sleep or while-sleep polling loops.
var SleepPollingLoop = Detector{
	Name:        "sleep-polling-loop",
	Description: "Bash long `sleep` or while-sleep polling loops.",
	Category:    "Wasteful",
	Severity:    "info",
	Detect: func(event ToolEvent, _ DetectorSessionState) *DetectorHit {
		if event.ToolName != "Bash" {
			return nil
		}
		command, _ := event.ToolInput["command"].(string)
		if command == "" {
			return nil
		}
		// while-sleep loop
		if whileSleepRe.MatchString(command) {
			return &DetectorHit{
				DetectorName: "sleep-polling-loop",
				Example:      truncate(command, 80),
				Severity:     "info",
				Category:     "Wasteful",
			}
		}
		// Standalone long sleep
		match := sleepRe.FindStringSubmatch(command)
		if match != nil {
			n, _ := strconv.ParseFloat(match[1], 64)
			unit := "s"
			if match[2] != "" {
				unit = match[2]
			}
			var seconds float64
			switch unit {
			case "m":
				seconds = n * 60
			case "h":
				seconds = n * 3600
			case "d":
				seconds = n * 86400
			default:
				seconds = n
			}
			if seconds >= sleepThresholdSeconds {
				return &DetectorHit{
					DetectorName: "sleep-polling-loop",
					Example:      truncate(command, 80),
					Severity:     "info",
					Category:     "Wasteful",
				}
			}
		}
		return nil
	},
}

var (
	findRe     = regexp.MustCompile(`(?:^|[\s;|&])find\s+(?:-\S+\s+)*("[^"]+"|'[^']+'|\S+)`)
	riskyRoots = map[string]bool{
		"/": true, "/home": true, "/usr": true, "/etc": true,
		"/var": true, "/opt": true, "/Users": true,
	}
)

// FindFromRoot detects `find` against filesystem root.
var FindFromRoot = Detector{
	Name:        "find-from-root",
	Description: "Bash `find` against /, /home, /usr, etc.",
	Category:    "Risky",
	Severity:    "warn",
	Detect: func(event ToolEvent, _ DetectorSessionState) *DetectorHit {
		if event.ToolName != "Bash" {
			return nil
		}
		command, _ := event.ToolInput["command"].(string)
		if command == "" {
			return nil
		}
		match := findRe.FindStringSubmatch(command)
		if match == nil {
			return nil
		}
		raw := strings.Trim(match[1], "\"'")
		stripped := strings.TrimRight(raw, "/")
		if stripped == "" {
			stripped = "/"
		}
		if !riskyRoots[stripped] {
			return nil
		}
		return &DetectorHit{
			DetectorName: "find-from-root",
			Example:      truncate(command, 80),
			Severity:     "warn",
			Category:     "Risky",
		}
	},
}

var (
	gitCommitRe = regexp.MustCompile(`\bgit\s+commit\b`)
	noVerifyRe  = regexp.MustCompile(`\s--no-verify\b|\s-n\b`)
)

// GitCommitNoVerify detects `git commit --no-verify`.
var GitCommitNoVerify = Detector{
	Name:        "git-commit-no-verify",
	Description: "git commit invoked with --no-verify / -n, skipping hooks.",
	Category:    "Risky",
	Severity:    "warn",
	Detect: func(event ToolEvent, _ DetectorSessionState) *DetectorHit {
		if event.ToolName != "Bash" {
			return nil
		}
		command, _ := event.ToolInput["command"].(string)
		if command == "" {
			return nil
		}
		if !gitCommitRe.MatchString(command) {
			return nil
		}
		if noVerifyRe.MatchString(command) {
			return &DetectorHit{
				DetectorName: "git-commit-no-verify",
				Example:      truncate(command, 80),
				Severity:     "warn",
				Category:     "Risky",
			}
		}
		return nil
	},
}

// RereadAfterEdit detects reading a file that was just edited.
type rereadState struct {
	Countdown map[string]int
}

func getRereadState(state DetectorSessionState) *rereadState {
	s, ok := state["rereadAfterEdit"]
	if !ok {
		st := &rereadState{Countdown: make(map[string]int)}
		state["rereadAfterEdit"] = st
		return st
	}
	rs, _ := s.(*rereadState)
	if rs == nil {
		rs = &rereadState{Countdown: make(map[string]int)}
		state["rereadAfterEdit"] = rs
	}
	return rs
}

const rereadWindow = 5

// RereadAfterEdit detects reading a file that was just edited.
var RereadAfterEdit = Detector{
	Name:        "reread-after-edit",
	Description: "Read of a file that was just Edit'd or Write'n in the same session.",
	Category:    "Wasteful",
	Severity:    "info",
	Detect: func(event ToolEvent, state DetectorSessionState) *DetectorHit {
		rs := getRereadState(state)

		// Tick down every existing countdown
		for key, n := range rs.Countdown {
			if n <= 1 {
				delete(rs.Countdown, key)
			} else {
				rs.Countdown[key] = n - 1
			}
		}

		filePath, _ := event.ToolInput["file_path"].(string)
		if filePath == "" {
			return nil
		}

		if event.ToolName == "Edit" || event.ToolName == "Write" {
			rs.Countdown[filePath] = rereadWindow
			return nil
		}

		if event.ToolName == "Read" {
			if _, ok := rs.Countdown[filePath]; ok {
				delete(rs.Countdown, filePath)
				return &DetectorHit{
					DetectorName: "reread-after-edit",
					Example:      fmt.Sprintf("Read %s immediately after Edit/Write", filePath),
					Severity:     "info",
					Category:     "Wasteful",
				}
			}
		}

		return nil
	},
}

// truncate truncates a string to maxLen characters.
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	// Rune-safe truncation: never split a multibyte UTF-8 sequence.
	if runes := []rune(s); len(runes) > maxLen {
		return string(runes[:maxLen-3]) + "..."
	}
	return s
}
