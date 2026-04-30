package profile

import (
	"os"
	"strings"
	"testing"
)

func TestGetStats(t *testing.T) {
	s := GetStats()
	if s.NumGoroutine == 0 {
		t.Fatal("expected some goroutines")
	}
	if s.NumCPU == 0 {
		t.Fatal("expected some CPUs")
	}
}

func TestFormat(t *testing.T) {
	s := Stats{
		NumGoroutine:  10,
		NumCPU:        8,
		MemAlloc:      1024 * 1024,
		MemTotalAlloc: 2 * 1024 * 1024,
		MemSys:        4 * 1024 * 1024,
		MemNumGC:      5,
	}
	formatted := Format(s)
	if !strings.Contains(formatted, "goroutines: 10") {
		t.Fatalf("expected goroutines in output, got: %s", formatted)
	}
}

func TestMemoryProfile(t *testing.T) {
	path := "/tmp/test_heap.prof"
	defer os.Remove(path)

	err := MemoryProfile(path)
	if err != nil {
		t.Fatal(err)
	}

	_, err = os.Stat(path)
	if err != nil {
		t.Fatal("profile file not created")
	}
}

func TestGoroutineProfile(t *testing.T) {
	path := "/tmp/test_goroutine.prof"
	defer os.Remove(path)

	err := GoroutineProfile(path)
	if err != nil {
		t.Fatal(err)
	}

	_, err = os.Stat(path)
	if err != nil {
		t.Fatal("profile file not created")
	}
}
