package metrics

import (
	"sync"
	"testing"
)

func TestToolAggRecordAndGet(t *testing.T) {
	agg := NewToolAggregator()

	agg.Record("Read", 1024, 50, false)
	agg.Record("Read", 2048, 30, false)
	agg.Record("Read", 512, 10, true)

	got := agg.Get("Read")
	if got == nil {
		t.Fatal("expected aggregate for Read, got nil")
	}

	if got.CallCount != 3 {
		t.Errorf("CallCount: want 3, got %d", got.CallCount)
	}
	if got.TotalBytes != 3584 {
		t.Errorf("TotalBytes: want 3584, got %d", got.TotalBytes)
	}
	if got.TotalMs != 90 {
		t.Errorf("TotalMs: want 90, got %d", got.TotalMs)
	}
	if got.Errors != 1 {
		t.Errorf("Errors: want 1, got %d", got.Errors)
	}
}

func TestToolAggMultipleTools(t *testing.T) {
	agg := NewToolAggregator()

	agg.Record("Read", 100, 10, false)
	agg.Record("Write", 200, 20, false)
	agg.Record("Bash", 300, 30, true)

	all := agg.All()
	if len(all) != 3 {
		t.Fatalf("expected 3 tools, got %d", len(all))
	}

	if all["Read"].CallCount != 1 {
		t.Errorf("Read CallCount: want 1, got %d", all["Read"].CallCount)
	}
	if all["Write"].TotalBytes != 200 {
		t.Errorf("Write TotalBytes: want 200, got %d", all["Write"].TotalBytes)
	}
	if all["Bash"].Errors != 1 {
		t.Errorf("Bash Errors: want 1, got %d", all["Bash"].Errors)
	}

	// Verify Get returns nil for unknown tool.
	if agg.Get("Unknown") != nil {
		t.Error("expected nil for unknown tool")
	}
}

func TestToolAggReset(t *testing.T) {
	agg := NewToolAggregator()

	agg.Record("Read", 100, 10, false)
	agg.Record("Write", 200, 20, false)

	agg.Reset()

	all := agg.All()
	if len(all) != 0 {
		t.Errorf("expected empty after reset, got %d tools", len(all))
	}

	if agg.Get("Read") != nil {
		t.Error("expected nil for Read after reset")
	}
}

func TestToolAggConcurrentAccess(t *testing.T) {
	agg := NewToolAggregator()

	const goroutines = 50
	const recordsPerGoroutine = 100

	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func(id int) {
			defer wg.Done()
			toolName := "Tool"
			for j := 0; j < recordsPerGoroutine; j++ {
				agg.Record(toolName, 10, 1, j%10 == 0)
			}
		}(i)
	}

	wg.Wait()

	got := agg.Get("Tool")
	if got == nil {
		t.Fatal("expected aggregate for Tool")
	}

	expectedCalls := goroutines * recordsPerGoroutine
	if got.CallCount != expectedCalls {
		t.Errorf("CallCount: want %d, got %d", expectedCalls, got.CallCount)
	}

	expectedBytes := int64(expectedCalls * 10)
	if got.TotalBytes != expectedBytes {
		t.Errorf("TotalBytes: want %d, got %d", expectedBytes, got.TotalBytes)
	}

	expectedMs := int64(expectedCalls * 1)
	if got.TotalMs != expectedMs {
		t.Errorf("TotalMs: want %d, got %d", expectedMs, got.TotalMs)
	}

	// Every 10th record is an error: recordsPerGoroutine/10 * goroutines
	expectedErrors := goroutines * (recordsPerGoroutine / 10)
	if got.Errors != expectedErrors {
		t.Errorf("Errors: want %d, got %d", expectedErrors, got.Errors)
	}
}
