package daemon

import (
	"testing"
	"time"
)

func TestPreheater_StartStop(t *testing.T) {
	p := NewPreheater(100 * time.Millisecond)
	p.Start([]string{})
	time.Sleep(150 * time.Millisecond)
	if !p.Ready() {
		t.Error("expected preheater to be ready after warmup")
	}
	p.Stop()
	if p.Ready() {
		t.Error("expected preheater to not be ready after stop")
	}
}

func TestPreheater_Transport(t *testing.T) {
	p := NewPreheater(time.Second)
	tr := p.Transport()
	if tr == nil {
		t.Fatal("expected non-nil transport")
	}
	if tr.MaxIdleConns != 10 {
		t.Errorf("expected MaxIdleConns=10, got %d", tr.MaxIdleConns)
	}
}

func TestPreheater_DoubleStart(t *testing.T) {
	p := NewPreheater(time.Second)
	p.Start([]string{})
	p.Start([]string{}) // should not panic
	p.Stop()
}
