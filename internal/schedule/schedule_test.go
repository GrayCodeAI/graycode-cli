package schedule

import (
	"sync"
	"testing"
	"time"

	"github.com/GrayCodeAI/hawk/internal/eventlog"
)

func TestFoldEvents(t *testing.T) {
	due1 := time.Now().Add(10 * time.Minute)
	due2 := time.Now().Add(20 * time.Minute)

	log := eventlog.New(nil)
	log.AppendScheduleCreate(eventlog.ScheduleCreateFact{
		ID:        "sched-1",
		Prompt:    "Check build metrics",
		DueAt:     due1,
		Recurring: false,
	})
	log.AppendScheduleCreate(eventlog.ScheduleCreateFact{
		ID:        "sched-2",
		Prompt:    "Poll cluster status",
		DueAt:     due2,
		Recurring: true,
		Interval:  "5m",
	})

	// Initial fold -> 2 active items
	active := Fold(log.Snapshot())
	if len(active) != 2 {
		t.Fatalf("expected 2 active schedules, got %d", len(active))
	}
	if active["sched-1"].Prompt != "Check build metrics" {
		t.Errorf("sched-1 prompt mismatch: %s", active["sched-1"].Prompt)
	}

	// Update sched-1 prompt
	newPrompt := "Check build metrics and memory profile"
	log.AppendScheduleUpdate(eventlog.ScheduleUpdateFact{
		ID:     "sched-1",
		Prompt: &newPrompt,
	})
	active = Fold(log.Snapshot())
	if active["sched-1"].Prompt != newPrompt {
		t.Errorf("expected updated prompt %q, got %q", newPrompt, active["sched-1"].Prompt)
	}

	// Mark sched-1 as due (one-shot) -> should be removed from active
	log.AppendScheduleDue(eventlog.ScheduleDueFact{
		ID:          "sched-1",
		DeliveredAt: time.Now(),
	})
	active = Fold(log.Snapshot())
	if _, exists := active["sched-1"]; exists {
		t.Errorf("expected one-shot sched-1 to be inactive after delivery")
	}

	// Delete sched-2
	log.AppendScheduleDelete("sched-2", "user cancelled")
	active = Fold(log.Snapshot())
	if len(active) != 0 {
		t.Fatalf("expected 0 active schedules after deletion, got %d", len(active))
	}
}

func TestManager_CreateListDelete(t *testing.T) {
	log := eventlog.New(nil)
	mgr := NewManager()
	defer mgr.Close()

	mgr.Attach(log, nil)

	item1, err := mgr.Create("Run tests", time.Now().Add(1*time.Hour), "", false)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	item2, err := mgr.Create("Check memory", time.Now().Add(2*time.Hour), "30m", true)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	items := mgr.List()
	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}
	if items[0].ID != item1.ID || items[1].ID != item2.ID {
		t.Errorf("unexpected list ordering: %#v", items)
	}

	// Delete item1
	if err := mgr.Delete(item1.ID, "done"); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	itemsAfter := mgr.List()
	if len(itemsAfter) != 1 || itemsAfter[0].ID != item2.ID {
		t.Fatalf("expected 1 remaining item %s, got %#v", item2.ID, itemsAfter)
	}
}

func TestManager_ColdSessionOverdueResume(t *testing.T) {
	log := eventlog.New(nil)
	overdueTime := time.Now().Add(-10 * time.Minute)

	// Simulate an overdue schedule created in a previous session turn
	log.AppendScheduleCreate(eventlog.ScheduleCreateFact{
		ID:        "sched-cold-1",
		Prompt:    "Resume long running experiment review",
		DueAt:     overdueTime,
		Recurring: false,
	})

	var delivered []Item
	var mu sync.Mutex

	mgr := NewManager()
	defer mgr.Close()

	// Attaching should trigger immediate catchup delivery
	mgr.Attach(log, func(item Item) error {
		mu.Lock()
		defer mu.Unlock()
		delivered = append(delivered, item)
		return nil
	})

	mu.Lock()
	defer mu.Unlock()
	if len(delivered) != 1 {
		t.Fatalf("expected 1 overdue item delivered on cold attach, got %d", len(delivered))
	}
	if delivered[0].ID != "sched-cold-1" {
		t.Errorf("expected sched-cold-1, got %s", delivered[0].ID)
	}

	// Check that schedule.due fact was recorded in log
	active := Fold(log.Snapshot())
	if len(active) != 0 {
		t.Fatalf("expected one-shot overdue schedule to be completed in log, active: %#v", active)
	}
}

func TestManager_TimerDueDelivery(t *testing.T) {
	log := eventlog.New(nil)
	var delivered []Item
	var mu sync.Mutex
	doneCh := make(chan struct{})

	mgr := NewManager()
	defer mgr.Close()

	mgr.Attach(log, func(item Item) error {
		mu.Lock()
		delivered = append(delivered, item)
		mu.Unlock()
		close(doneCh)
		return nil
	})

	// Create a short timer (30ms)
	dueAt := time.Now().Add(30 * time.Millisecond)
	item, err := mgr.Create("Ping backend health", dueAt, "", false)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	select {
	case <-doneCh:
		mu.Lock()
		defer mu.Unlock()
		if len(delivered) != 1 || delivered[0].ID != item.ID {
			t.Fatalf("unexpected delivered items: %#v", delivered)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for scheduled delivery")
	}
}
