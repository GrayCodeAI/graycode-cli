package cmd

import (
	"strings"
	"unicode"

	tea "github.com/charmbracelet/bubbletea"
)

// VimMode represents the current vim mode.
type VimMode int

const (
	VimInsert VimMode = iota
	VimNormal
)

// Operator represents a vim operator (d, c, y).
type Operator int

const (
	OpNone Operator = iota
	OpDelete
	OpChange
	OpYank
)

// CommandType represents what the command state is waiting for.
type CommandType int

const (
	CmdIdle CommandType = iota
	CmdCount
	CmdOperator
	CmdFind
	CmdOperatorFind
	CmdOperatorTextObj
	CmdReplace
)

// CommandState tracks the in-progress vim command.
type CommandState struct {
	Type     CommandType
	Op       Operator
	Count    int
	Digits   string
	FindType byte // f, F, t, T
	Scope    byte // i (inner), a (around)
}

// RecordedChange stores info needed for dot-repeat.
type RecordedChange struct {
	Type     string // "insert", "operator", "replace", "x", "toggleCase"
	Keys     []tea.KeyMsg
	Text     string
	StartPos int
	EndPos   int
}

// PersistentState survives across commands.
type PersistentState struct {
	LastFind     byte
	LastFindType byte
	Register     string
	RegisterLine bool
	LastChange   *RecordedChange
	Recording    *RecordedChange
}

// VimState is the full vim state machine.
type VimState struct {
	Mode       VimMode
	Command    CommandState
	Persistent PersistentState
	enabled    bool
}

// NewVimState creates a new vim state starting in insert mode.
func NewVimState() *VimState {
	return &VimState{
		Mode:    VimInsert,
		enabled: true,
	}
}

// IsEnabled returns whether vim mode is active.
func (v *VimState) IsEnabled() bool { return v.enabled }

// SetEnabled enables or disables vim mode.
func (v *VimState) SetEnabled(enabled bool) { v.enabled = enabled }

// ModeString returns a display string for the current mode.
func (v *VimState) ModeString() string {
	if !v.enabled {
		return ""
	}
	switch v.Mode {
	case VimNormal:
		return "NORMAL"
	case VimInsert:
		return "INSERT"
	}
	return ""
}

// HandleKey processes a key event and returns the new text, cursor position,
// and whether the key was consumed by vim.
func (v *VimState) HandleKey(msg tea.KeyMsg, text string, cursor int) (string, int, bool) {
	if !v.enabled {
		return text, cursor, false
	}

	switch v.Mode {
	case VimInsert:
		return v.handleInsertMode(msg, text, cursor)
	case VimNormal:
		return v.handleNormalMode(msg, text, cursor)
	}
	return text, cursor, false
}

func (v *VimState) handleInsertMode(msg tea.KeyMsg, text string, cursor int) (string, int, bool) {
	if msg.Type == tea.KeyEscape {
		v.Mode = VimNormal
		if cursor > 0 {
			cursor--
		}
		return text, cursor, true
	}
	return text, cursor, false
}

