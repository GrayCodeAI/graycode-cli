package swift

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// StableRule is the rendered form of a persisted exact permission rule.
type StableRule struct {
	ID       uint64
	Kind     string
	Identity string
	Decision string
	// Sensitive is set when the identity is a path/command that should be
	// masked before this rule is shared.
	Sensitive bool
}

// Activity is one recent tool/command event in the session's Recent Activity.
type Activity struct {
	Timestamp time.Time
	Kind      string // e.g. "tool", "command", "edit", "read"
	Name      string
	OK        bool
	Duration  time.Duration
}

// LogEntry is one line of a log/transcript tail.
type LogEntry struct {
	Line      string
	Sensitive bool
}

const (
	// MaxLogTailLines caps the number of log lines rendered in the tail.
	MaxLogTailLines = 80
	// MaxLogTailBytes is the number of trailing bytes read from a log file.
	MaxLogTailBytes = 6 * 1024
	// MaxLineBytes caps any single rendered line.
	MaxLineBytes = 300
)

// Snapshot captures the hawk diagnostic context rendered by Build. It is
// deliberately engine-agnostic: callers (engine integration or the CLI) fill
// it from their live state, and Build renders it purely.
type Snapshot struct {
	Timestamp      time.Time
	Version        string
	GitCommit      string
	Build          string
	Platform       string // os/arch
	Model          string
	FastMode       bool
	PermissionMode string
	Sandbox        string
	Workspace      string

	SessionID  string
	SessionDir string
	PID        int
	Terminal   string // "colsxrows"
	Env        []string

	StableRules      []StableRule
	PermissionGrants []string

	Activity   []Activity
	LogTail    []LogEntry
	Transcript []LogEntry
}

// Build renders the snapshot as the hawk swift diagnostic markdown, mirroring
// fx's `/swift` report structure. Every potentially secret field is masked.
func Build(s *Snapshot) string {
	if s == nil {
		s = &Snapshot{}
	}
	var b strings.Builder
	b.Reset()

	b.WriteString("# hawk swift\n\n")
	b.WriteString("Private diagnostic report. It may include prompts, file paths, command output, and file snippets.\n")

	writeSummary(&b, s)
	writeCurrentState(&b, s)
	writePermissions(&b, s)
	writeRecentActivity(&b, s)
	writeLogTail(&b, s)
	writeTranscript(&b, s)

	return b.String()
}

func writeSummary(b *strings.Builder, s *Snapshot) {
	b.WriteString("\n## Summary\n")
	ts := s.Timestamp
	if ts.IsZero() {
		ts = time.Now()
	}
	b.WriteString("generated: " + ts.UTC().Format("2006-01-02T15:04:05Z") + "\n")
	ver := s.Version
	if ver == "" {
		ver = "dev"
	}
	b.WriteString("version: " + ver)
	if s.GitCommit != "" {
		b.WriteString(" (" + s.GitCommit + ")")
	}
	b.WriteString("\n")
	b.WriteString("platform: " + s.Platform + "\n")
	if s.Build != "" {
		b.WriteString("build: " + s.Build + "\n")
	}
	if s.Model != "" {
		b.WriteString("model: " + s.Model + "\n")
	}
	if s.FastMode {
		b.WriteString("fast_mode: on\n")
	}
	pm := s.PermissionMode
	if pm == "" {
		pm = "default"
	}
	b.WriteString("permission_mode: " + pm + "\n")
	sandbox := s.Sandbox
	if sandbox == "" {
		sandbox = "default"
	}
	b.WriteString("sandbox: " + sandbox + "\n")
	b.WriteString("workspace: " + s.Workspace + "\n")
}

func writeCurrentState(b *strings.Builder, s *Snapshot) {
	b.WriteString("\n## Current State\n")
	if s.SessionID != "" {
		b.WriteString("session_id: " + s.SessionID + "\n")
	}
	if s.SessionDir != "" {
		b.WriteString("session_dir: " + s.SessionDir + "\n")
	}
	b.WriteString("process: pid=" + fmt.Sprint(s.PID) + "\n")
	if s.Terminal != "" {
		b.WriteString("terminal: " + s.Terminal + "\n")
	}
	if len(s.Env) > 0 {
		b.WriteString("env:\n")
		for _, kv := range s.Env {
			b.WriteString("  " + maskSecrets(kv) + "\n")
		}
	}
}

func writePermissions(b *strings.Builder, s *Snapshot) {
	b.WriteString("\n## Permissions\n")
	if len(s.StableRules) == 0 && len(s.PermissionGrants) == 0 {
		b.WriteString("(none)\n")
		return
	}
	if len(s.StableRules) > 0 {
		rules := append([]StableRule(nil), s.StableRules...)
		sort.Slice(rules, func(i, j int) bool { return rules[i].ID < rules[j].ID })
		b.WriteString(fmt.Sprintf("stable_rules (%d):\n", len(rules)))
		for _, r := range rules {
			id := r.Identity
			if r.Sensitive {
				id = maskSecrets(id)
			}
			b.WriteString(fmt.Sprintf("  - id=%d kind=%s decision=%s identity=%s\n",
				r.ID, r.Kind, r.Decision, id))
		}
	}
	for _, g := range s.PermissionGrants {
		b.WriteString("  grant: " + maskSecrets(g) + "\n")
	}
}

func writeRecentActivity(b *strings.Builder, s *Snapshot) {
	b.WriteString("\n## Recent Activity\n")
	if len(s.Activity) == 0 {
		b.WriteString("(none)\n")
		return
	}
	for _, a := range s.Activity {
		ts := ""
		if !a.Timestamp.IsZero() {
			ts = a.Timestamp.UTC().Format("15:04:05")
		}
		ok := "ok"
		if !a.OK {
			ok = "FAILED"
		}
		dur := ""
		if a.Duration > 0 {
			dur = " " + a.Duration.Round(time.Millisecond).String()
		}
		b.WriteString(fmt.Sprintf("  %s %s %s %s%s\n", ts, a.Kind, maskSecrets(a.Name), ok, dur))
	}
}

func writeLogTail(b *strings.Builder, s *Snapshot) {
	if len(s.LogTail) == 0 {
		return
	}
	b.WriteString("\n## Logs\nonly obvious secrets masked\n")
	if len(s.LogTail) > 0 {
		b.WriteString(fmt.Sprintf("recent_lines (%d):\n", len(s.LogTail)))
	}
	for _, e := range s.LogTail {
		line := e.Line
		if e.Sensitive {
			line = maskSecrets(line)
		}
		if len(line) > MaxLineBytes {
			line = line[:MaxLineBytes] + " ..."
		}
		b.WriteString("  " + line + "\n")
	}
}

func writeTranscript(b *strings.Builder, s *Snapshot) {
	if len(s.Transcript) == 0 {
		return
	}
	b.WriteString("\n## Transcript\n")
	for _, e := range s.Transcript {
		line := e.Line
		if e.Sensitive {
			line = maskSecrets(line)
		}
		line = stripANSI(line)
		if len(line) > MaxLineBytes {
			line = line[:MaxLineBytes] + " ..."
		}
		b.WriteString("  " + line + "\n")
	}
}
