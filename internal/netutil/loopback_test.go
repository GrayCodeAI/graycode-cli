package netutil

import "testing"

func TestLoopbackConstants(t *testing.T) {
	if LoopbackHost != "127.0.0.1" {
		t.Errorf("LoopbackHost = %q, want %q", LoopbackHost, "127.0.0.1")
	}
	if LoopbackDynamicAddr != "127.0.0.1:0" {
		t.Errorf("LoopbackDynamicAddr = %q, want %q", LoopbackDynamicAddr, "127.0.0.1:0")
	}
	if LoopbackNoProxy != "localhost,127.0.0.1" {
		t.Errorf("LoopbackNoProxy = %q, want %q", LoopbackNoProxy, "localhost,127.0.0.1")
	}
}
