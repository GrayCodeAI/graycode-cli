package cmd

import (
	"strings"
	"testing"
)

func TestRenderPrintResponse(t *testing.T) {
	const md = "## Hello **world**\n\n- one\n- two\n"

	// markdown off -> raw, regardless of color
	if got := renderPrintResponse(md, false, true); got != md {
		t.Errorf("markdown=false: got %q, want raw %q", got, md)
	}
	// color off -> raw, regardless of markdown
	if got := renderPrintResponse(md, true, false); got != md {
		t.Errorf("color=false: got %q, want raw %q", got, md)
	}
	// empty stays empty
	if got := renderPrintResponse("", true, true); got != "" {
		t.Errorf("empty: got %q, want empty", got)
	}
	// markdown + color -> styled ANSI, raw markers stripped
	got := renderPrintResponse(md, true, true)
	if !strings.Contains(got, "\x1b[") {
		t.Errorf("markdown+color: no ANSI in %q", got)
	}
	if strings.Contains(got, "**world**") {
		t.Errorf("markdown+color: raw ** still present in %q", got)
	}
}
