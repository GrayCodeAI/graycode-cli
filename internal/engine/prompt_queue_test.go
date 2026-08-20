package engine

import (
	"sync"
	"testing"
	"time"
)

func TestPromptQueue_PriorityAndFIFO(t *testing.T) {
	pq := NewPromptQueue()

	// Enqueue in mixed order
	id1 := pq.EnqueueText("normal 1", PriorityNormal, "user")
	time.Sleep(2 * time.Millisecond)
	id2 := pq.EnqueueText("normal 2", PriorityNormal, "user")
	time.Sleep(2 * time.Millisecond)
	id3 := pq.EnqueueText("steering 1", PrioritySteering, "schedule")
	time.Sleep(2 * time.Millisecond)
	id4 := pq.EnqueueText("interjection 1", PriorityInterjection, "btw")

	if pq.Len() != 4 {
		t.Fatalf("expected length 4, got %d", pq.Len())
	}

	// Dequeue 1: should be interjection 1
	p1, ok := pq.Dequeue()
	if !ok || p1.ID != id4 || p1.Text != "interjection 1" {
		t.Errorf("first dequeue = %v, want interjection 1", p1)
	}

	// Dequeue 2: should be steering 1
	p2, ok := pq.Dequeue()
	if !ok || p2.ID != id3 || p2.Text != "steering 1" {
		t.Errorf("second dequeue = %v, want steering 1", p2)
	}

	// Dequeue 3: should be normal 1 (FIFO before normal 2)
	p3, ok := pq.Dequeue()
	if !ok || p3.ID != id1 || p3.Text != "normal 1" {
		t.Errorf("third dequeue = %v, want normal 1", p3)
	}

	// Dequeue 4: should be normal 2
	p4, ok := pq.Dequeue()
	if !ok || p4.ID != id2 || p4.Text != "normal 2" {
		t.Errorf("fourth dequeue = %v, want normal 2", p4)
	}

	// Queue should now be empty
	if !pq.IsEmpty() {
		t.Error("expected empty queue")
	}
}

func TestPromptQueue_PauseAndResume(t *testing.T) {
	pq := NewPromptQueue()
	pq.EnqueueText("task 1", PriorityNormal, "user")

	pq.Pause()
	if !pq.IsPaused() {
		t.Error("expected queue to be paused")
	}

	// Dequeue while paused should return false
	_, ok := pq.Dequeue()
	if ok {
		t.Error("expected Dequeue to fail while paused")
	}

	// Peek should still work while paused
	p, ok := pq.Peek()
	if !ok || p.Text != "task 1" {
		t.Errorf("Peek while paused failed: %v", p)
	}

	pq.Resume()
	if pq.IsPaused() {
		t.Error("expected queue to be resumed")
	}

	p, ok = pq.Dequeue()
	if !ok || p.Text != "task 1" {
		t.Errorf("Dequeue after resume failed: %v", p)
	}
}

func TestPromptQueue_DrainAndClear(t *testing.T) {
	pq := NewPromptQueue()
	pq.EnqueueText("item 1", PriorityNormal, "user")
	pq.EnqueueText("item 2", PrioritySteering, "schedule")

	items := pq.Drain()
	if len(items) != 2 {
		t.Errorf("Drain returned %d items, want 2", len(items))
	}
	if !pq.IsEmpty() {
		t.Error("expected queue to be empty after Drain")
	}

	pq.EnqueueText("item 3", PriorityNormal, "user")
	pq.Clear()
	if pq.Len() != 0 {
		t.Errorf("expected 0 items after Clear, got %d", pq.Len())
	}
}

func TestPromptQueue_Remove(t *testing.T) {
	pq := NewPromptQueue()
	id1 := pq.EnqueueText("item 1", PriorityNormal, "user")
	id2 := pq.EnqueueText("item 2", PriorityNormal, "user")

	if !pq.Remove(id1) {
		t.Error("expected Remove(id1) to succeed")
	}
	if pq.Remove("nonexistent-id") {
		t.Error("expected Remove on nonexistent ID to fail")
	}

	list := pq.List()
	if len(list) != 1 || list[0].ID != id2 {
		t.Errorf("expected list with item 2, got %v", list)
	}
}

func TestPromptQueue_Concurrent(t *testing.T) {
	pq := NewPromptQueue()
	var wg sync.WaitGroup

	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			pq.EnqueueText("prompt", PromptPriority(n%3), "test")
		}(i)
	}

	wg.Wait()
	if pq.Len() != 50 {
		t.Errorf("expected length 50 after concurrent enqueues, got %d", pq.Len())
	}

	dequeued := 0
	for {
		_, ok := pq.Dequeue()
		if !ok {
			break
		}
		dequeued++
	}

	if dequeued != 50 {
		t.Errorf("dequeued %d items, want 50", dequeued)
	}
}
