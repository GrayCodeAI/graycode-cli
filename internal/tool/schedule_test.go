package tool

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/GrayCodeAI/graycode-cli/internal/eventlog"
	"github.com/GrayCodeAI/graycode-cli/internal/schedule"
)

func TestScheduleTools_RoundTrip(t *testing.T) {
	log := eventlog.New(nil)
	mgr := schedule.NewManager()
	defer mgr.Close()
	mgr.Attach(log, nil)

	createTool := ScheduleCreateTool{Manager: mgr}
	listTool := ScheduleListTool{Manager: mgr}
	deleteTool := ScheduleDeleteTool{Manager: mgr}
	ctx := context.Background()

	// 1. Initially empty list
	outListEmpty, err := listTool.Execute(ctx, nil)
	if err != nil {
		t.Fatalf("ListTool failed: %v", err)
	}
	if !strings.Contains(outListEmpty, "No active") {
		t.Fatalf("expected 'No active', got %s", outListEmpty)
	}

	// 2. Create a 1h reminder
	inCreate, _ := json.Marshal(map[string]interface{}{
		"prompt":   "Review memory leak report",
		"duration": "1h",
	})
	outCreate, err := createTool.Execute(ctx, inCreate)
	if err != nil {
		t.Fatalf("CreateTool failed: %v", err)
	}
	if !strings.Contains(outCreate, "Scheduled in-conversation reminder") {
		t.Fatalf("unexpected create output: %s", outCreate)
	}

	// 3. List active schedules
	outList, err := listTool.Execute(ctx, nil)
	if err != nil {
		t.Fatalf("ListTool failed: %v", err)
	}
	if !strings.Contains(outList, "Review memory leak report") {
		t.Fatalf("expected prompt in list output: %s", outList)
	}

	// Extract schedule ID
	items := mgr.List()
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	schedID := items[0].ID

	// 4. Delete the schedule
	inDelete, _ := json.Marshal(map[string]interface{}{
		"id":     schedID,
		"reason": "completed early",
	})
	outDelete, err := deleteTool.Execute(ctx, inDelete)
	if err != nil {
		t.Fatalf("DeleteTool failed: %v", err)
	}
	if !strings.Contains(outDelete, "Cancelled scheduled reminder") {
		t.Fatalf("unexpected delete output: %s", outDelete)
	}

	// 5. Verify list is empty again
	itemsAfter := mgr.List()
	if len(itemsAfter) != 0 {
		t.Fatalf("expected 0 items after deletion, got %d", len(itemsAfter))
	}
}

func TestScheduleCreateTool_Validation(t *testing.T) {
	createTool := ScheduleCreateTool{}
	ctx := context.Background()

	// Empty prompt
	inEmpty, _ := json.Marshal(map[string]interface{}{
		"prompt": "",
	})
	_, err := createTool.Execute(ctx, inEmpty)
	if err == nil {
		t.Fatal("expected error for empty prompt, got nil")
	}

	// Invalid timestamp
	inBadAt, _ := json.Marshal(map[string]interface{}{
		"prompt": "Test",
		"at":     "not-a-timestamp",
	})
	_, err = createTool.Execute(ctx, inBadAt)
	if err == nil {
		t.Fatal("expected error for invalid 'at' timestamp, got nil")
	}

	// Valid absolute RFC3339 timestamp
	validAt := time.Now().Add(2 * time.Hour).Format(time.RFC3339)
	inGoodAt, _ := json.Marshal(map[string]interface{}{
		"prompt": "Test RFC3339",
		"at":     validAt,
	})
	out, err := createTool.Execute(ctx, inGoodAt)
	if err != nil {
		t.Fatalf("unexpected error for valid timestamp: %v", err)
	}
	if !strings.Contains(out, "Scheduled in-conversation reminder") {
		t.Fatalf("expected success, got %s", out)
	}
}
