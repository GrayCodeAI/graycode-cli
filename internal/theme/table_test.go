package theme

import (
	"bytes"
	"strings"
	"testing"
)

func stripANSI(s string) string {
	return ansiRe.ReplaceAllString(s, "")
}

func TestPrintTableAlignsColumns(t *testing.T) {
	var buf bytes.Buffer
	err := PrintTable(
		&buf,
		[]string{"NAME", "VERSION", "STATE"},
		[][]string{
			{"foo", "1.0", "active"},
			{"verylongname", "2.0", "enabled"},
		},
	)
	if err != nil {
		t.Fatalf("PrintTable returned error: %v", err)
	}
	got := stripANSI(buf.String())
	want := "" +
		"NAME          VERSION  STATE\n" +
		"foo           1.0      active\n" +
		"verylongname  2.0      enabled\n"
	if got != want {
		t.Errorf("unexpected table:\n got:\n%q\nwant:\n%q", got, want)
	}
}

func TestPrintTableColorDoesNotBreakAlignment(t *testing.T) {
	// Colorizing a header/value must not shift columns: widths are computed
	// on visible (ANSI-stripped) width.
	var buf bytes.Buffer
	err := PrintTable(
		&buf,
		[]string{"NAME", "VERSION", "STATE"},
		[][]string{
			{"foo", "1.0", Tint("active", ReportSuccess)},
		},
	)
	if err != nil {
		t.Fatalf("PrintTable returned error: %v", err)
	}
	got := stripANSI(buf.String())
	want := "" +
		"NAME  VERSION  STATE\n" +
		"foo   1.0      active\n"
	if got != want {
		t.Errorf("unexpected table:\n got:\n%q\nwant:\n%q", got, want)
	}
}

func TestPrintTableShortRows(t *testing.T) {
	var buf bytes.Buffer
	err := PrintTable(
		&buf,
		[]string{"A", "B", "C"},
		[][]string{{"1"}},
	)
	if err != nil {
		t.Fatalf("PrintTable returned error: %v", err)
	}
	got := stripANSI(buf.String())
	if !strings.Contains(got, "1") {
		t.Errorf("short row not emitted, got: %q", got)
	}
}

func TestPrintTableEmptyHeaderNoop(t *testing.T) {
	var buf bytes.Buffer
	if err := PrintTable(&buf, nil, [][]string{{"x"}}); err != nil {
		t.Fatalf("PrintTable returned error: %v", err)
	}
	if buf.Len() != 0 {
		t.Errorf("expected no output for empty header, got: %q", buf.String())
	}
}
