// Package profile provides runtime profiling helpers.
package profile

import (
	"fmt"
	"os"
	"runtime"
	"runtime/pprof"
	"time"
)

// CPUProfile starts CPU profiling to the given file.
func CPUProfile(path string) (func(), error) {
	f, err := os.Create(path)
	if err != nil {
		return nil, fmt.Errorf("create profile file: %w", err)
	}
	if err := pprof.StartCPUProfile(f); err != nil {
		f.Close()
		return nil, fmt.Errorf("start CPU profile: %w", err)
	}
	return func() {
		pprof.StopCPUProfile()
		f.Close()
	}, nil
}

// MemoryProfile writes a heap profile to the given file.
func MemoryProfile(path string) error {
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create profile file: %w", err)
	}
	defer f.Close()

	runtime.GC()
	if err := pprof.WriteHeapProfile(f); err != nil {
		return fmt.Errorf("write heap profile: %w", err)
	}
	return nil
}

// GoroutineProfile writes a goroutine profile to the given file.
func GoroutineProfile(path string) error {
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create profile file: %w", err)
	}
	defer f.Close()

	if err := pprof.Lookup("goroutine").WriteTo(f, 1); err != nil {
		return fmt.Errorf("write goroutine profile: %w", err)
	}
	return nil
}

// Stats returns runtime statistics.
type Stats struct {
	NumGoroutine int    `json:"num_goroutine"`
	NumCPU       int    `json:"num_cpu"`
	MemAlloc     uint64 `json:"mem_alloc"`
	MemTotalAlloc uint64 `json:"mem_total_alloc"`
	MemSys       uint64 `json:"mem_sys"`
	MemNumGC     uint32 `json:"mem_num_gc"`
	Timestamp    int64  `json:"timestamp"`
}

// GetStats returns current runtime statistics.
func GetStats() Stats {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return Stats{
		NumGoroutine:  runtime.NumGoroutine(),
		NumCPU:        runtime.NumCPU(),
		MemAlloc:      m.Alloc,
		MemTotalAlloc: m.TotalAlloc,
		MemSys:        m.Sys,
		MemNumGC:      m.NumGC,
		Timestamp:     time.Now().Unix(),
	}
}

// Format returns formatted runtime stats.
func Format(s Stats) string {
	return fmt.Sprintf(
		"goroutines: %d | CPU: %d | alloc: %.2f MB | total: %.2f MB | sys: %.2f MB | GC: %d",
		s.NumGoroutine, s.NumCPU,
		float64(s.MemAlloc)/(1024*1024),
		float64(s.MemTotalAlloc)/(1024*1024),
		float64(s.MemSys)/(1024*1024),
		s.MemNumGC,
	)
}
