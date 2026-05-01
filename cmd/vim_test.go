package cmd

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func key(s string) tea.KeyMsg {
	if len(s) == 1 {
		return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
	}
	switch s {
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEscape}
	case "left":
		return tea.KeyMsg{Type: tea.KeyLeft}
	case "right":
		return tea.KeyMsg{Type: tea.KeyRight}
	}
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

func TestVimState_InsertToNormal(t *testing.T) {
	v := NewVimState()
	if v.Mode != VimInsert {
		t.Error("should start in insert mode")
	}

	text, cursor, consumed := v.HandleKey(key("esc"), "hello", 3)
	if !consumed {
		t.Error("escape should be consumed")
	}
	if v.Mode != VimNormal {
		t.Error("should switch to normal mode")
	}
	if cursor != 2 {
		t.Errorf("cursor should move back to 2, got %d", cursor)
	}
	_ = text
}

func TestVimState_NormalToInsert(t *testing.T) {
	v := NewVimState()
	v.Mode = VimNormal

	_, _, _ = v.HandleKey(key("i"), "hello", 2)
	if v.Mode != VimInsert {
		t.Error("'i' should switch to insert mode")
	}
}

func TestVimState_MotionH(t *testing.T) {
	v := NewVimState()
	v.Mode = VimNormal

	_, cursor, _ := v.HandleKey(key("h"), "hello", 3)
	if cursor != 2 {
		t.Errorf("h should move left: expected 2, got %d", cursor)
	}
}

func TestVimState_MotionL(t *testing.T) {
	v := NewVimState()
	v.Mode = VimNormal

	_, cursor, _ := v.HandleKey(key("l"), "hello", 2)
	if cursor != 3 {
		t.Errorf("l should move right: expected 3, got %d", cursor)
	}
}

func TestVimState_Motion0Dollar(t *testing.T) {
	v := NewVimState()
	v.Mode = VimNormal

	_, cursor, _ := v.HandleKey(key("0"), "hello world", 5)
	if cursor != 0 {
		t.Errorf("0 should go to start: expected 0, got %d", cursor)
	}

	_, cursor, _ = v.HandleKey(key("$"), "hello world", 0)
	if cursor != 10 {
		t.Errorf("$ should go to end: expected 10, got %d", cursor)
	}
}

func TestVimState_MotionW(t *testing.T) {
	v := NewVimState()
	v.Mode = VimNormal

	_, cursor, _ := v.HandleKey(key("w"), "hello world", 0)
	if cursor < 5 || cursor > 6 {
		t.Errorf("w should move to next word: expected ~6, got %d", cursor)
	}
}

func TestVimState_MotionB(t *testing.T) {
	v := NewVimState()
	v.Mode = VimNormal

	// "hello world" - b from pos 10 ('d') should move backward into 'world'
	_, cursor, _ := v.HandleKey(key("b"), "hello world", 10)
	if cursor >= 10 {
		t.Errorf("b should move left from 10, got %d", cursor)
	}
	// Second b should reach "hello"
	_, cursor2, _ := v.HandleKey(key("b"), "hello world", cursor)
	if cursor2 >= cursor {
		t.Errorf("second b should move further left: from %d got %d", cursor, cursor2)
	}
}

func TestVimState_DeleteX(t *testing.T) {
	v := NewVimState()
	v.Mode = VimNormal

	text, cursor, _ := v.HandleKey(key("x"), "hello", 2)
	if text != "helo" {
		t.Errorf("x should delete char: expected 'helo', got %q", text)
	}
	if cursor != 2 {
		t.Errorf("cursor should stay at 2, got %d", cursor)
	}
}

func TestVimState_DeleteX_End(t *testing.T) {
	v := NewVimState()
	v.Mode = VimNormal

	text, cursor, _ := v.HandleKey(key("x"), "hi", 1)
	if text != "h" {
		t.Errorf("expected 'h', got %q", text)
	}
	if cursor != 0 {
		t.Errorf("cursor should adjust to 0, got %d", cursor)
	}
}

func TestVimState_DD(t *testing.T) {
	v := NewVimState()
	v.Mode = VimNormal

	v.HandleKey(key("d"), "hello world", 3)
	text, cursor, _ := v.HandleKey(key("d"), "hello world", 3)
	if text != "" {
		t.Errorf("dd should clear line, got %q", text)
	}
	if cursor != 0 {
		t.Errorf("cursor should be 0, got %d", cursor)
	}
}

