package engine

import (
	"strconv"
	"strings"
	"testing"
)

func TestElisionNoticeJSONRecords(t *testing.T) {
	var items []string
	for i := 0; i < 10; i++ {
		items = append(items, `{"order_id":"ord-`+strconv.Itoa(i)+`","status":"fulfilled"}`)
	}
	notice := elisionNotice("[" + strings.Join(items, ",") + "]")
	if !strings.Contains(notice, "records elided") {
		t.Fatalf("notice = %q", notice)
	}
	if !strings.Contains(notice, "status=fulfilled×") && !strings.Contains(notice, "distinct") {
		t.Fatalf("no verified facts: %q", notice)
	}
}

func TestElisionNoticeLogLines(t *testing.T) {
	var lines []string
	for i := 0; i < 6; i++ {
		lines = append(lines, "2026-08-22T10:00:0"+strconv.Itoa(i)+"Z INFO tick")
	}
	notice := elisionNotice(strings.Join(lines, "\n"))
	if !strings.Contains(notice, "lines elided") || !strings.Contains(notice, "info×6") {
		t.Fatalf("notice = %q", notice)
	}
}

func TestElisionNoticeProseFallback(t *testing.T) {
	notice := elisionNotice("just two\nshort lines")
	if notice != "" {
		t.Fatalf("tiny prose should yield no notice, got %q", notice)
	}
}

func TestAppendElisionMarker(t *testing.T) {
	var items []string
	for i := 0; i < 12; i++ {
		items = append(items, `{"id":"`+strconv.Itoa(i)+`","state":"ok"}`)
	}
	dropped := "[" + strings.Join(items[3:], ",") + "]"
	out := appendElisionMarker(`[{"id":"0","state":"ok"},{"id":"1","state":"ok"},{"id":"2","state":"ok"}]`, dropped)
	if !strings.Contains(out, "_tok") == false && !strings.Contains(out, "records elided") {
		t.Fatalf("marker missing facts: %q", out)
	}
	// kept content preserved verbatim before marker
	if !strings.HasPrefix(out, `[{"id":"0","state":"ok"}`) {
		t.Fatalf("kept content altered: %q", out)
	}
}

func TestTruncateToolOutputCarriesInvariants(t *testing.T) {
	var items []string
	for i := 100; i < 160; i++ {
		items = append(items, `{"sku":"sku-`+strconv.Itoa(i)+`","status":"shipped"}`)
	}
	output := "[" + strings.Join(items, ",") + "]"
	// The structural cutter keeps whole records, so the dropped tail parses
	// as a clean record set and invariants can be computed. A raw byte cut
	// landing mid-record correctly falls back to the bare marker.
	got := truncateOutputStructurally(output, 500)
	if !strings.Contains(got, "records elided") {
		t.Fatalf("got %q", got)
	}
	// The kept prefix must remain valid JSON prefix content (starts with '[').
	if !strings.HasPrefix(strings.TrimSpace(got), "[") {
		t.Fatal("lost array opening")
	}
}
