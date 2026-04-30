package health

import (
	"context"
	"testing"
	"time"
)

func TestRegistry(t *testing.T) {
	r := NewRegistry()

	// Register a healthy check
	r.Register("test", func(ctx context.Context) Check {
		return Check{Name: "test", Status: Healthy}
	})

	results := r.Run(context.Background())
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	if results["test"].Status != Healthy {
		t.Fatalf("expected healthy, got %s", results["test"].Status)
	}

	if r.Status() != Healthy {
		t.Fatalf("expected healthy status, got %s", r.Status())
	}
}

func TestUnhealthy(t *testing.T) {
	r := NewRegistry()

	r.Register("bad", func(ctx context.Context) Check {
		return Check{Name: "bad", Status: Unhealthy}
	})

	r.Run(context.Background())
	if r.Status() != Unhealthy {
		t.Fatalf("expected unhealthy, got %s", r.Status())
	}
}

func TestDegraded(t *testing.T) {
	r := NewRegistry()

	r.Register("ok", func(ctx context.Context) Check {
		return Check{Name: "ok", Status: Healthy}
	})
	r.Register("slow", func(ctx context.Context) Check {
		return Check{Name: "slow", Status: Degraded}
	})

	r.Run(context.Background())
	if r.Status() != Degraded {
		t.Fatalf("expected degraded, got %s", r.Status())
	}
}

func TestResult(t *testing.T) {
	r := NewRegistry()

	r.Register("test", func(ctx context.Context) Check {
		return Check{Name: "test", Status: Healthy}
	})

	r.Run(context.Background())

	check, ok := r.Result("test")
	if !ok {
		t.Fatal("expected to find result")
	}
	if check.Status != Healthy {
		t.Fatalf("expected healthy, got %s", check.Status)
	}

	_, ok = r.Result("missing")
	if ok {
		t.Fatal("expected not to find missing result")
	}
}

func TestAPIKeyChecker(t *testing.T) {
	checker := APIKeyChecker("anthropic", "sk-test")
	check := checker(context.Background())
	if check.Status != Healthy {
		t.Fatalf("expected healthy, got %s", check.Status)
	}

	checker = APIKeyChecker("anthropic", "")
	check = checker(context.Background())
	if check.Status != Unhealthy {
		t.Fatalf("expected unhealthy, got %s", check.Status)
	}
}

func TestDiskSpaceChecker(t *testing.T) {
	checker := DiskSpaceChecker(1)
	check := checker(context.Background())
	if check.Status != Healthy {
		t.Fatalf("expected healthy, got %s", check.Status)
	}
	if check.Name != "disk_space" {
		t.Fatalf("expected name 'disk_space', got %q", check.Name)
	}
}

func TestCheckTimestamp(t *testing.T) {
	r := NewRegistry()
	r.Register("test", func(ctx context.Context) Check {
		return Check{Name: "test", Status: Healthy, LastChecked: time.Now()}
	})
	results := r.Run(context.Background())

	if results["test"].LastChecked.IsZero() {
		t.Fatal("last_checked should not be zero")
	}
}