func TestVimState_CC(t *testing.T) {
	v := NewVimState()
	v.Mode = VimNormal

	v.HandleKey(key("c"), "hello", 2)
	text, _, _ := v.HandleKey(key("c"), "hello", 2)
	if text != "" {
		t.Errorf("cc should clear line, got %q", text)
	}
	if v.Mode != VimInsert {
		t.Error("cc should enter insert mode")
	}
}

func TestVimState_YY(t *testing.T) {
	v := NewVimState()
	v.Mode = VimNormal

	v.HandleKey(key("y"), "hello", 2)
	v.HandleKey(key("y"), "hello", 2)
	if v.Persistent.Register != "hello" {
		t.Errorf("yy should yank line, got %q", v.Persistent.Register)
	}
}

func TestVimState_Replace(t *testing.T) {
	v := NewVimState()
	v.Mode = VimNormal

	v.HandleKey(key("r"), "hello", 1)
	text, _, _ := v.HandleKey(key("a"), "hello", 1)
	if text != "hallo" {
		t.Errorf("r should replace char: expected 'hallo', got %q", text)
	}
	if v.Mode != VimNormal {
		t.Error("should stay in normal mode after replace")
	}
}

func TestVimState_ToggleCase(t *testing.T) {
	v := NewVimState()
	v.Mode = VimNormal

	text, cursor, _ := v.HandleKey(key("~"), "Hello", 0)
	if text != "hello" {
		t.Errorf("~ should toggle H->h: expected 'hello', got %q", text)
	}
	if cursor != 1 {
		t.Errorf("~ should advance cursor: expected 1, got %d", cursor)
	}
}

func TestVimState_FindF(t *testing.T) {
	v := NewVimState()
	v.Mode = VimNormal

	v.HandleKey(key("f"), "hello world", 0)
	_, cursor, _ := v.HandleKey(key("o"), "hello world", 0)
	if cursor != 4 {
		t.Errorf("f+o should find 'o': expected 4, got %d", cursor)
	}
}

func TestVimState_FindF_Backward(t *testing.T) {
	v := NewVimState()
	v.Mode = VimNormal

	v.HandleKey(key("F"), "hello world", 8)
	_, cursor, _ := v.HandleKey(key("o"), "hello world", 8)
	if cursor != 7 {
		t.Errorf("F+o should find 'o' backward: expected 7, got %d", cursor)
	}
}

func TestVimState_Paste(t *testing.T) {
	v := NewVimState()
	v.Mode = VimNormal
	v.Persistent.Register = "XY"

	text, cursor, _ := v.HandleKey(key("p"), "hello", 2)
	if text != "helXYlo" {
		t.Errorf("p should paste after cursor: expected 'helXYlo', got %q", text)
	}
	if cursor != 4 {
		t.Errorf("cursor should be at end of paste: expected 4, got %d", cursor)
	}
}

func TestVimState_InsertA(t *testing.T) {
	v := NewVimState()
	v.Mode = VimNormal

	_, cursor, _ := v.HandleKey(key("A"), "hello", 2)
	if cursor != 5 {
		t.Errorf("A should go to end: expected 5, got %d", cursor)
	}
	if v.Mode != VimInsert {
		t.Error("A should enter insert mode")
	}
}

func TestVimState_InsertI_Uppercase(t *testing.T) {
	v := NewVimState()
	v.Mode = VimNormal

	_, cursor, _ := v.HandleKey(key("I"), "  hello", 4)
	if cursor != 2 {
		t.Errorf("I should go to first non-blank: expected 2, got %d", cursor)
	}
	if v.Mode != VimInsert {
		t.Error("I should enter insert mode")
	}
}

func TestVimState_Count(t *testing.T) {
	v := NewVimState()
	v.Mode = VimNormal

	v.HandleKey(key("3"), "hello world!", 0)
	_, cursor, _ := v.HandleKey(key("l"), "hello world!", 0)
	if cursor != 3 {
		t.Errorf("3l should move 3 right: expected 3, got %d", cursor)
	}
}

func TestVimState_Disabled(t *testing.T) {
	v := NewVimState()
	v.SetEnabled(false)

	_, _, consumed := v.HandleKey(key("esc"), "hello", 3)
	if consumed {
		t.Error("disabled vim should not consume keys")
	}
}

func TestVimState_ModeString(t *testing.T) {
	v := NewVimState()
	if v.ModeString() != "INSERT" {
		t.Errorf("expected INSERT, got %s", v.ModeString())
	}
	v.Mode = VimNormal
	if v.ModeString() != "NORMAL" {
		t.Errorf("expected NORMAL, got %s", v.ModeString())
	}
	v.SetEnabled(false)
	if v.ModeString() != "" {
		t.Errorf("expected empty when disabled, got %s", v.ModeString())
	}
}
