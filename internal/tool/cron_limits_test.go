package tool

import (
	"strings"
	"testing"
)

func TestCronScheduler_CreateLimit(t *testing.T) {
	t.Parallel()

	s := &CronScheduler{jobs: make(map[string]*CronJob)}
	for i := 0; i < maxCronJobs; i++ {
		if _, err := s.Create("*/5 * * * *", "task", true, false); err != nil {
			t.Fatalf("create job %d: %v", i, err)
		}
	}

	_, err := s.Create("*/5 * * * *", "overflow", true, false)
	if err == nil || !strings.Contains(err.Error(), "cron job limit reached") {
		t.Fatalf("expected cron limit error, got %v", err)
	}
}