func (v *VimState) handleNormalMode(msg tea.KeyMsg, text string, cursor int) (string, int, bool) {
	if v.Command.Type == CmdFind || v.Command.Type == CmdOperatorFind {
		return v.handleFindChar(msg, text, cursor)
	}
	if v.Command.Type == CmdReplace {
		return v.handleReplace(msg, text, cursor)
	}

	key := msg.String()

	// Count accumulation
	if len(key) == 1 && key[0] >= '1' && key[0] <= '9' && v.Command.Type == CmdIdle {
		v.Command.Type = CmdCount
		v.Command.Digits += key
		v.Command.Count = atoi(v.Command.Digits)
		return text, cursor, true
	}
	if len(key) == 1 && key[0] >= '0' && key[0] <= '9' && v.Command.Type == CmdCount {
		v.Command.Digits += key
		v.Command.Count = atoi(v.Command.Digits)
		return text, cursor, true
	}

	count := v.Command.Count
	if count == 0 {
		count = 1
	}

	switch key {
	// Mode switches
	case "i":
		if v.Command.Type == CmdOperator {
			v.Command.Type = CmdOperatorTextObj
			v.Command.Scope = 'i'
			return text, cursor, true
		}
		v.Mode = VimInsert
		v.resetCommand()
		return text, cursor, true
	case "I":
		v.Mode = VimInsert
		cursor = firstNonBlank(text)
		v.resetCommand()
		return text, cursor, true
	case "a":
		if v.Command.Type == CmdOperator {
			v.Command.Type = CmdOperatorTextObj
			v.Command.Scope = 'a'
			return text, cursor, true
		}
		v.Mode = VimInsert
		if cursor < len(text) {
			cursor++
		}
		v.resetCommand()
		return text, cursor, true
	case "A":
		v.Mode = VimInsert
		cursor = len(text)
		v.resetCommand()
		return text, cursor, true

	// Motions
	case "h", "left":
		cursor = maxInt(0, cursor-count)
		v.resetCommand()
		return text, cursor, true
	case "l", "right":
		cursor = minInt(len(text)-1, cursor+count)
		if cursor < 0 {
			cursor = 0
		}
		v.resetCommand()
		return text, cursor, true
	case "0":
		if v.Command.Type == CmdCount {
			v.Command.Digits += "0"
			v.Command.Count = atoi(v.Command.Digits)
			return text, cursor, true
		}
		cursor = 0
		v.resetCommand()
		return text, cursor, true
	case "$":
		cursor = maxInt(0, len(text)-1)
		v.resetCommand()
		return text, cursor, true
	case "^":
		cursor = firstNonBlank(text)
		v.resetCommand()
		return text, cursor, true
	case "w":
		for i := 0; i < count; i++ {
			cursor = nextWordStart(text, cursor)
		}
		v.resetCommand()
		return text, cursor, true
	case "b":
		for i := 0; i < count; i++ {
			cursor = prevWordStart(text, cursor)
		}
		v.resetCommand()
		return text, cursor, true
	case "e":
		for i := 0; i < count; i++ {
			cursor = wordEnd(text, cursor)
		}
		v.resetCommand()
		return text, cursor, true

	// Operators
	case "d":
		if v.Command.Type == CmdOperator && v.Command.Op == OpDelete {
			// dd - delete whole line
			v.resetCommand()
			return "", 0, true
		}
		v.Command.Type = CmdOperator
		v.Command.Op = OpDelete
		return text, cursor, true
	case "c":
		if v.Command.Type == CmdOperator && v.Command.Op == OpChange {
			// cc - change whole line
			v.Mode = VimInsert
			v.resetCommand()
			return "", 0, true
		}
		v.Command.Type = CmdOperator
		v.Command.Op = OpChange
		return text, cursor, true
	case "y":
		if v.Command.Type == CmdOperator && v.Command.Op == OpYank {
			// yy - yank whole line
			v.Persistent.Register = text
			v.Persistent.RegisterLine = true
			v.resetCommand()
			return text, cursor, true
		}
		v.Command.Type = CmdOperator
		v.Command.Op = OpYank
		return text, cursor, true

	// Single-char operations
	case "x":
		if cursor < len(text) {
			text = text[:cursor] + text[cursor+1:]
			if cursor >= len(text) && cursor > 0 {
				cursor--
			}
		}
		v.Persistent.LastChange = &RecordedChange{
			Type: "x",
			Keys: []tea.KeyMsg{msg},
		}
		v.resetCommand()
		return text, cursor, true
	case "X":
		if cursor > 0 {
			text = text[:cursor-1] + text[cursor:]
			cursor--
		}
		v.resetCommand()
		return text, cursor, true
	case "r":
		v.Command.Type = CmdReplace
		return text, cursor, true
	case "~":
		if cursor < len(text) {
			r := rune(text[cursor])
			if unicode.IsLower(r) {
				r = unicode.ToUpper(r)
			} else {
				r = unicode.ToLower(r)
			}
			text = text[:cursor] + string(r) + text[cursor+1:]
			if cursor < len(text)-1 {
				cursor++
			}
		}
		v.resetCommand()
		return text, cursor, true

	// Find
	case "f", "F", "t", "T":
		v.Command.FindType = key[0]
		if v.Command.Type == CmdOperator {
			v.Command.Type = CmdOperatorFind
		} else {
			v.Command.Type = CmdFind
		}
		return text, cursor, true

	// Paste
	case "p":
		if v.Persistent.Register != "" {
			text = text[:cursor+1] + v.Persistent.Register + text[cursor+1:]
			cursor += len(v.Persistent.Register)
		}
		v.resetCommand()
		return text, cursor, true
	case "P":
		if v.Persistent.Register != "" {
			text = text[:cursor] + v.Persistent.Register + text[cursor:]
			cursor += len(v.Persistent.Register) - 1
		}
		v.resetCommand()
		return text, cursor, true

	// Dot-repeat
	case ".":
		if v.Persistent.LastChange != nil {
			for _, k := range v.Persistent.LastChange.Keys {
				text, cursor, _ = v.HandleKey(k, text, cursor)
			}
		}
		return text, cursor, true

	// Semicolon - repeat last find
	case ";":
		if v.Persistent.LastFind != 0 {
			v.Command.FindType = v.Persistent.LastFindType
			v.Command.Type = CmdFind
			fakeKey := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{rune(v.Persistent.LastFind)}}
			return v.handleFindChar(fakeKey, text, cursor)
		}
		return text, cursor, true

	// Comma - repeat last find in opposite direction
	case ",":
		if v.Persistent.LastFind != 0 {
			opposite := v.Persistent.LastFindType
			switch opposite {
			case 'f':
				opposite = 'F'
			case 'F':
				opposite = 'f'
			case 't':
				opposite = 'T'
			case 'T':
				opposite = 't'
			}
			v.Command.FindType = opposite
			v.Command.Type = CmdFind
			fakeKey := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{rune(v.Persistent.LastFind)}}
			return v.handleFindChar(fakeKey, text, cursor)
		}
		return text, cursor, true

	// Undo placeholder (u)
	case "u":
		v.resetCommand()
		return text, cursor, true

	// Escape
	case "esc":
		v.resetCommand()
		return text, cursor, true
	}

	v.resetCommand()
	return text, cursor, true
}

