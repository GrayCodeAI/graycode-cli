package cmd

import (
	"strings"
	"testing"
	"time"
)

func TestStartProfile(t *testing.T) {
	p := startProfile()
	if p == nil {
		t.Fatal("startProfile returned nil")
	}
	if p.StartTime.IsZero() {
		t.Fatal("StartTime should be set")
	}
}

func TestQueryProfile_RecordTTFT(t *testing.T) {
	p := startProfile()
	time.Sleep(5 * time.Millisecond)
	p.RecordTTFT()
	if p.TTFT == 0 {
		t.Fatal("TTFT should be non-zero after recording")
	}
	// Second call should not change TTFT
	first := p.TTFT
	time.Sleep(5 * time.Millisecond)
	p.RecordTTFT()
	if p.TTFT != first {
		t.Fatal("TTFT should not change on second RecordTTFT call")
	}
}

func TestQueryProfile_ToolExec(t *testing.T) {
	p := startProfile()
	p.RecordToolExecStart()
	time.Sleep(5 * time.Millisecond)
	p.RecordToolExecEnd()
	if p.ToolExec == 0 {
		t.Fatal("ToolExec should be non-zero")
	}

	// Accumulates across multiple tool calls
	first := p.ToolExec
	p.RecordToolExecStart()
	time.Sleep(5 * time.Millisecond)
	p.RecordToolExecEnd()
	if p.ToolExec <= first {
		t.Fatal("ToolExec should accumulate across calls")
	}
}

func TestQueryProfile_Finish(t *testing.T) {
	p := startProfile()
	time.Sleep(5 * time.Millisecond)
	p.Finish()
	if p.TotalTime == 0 {
		t.Fatal("TotalTime should be non-zero after Finish")
	}
}

func TestQueryProfile_String(t *testing.T) {
	p := startProfile()
	p.TokensIn = 100
	p.TokensOut = 50
	p.RecordTTFT()
	p.Finish()

	s := p.String()
	if !strings.Contains(s, "Profile:") {
		t.Fatalf("expected Profile: prefix, got %q", s)
	}
	if !strings.Contains(s, "tokens_in=100") {
		t.Fatalf("expected tokens_in=100 in output, got %q", s)
	}
	if !strings.Contains(s, "tokens_out=50") {
		t.Fatalf("expected tokens_out=50 in output, got %q", s)
	}
}
