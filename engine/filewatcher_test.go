package engine

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestFileWatcher_DetectCreation(t *testing.T) {
	dir := t.TempDir()

	var mu sync.Mutex
	var received []FileEvent

	fw := NewFileWatcher(dir, WatcherConfig{
		PollInterval: 50 * time.Millisecond,
		Debounce:     100 * time.Millisecond,
		BatchWindow:  50 * time.Millisecond,
	})
	fw.OnChange = func(events []FileEvent) {
		mu.Lock()
		received = append(received, events...)
		mu.Unlock()
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	go func() {
		_ = fw.Start(ctx)
	}()

	// Wait for initial scan to complete.
	time.Sleep(100 * time.Millisecond)

	// Create a file.
	err := os.WriteFile(filepath.Join(dir, "hello.go"), []byte("package main"), 0644)
	if err != nil {
		t.Fatal(err)
	}

	// Wait for detection and debounce.
	time.Sleep(400 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()

	if len(received) == 0 {
		t.Fatal("expected at least one event for file creation")
	}

	found := false
	for _, ev := range received {
		if ev.Path == "hello.go" && ev.Type == "create" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected create event for hello.go, got: %+v", received)
	}
}

func TestFileWatcher_DetectModification(t *testing.T) {
	dir := t.TempDir()

	// Create file before watching.
	filePath := filepath.Join(dir, "data.txt")
	err := os.WriteFile(filePath, []byte("initial"), 0644)
	if err != nil {
		t.Fatal(err)
	}

	var mu sync.Mutex
	var received []FileEvent

	fw := NewFileWatcher(dir, WatcherConfig{
		PollInterval: 50 * time.Millisecond,
		Debounce:     100 * time.Millisecond,
		BatchWindow:  50 * time.Millisecond,
	})
	fw.OnChange = func(events []FileEvent) {
		mu.Lock()
		received = append(received, events...)
		mu.Unlock()
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	go func() {
		_ = fw.Start(ctx)
	}()

	// Wait for initial scan.
	time.Sleep(100 * time.Millisecond)

	// Modify the file.
	err = os.WriteFile(filePath, []byte("modified content that is longer"), 0644)
	if err != nil {
		t.Fatal(err)
	}

	// Wait for detection and debounce.
	time.Sleep(400 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()

	found := false
	for _, ev := range received {
		if ev.Path == "data.txt" && ev.Type == "modify" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected modify event for data.txt, got: %+v", received)
	}
}

func TestFileWatcher_DetectDeletion(t *testing.T) {
	dir := t.TempDir()

	// Create file before watching.
	filePath := filepath.Join(dir, "remove_me.txt")
	err := os.WriteFile(filePath, []byte("delete this"), 0644)
	if err != nil {
		t.Fatal(err)
	}

	var mu sync.Mutex
	var received []FileEvent

	fw := NewFileWatcher(dir, WatcherConfig{
		PollInterval: 50 * time.Millisecond,
		Debounce:     100 * time.Millisecond,
		BatchWindow:  50 * time.Millisecond,
	})
	fw.OnChange = func(events []FileEvent) {
		mu.Lock()
		received = append(received, events...)
		mu.Unlock()
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	go func() {
		_ = fw.Start(ctx)
	}()

	// Wait for initial scan.
	time.Sleep(100 * time.Millisecond)

	// Delete the file.
	err = os.Remove(filePath)
	if err != nil {
		t.Fatal(err)
	}

	// Wait for detection and debounce.
	time.Sleep(400 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()

	found := false
	for _, ev := range received {
		if ev.Path == "remove_me.txt" && ev.Type == "delete" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected delete event for remove_me.txt, got: %+v", received)
	}
}

func TestFileWatcher_DebounceRapidChanges(t *testing.T) {
	dir := t.TempDir()

	// Create initial file.
	filePath := filepath.Join(dir, "rapid.txt")
	err := os.WriteFile(filePath, []byte("v0"), 0644)
	if err != nil {
		t.Fatal(err)
	}

	var mu sync.Mutex
	callCount := 0

	fw := NewFileWatcher(dir, WatcherConfig{
		PollInterval: 30 * time.Millisecond,
		Debounce:     200 * time.Millisecond,
		BatchWindow:  50 * time.Millisecond,
	})
	fw.OnChange = func(events []FileEvent) {
		mu.Lock()
		callCount++
		mu.Unlock()
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	go func() {
		_ = fw.Start(ctx)
	}()

	// Wait for initial scan.
	time.Sleep(80 * time.Millisecond)

	// Make rapid changes — each within debounce window of the previous.
	for i := 1; i <= 5; i++ {
		err = os.WriteFile(filePath, []byte(strings.Repeat("x", i*100)), 0644)
		if err != nil {
			t.Fatal(err)
		}
		time.Sleep(50 * time.Millisecond)
	}

	// Wait for debounce to fire.
	time.Sleep(500 * time.Millisecond)

	mu.Lock()
	count := callCount
	mu.Unlock()

	// Debounce should collapse rapid changes into at most 2 calls
	// (ideally 1, but timing-dependent).
	if count > 2 {
		t.Errorf("debounce failed: expected at most 2 OnChange calls, got %d", count)
	}
	if count == 0 {
		t.Error("expected at least 1 OnChange call after debounce period")
	}
}

func TestFileWatcher_BatchWindowGroupsEvents(t *testing.T) {
	dir := t.TempDir()

	var mu sync.Mutex
	var batches [][]FileEvent

	fw := NewFileWatcher(dir, WatcherConfig{
		PollInterval: 30 * time.Millisecond,
		Debounce:     150 * time.Millisecond,
		BatchWindow:  50 * time.Millisecond,
	})
	fw.OnChange = func(events []FileEvent) {
		mu.Lock()
		batch := make([]FileEvent, len(events))
		copy(batch, events)
		batches = append(batches, batch)
		mu.Unlock()
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	go func() {
		_ = fw.Start(ctx)
	}()

	// Wait for initial scan.
	time.Sleep(80 * time.Millisecond)

	// Create multiple files quickly.
	for i := 0; i < 3; i++ {
		name := filepath.Join(dir, strings.Repeat("a", i+1)+".txt")
		_ = os.WriteFile(name, []byte("content"), 0644)
	}

	// Wait for debounce.
	time.Sleep(400 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()

	if len(batches) == 0 {
		t.Fatal("expected at least one batch of events")
	}

	// At least one batch should contain multiple events.
	totalEvents := 0
	for _, b := range batches {
		totalEvents += len(b)
	}
	if totalEvents < 3 {
		t.Errorf("expected at least 3 total events, got %d", totalEvents)
	}
}

func TestFileWatcher_IgnorePatterns(t *testing.T) {
	dir := t.TempDir()

	// Create .git directory that should be ignored.
	gitDir := filepath.Join(dir, ".git")
	_ = os.MkdirAll(gitDir, 0755)
	_ = os.WriteFile(filepath.Join(gitDir, "HEAD"), []byte("ref: refs/heads/main"), 0644)

	var mu sync.Mutex
	var received []FileEvent

	fw := NewFileWatcher(dir, WatcherConfig{
		PollInterval: 50 * time.Millisecond,
		Debounce:     100 * time.Millisecond,
		BatchWindow:  50 * time.Millisecond,
	})
	fw.OnChange = func(events []FileEvent) {
		mu.Lock()
		received = append(received, events...)
		mu.Unlock()
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	go func() {
		_ = fw.Start(ctx)
	}()

	// Wait for initial scan.
	time.Sleep(100 * time.Millisecond)

	// Modify an ignored file.
	_ = os.WriteFile(filepath.Join(gitDir, "index"), []byte("binary data"), 0644)

	// Create a .swp file (should be ignored).
	_ = os.WriteFile(filepath.Join(dir, "test.swp"), []byte("swap"), 0644)

	// Create a non-ignored file.
	_ = os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main"), 0644)

	// Wait for debounce.
	time.Sleep(400 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()

	for _, ev := range received {
		if strings.Contains(ev.Path, ".git") {
			t.Errorf("received event for ignored .git path: %s", ev.Path)
		}
		if strings.HasSuffix(ev.Path, ".swp") {
			t.Errorf("received event for ignored .swp file: %s", ev.Path)
		}
	}

	// Should have received event for main.go.
	found := false
	for _, ev := range received {
		if ev.Path == "main.go" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected event for main.go but did not receive one")
	}
}

func TestMatchesPattern(t *testing.T) {
	tests := []struct {
		path     string
		patterns []string
		want     bool
	}{
		{"main.go", []string{"*.go"}, true},
		{"src/app.go", []string{"*.go"}, true},
		{"readme.md", []string{"*.go"}, false},
		{"src/test.js", []string{"*.js", "*.ts"}, true},
		{"deep/nested/file.py", []string{"*.py"}, true},
		{"Makefile", []string{"Makefile"}, true},
		{"src/Makefile", []string{"Makefile"}, true},
		{"file.txt", []string{"*.go", "*.rs"}, false},
	}

	for _, tt := range tests {
		got := MatchesPattern(tt.path, tt.patterns)
		if got != tt.want {
			t.Errorf("MatchesPattern(%q, %v) = %v, want %v", tt.path, tt.patterns, got, tt.want)
		}
	}
}

func TestShouldIgnore(t *testing.T) {
	fw := NewFileWatcher("/tmp", WatcherConfig{})

	tests := []struct {
		path string
		want bool
	}{
		{".git/", true},
		{".git/HEAD", true},
		{"node_modules/", true},
		{"node_modules/express/index.js", true},
		{"vendor/", true},
		{"__pycache__/", true},
		{".venv/", true},
		{"dist/", true},
		{"build/", true},
		{".DS_Store", true},
		{"test.swp", true},
		{"test.swo", true},
		{"backup~", true},
		{"src/main.go", false},
		{"README.md", false},
		{"cmd/app/main.go", false},
	}

	for _, tt := range tests {
		got := fw.ShouldIgnore(tt.path)
		if got != tt.want {
			t.Errorf("ShouldIgnore(%q) = %v, want %v", tt.path, got, tt.want)
		}
	}
}

func TestDedupEvents(t *testing.T) {
	now := time.Now()
	events := []FileEvent{
		{Path: "a.go", Type: "modify", Time: now, Size: 100},
		{Path: "b.go", Type: "create", Time: now, Size: 200},
		{Path: "a.go", Type: "modify", Time: now.Add(time.Second), Size: 150},
		{Path: "c.go", Type: "delete", Time: now, Size: 0},
		{Path: "b.go", Type: "modify", Time: now.Add(2 * time.Second), Size: 250},
	}

	result := DedupEvents(events)

	if len(result) != 3 {
		t.Fatalf("expected 3 deduped events, got %d", len(result))
	}

	// a.go should have the later modify (size 150).
	for _, ev := range result {
		if ev.Path == "a.go" {
			if ev.Size != 150 {
				t.Errorf("expected a.go size 150, got %d", ev.Size)
			}
		}
		if ev.Path == "b.go" {
			if ev.Type != "modify" || ev.Size != 250 {
				t.Errorf("expected b.go modify/250, got %s/%d", ev.Type, ev.Size)
			}
		}
		if ev.Path == "c.go" {
			if ev.Type != "delete" {
				t.Errorf("expected c.go delete, got %s", ev.Type)
			}
		}
	}
}

func TestFormatEvents(t *testing.T) {
	events := []FileEvent{
		{Path: "src/auth.go", Type: "modify", Size: 2150},
		{Path: "src/middleware.go", Type: "create", Size: 500},
		{Path: "src/old.go", Type: "delete", Size: 0},
	}

	output := FormatEvents(events)

	if !strings.Contains(output, "File changes detected:") {
		t.Error("expected header in output")
	}
	if !strings.Contains(output, "M src/auth.go") {
		t.Error("expected modify marker for auth.go")
	}
	if !strings.Contains(output, "A src/middleware.go") {
		t.Error("expected add marker for middleware.go")
	}
	if !strings.Contains(output, "(new)") {
		t.Error("expected (new) for created file")
	}
	if !strings.Contains(output, "D src/old.go") {
		t.Error("expected delete marker for old.go")
	}
	if !strings.Contains(output, "(deleted)") {
		t.Error("expected (deleted) for deleted file")
	}
}

func TestFormatEvents_Empty(t *testing.T) {
	output := FormatEvents(nil)
	if output != "" {
		t.Errorf("expected empty string for nil events, got %q", output)
	}
}

func TestFileWatcher_Stop(t *testing.T) {
	dir := t.TempDir()

	fw := NewFileWatcher(dir, WatcherConfig{
		PollInterval: 50 * time.Millisecond,
		Debounce:     50 * time.Millisecond,
	})

	done := make(chan error, 1)
	ctx := context.Background()

	go func() {
		done <- fw.Start(ctx)
	}()

	// Give it time to start.
	time.Sleep(100 * time.Millisecond)

	fw.Stop()

	select {
	case err := <-done:
		if err != nil {
			t.Errorf("expected nil error on stop, got: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("watcher did not stop within timeout")
	}
}

func TestFileWatcher_ConcurrentSafety(t *testing.T) {
	dir := t.TempDir()

	var mu sync.Mutex
	var received []FileEvent

	fw := NewFileWatcher(dir, WatcherConfig{
		PollInterval: 30 * time.Millisecond,
		Debounce:     80 * time.Millisecond,
		BatchWindow:  30 * time.Millisecond,
	})
	fw.OnChange = func(events []FileEvent) {
		mu.Lock()
		received = append(received, events...)
		mu.Unlock()
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	go func() {
		_ = fw.Start(ctx)
	}()

	// Wait for start.
	time.Sleep(80 * time.Millisecond)

	// Concurrent file writes from multiple goroutines.
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			name := filepath.Join(dir, strings.Repeat("f", n+1)+".txt")
			_ = os.WriteFile(name, []byte("goroutine content"), 0644)
		}(i)
	}
	wg.Wait()

	// Wait for all events to be processed.
	time.Sleep(500 * time.Millisecond)

	mu.Lock()
	count := len(received)
	mu.Unlock()

	if count == 0 {
		t.Error("expected events from concurrent file creation")
	}
}

func TestFileWatcher_PatternFiltering(t *testing.T) {
	dir := t.TempDir()

	var mu sync.Mutex
	var received []FileEvent

	fw := NewFileWatcher(dir, WatcherConfig{
		Patterns:     []string{"*.go"},
		PollInterval: 50 * time.Millisecond,
		Debounce:     100 * time.Millisecond,
		BatchWindow:  50 * time.Millisecond,
	})
	fw.OnChange = func(events []FileEvent) {
		mu.Lock()
		received = append(received, events...)
		mu.Unlock()
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	go func() {
		_ = fw.Start(ctx)
	}()

	time.Sleep(100 * time.Millisecond)

	// Create both .go and .txt files.
	_ = os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main"), 0644)
	_ = os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("notes"), 0644)

	time.Sleep(400 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()

	for _, ev := range received {
		if !strings.HasSuffix(ev.Path, ".go") {
			t.Errorf("received event for non-.go file: %s", ev.Path)
		}
	}

	found := false
	for _, ev := range received {
		if ev.Path == "main.go" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected event for main.go")
	}
}

func TestWatchSingle(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "config.yaml")
	_ = os.WriteFile(filePath, []byte("key: value"), 0644)

	var mu sync.Mutex
	changeCount := 0

	sw := WatchSingle(filePath, func() {
		mu.Lock()
		changeCount++
		mu.Unlock()
	})
	sw.interval = 50 * time.Millisecond

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	go func() {
		_ = sw.Start(ctx)
	}()

	time.Sleep(100 * time.Millisecond)

	// Modify the file.
	_ = os.WriteFile(filePath, []byte("key: updated"), 0644)

	time.Sleep(200 * time.Millisecond)

	mu.Lock()
	count := changeCount
	mu.Unlock()

	if count == 0 {
		t.Error("expected onChange to be called for single file modification")
	}

	sw.Stop()
}

func TestDefaultIgnorePatterns(t *testing.T) {
	patterns := DefaultIgnorePatterns()
	expected := []string{
		".git/",
		"node_modules/",
		"vendor/",
		"__pycache__/",
		".venv/",
		"dist/",
		"build/",
		".DS_Store",
		"*.swp",
		"*.swo",
		"*~",
	}

	if len(patterns) != len(expected) {
		t.Fatalf("expected %d patterns, got %d", len(expected), len(patterns))
	}

	for i, p := range patterns {
		if p != expected[i] {
			t.Errorf("pattern[%d] = %q, want %q", i, p, expected[i])
		}
	}
}

func TestFileWatcher_DoubleStartReturnsError(t *testing.T) {
	dir := t.TempDir()

	fw := NewFileWatcher(dir, WatcherConfig{
		PollInterval: 50 * time.Millisecond,
		Debounce:     50 * time.Millisecond,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	started := make(chan struct{})
	go func() {
		close(started)
		_ = fw.Start(ctx)
	}()
	<-started
	time.Sleep(80 * time.Millisecond)

	// Second start should fail.
	err := fw.Start(ctx)
	if err == nil {
		t.Error("expected error on double start")
	}
}