func (v *VimState) handleFindChar(msg tea.KeyMsg, text string, cursor int) (string, int, bool) {
	key := msg.String()
	if len(key) != 1 {
		v.resetCommand()
		return text, cursor, true
	}

	ch := key[0]
	v.Persistent.LastFind = ch
	v.Persistent.LastFindType = v.Command.FindType

	newCursor := cursor
	switch v.Command.FindType {
	case 'f':
		newCursor = findCharForward(text, cursor, ch)
	case 'F':
		newCursor = findCharBackward(text, cursor, ch)
	case 't':
		pos := findCharForward(text, cursor, ch)
		if pos > cursor {
			newCursor = pos - 1
		}
	case 'T':
		pos := findCharBackward(text, cursor, ch)
		if pos < cursor {
			newCursor = pos + 1
		}
	}

	if v.Command.Type == CmdOperatorFind && newCursor != cursor {
		start, end := cursor, newCursor
		if start > end {
			start, end = end, start
		}
		end++ // inclusive
		text, cursor = v.applyOperator(text, start, end, cursor)
	} else {
		cursor = newCursor
	}

	v.resetCommand()
	return text, cursor, true
}

func (v *VimState) handleReplace(msg tea.KeyMsg, text string, cursor int) (string, int, bool) {
	key := msg.String()
	if len(key) == 1 && cursor < len(text) {
		text = text[:cursor] + key + text[cursor+1:]
	}
	v.resetCommand()
	return text, cursor, true
}

func (v *VimState) applyOperator(text string, start, end, cursor int) (string, int) {
	if start < 0 {
		start = 0
	}
	if end > len(text) {
		end = len(text)
	}

	switch v.Command.Op {
	case OpDelete:
		v.Persistent.Register = text[start:end]
		text = text[:start] + text[end:]
		cursor = start
		if cursor >= len(text) && cursor > 0 {
			cursor = len(text) - 1
		}
	case OpChange:
		v.Persistent.Register = text[start:end]
		text = text[:start] + text[end:]
		cursor = start
		v.Mode = VimInsert
	case OpYank:
		v.Persistent.Register = text[start:end]
	}
	return text, cursor
}

func (v *VimState) resetCommand() {
	v.Command = CommandState{}
}

// Motion helpers

func firstNonBlank(text string) int {
	for i, r := range text {
		if !unicode.IsSpace(r) {
			return i
		}
	}
	return 0
}

func nextWordStart(text string, pos int) int {
	if pos >= len(text)-1 {
		return pos
	}
	// Skip current word
	i := pos + 1
	for i < len(text) && !isWordBoundary(text, i) {
		i++
	}
	// Skip whitespace
	for i < len(text) && unicode.IsSpace(rune(text[i])) {
		i++
	}
	if i >= len(text) {
		return len(text) - 1
	}
	return i
}

func prevWordStart(text string, pos int) int {
	if pos <= 0 {
		return 0
	}
	i := pos - 1
	// Skip whitespace
	for i > 0 && unicode.IsSpace(rune(text[i])) {
		i--
	}
	if i == 0 {
		return 0
	}
	// Determine current character class and skip same class
	if isWordChar(text, i) {
		for i > 0 && isWordChar(text, i-1) {
			i--
		}
	} else {
		for i > 0 && !isWordChar(text, i-1) && !unicode.IsSpace(rune(text[i-1])) {
			i--
		}
	}
	return i
}

func wordEnd(text string, pos int) int {
	if pos >= len(text)-1 {
		return pos
	}
	i := pos + 1
	// Skip whitespace
	for i < len(text) && unicode.IsSpace(rune(text[i])) {
		i++
	}
	// Move to end of word
	for i < len(text)-1 && !isWordBoundary(text, i+1) {
		i++
	}
	return i
}

func isWordBoundary(text string, pos int) bool {
	if pos <= 0 || pos >= len(text) {
		return true
	}
	curr := rune(text[pos])
	prev := rune(text[pos-1])
	currWord := unicode.IsLetter(curr) || unicode.IsDigit(curr) || curr == '_'
	prevWord := unicode.IsLetter(prev) || unicode.IsDigit(prev) || prev == '_'
	return currWord != prevWord
}

func findCharForward(text string, pos int, ch byte) int {
	for i := pos + 1; i < len(text); i++ {
		if text[i] == ch {
			return i
		}
	}
	return pos
}

func findCharBackward(text string, pos int, ch byte) int {
	for i := pos - 1; i >= 0; i-- {
		if text[i] == ch {
			return i
		}
	}
	return pos
}

func atoi(s string) int {
	n := 0
	for _, c := range s {
		n = n*10 + int(c-'0')
	}
	return n
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func isWordChar(s string, pos int) bool {
	if pos < 0 || pos >= len(s) {
		return false
	}
	r := rune(s[pos])
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_'
}

// unused but available for text objects
var _ = strings.ContainsRune
