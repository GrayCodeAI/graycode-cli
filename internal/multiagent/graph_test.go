package mission

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestMission_RunWaves_JoinsDeterministically(t *testing.T) {
	m := New("test", Config{MaxWorkers: 3})
	m.Features = []Feature{
		{ID: "f1", Description: "A", Status: FeaturePending},
		{ID: "f2", Description: "B", Status: FeaturePending},
		{ID: "f3", Description: "C", Status: FeaturePending},
	}

	var completedFirstWave atomic.Int32
	var mu sync.Mutex
	started := make([]string, 0, 3)
	workerFn := func(_ context.Context, feat *Feature, _ string, _ Config) (*Handoff, error) {
		mu.Lock()
		started = append(started, feat.ID)
		mu.Unlock()
		if feat.ID == "f3" && completedFirstWave.Load() != 2 {
			t.Fatalf("second wave started before first wave joined: completed=%d", completedFirstWave.Load())
		}
		switch feat.ID {
		case "f1":
			time.Sleep(15 * time.Millisecond)
			completedFirstWave.Add(1)
		case "f2":
			time.Sleep(5 * time.Millisecond)
			completedFirstWave.Add(1)
		}
		return &Handoff{Summary: "done " + feat.ID, TestsPassed: true}, nil
	}

	if err := m.RunWaves(context.Background(), [][]string{{"f1", "f2"}, {"f3"}}, workerFn); err != nil {
		t.Fatalf("RunWaves failed: %v", err)
	}
	if len(m.WaveJoins) != 2 {
		t.Fatalf("wave joins = %d, want 2", len(m.WaveJoins))
	}
	if got := m.WaveJoins[0].FeatureIDs; len(got) != 2 || got[0] != "f1" || got[1] != "f2" {
		t.Fatalf("first join feature order = %#v", got)
	}
	if m.Status != StatusCompleted {
		t.Fatalf("status = %s, want %s", m.Status, StatusCompleted)
	}
	for _, feat := range m.Features {
		if feat.Status != FeatureCompleted {
			t.Fatalf("feature %s status = %s, want completed", feat.ID, feat.Status)
		}
	}
}

func TestMission_RunWaves_StopsAfterFailure(t *testing.T) {
	m := New("test", Config{MaxWorkers: 2})
	m.Features = []Feature{
		{ID: "f1", Description: "A", Status: FeaturePending},
		{ID: "f2", Description: "B", Status: FeaturePending},
	}

	var calls atomic.Int32
	workerFn := func(_ context.Context, feat *Feature, _ string, _ Config) (*Handoff, error) {
		calls.Add(1)
		if feat.ID == "f2" {
			t.Fatal("downstream wave executed despite upstream failure")
		}
		if feat.ID == "f1" {
			return nil, context.DeadlineExceeded
		}
		return &Handoff{Summary: "done " + feat.ID, TestsPassed: true}, nil
	}

	if err := m.RunWaves(context.Background(), [][]string{{"f1"}, {"f2"}}, workerFn); err != nil {
		t.Fatalf("RunWaves failed: %v", err)
	}
	if calls.Load() != 3 {
		t.Fatalf("worker calls = %d, want 3 retries on first wave only", calls.Load())
	}
	if len(m.WaveJoins) != 1 {
		t.Fatalf("wave joins = %d, want 1", len(m.WaveJoins))
	}
	if got := m.WaveJoins[0].BlockedIDs; len(got) != 1 || got[0] != "f2" {
		t.Fatalf("blocked ids = %#v, want [f2]", got)
	}
	if m.Features[1].Status != FeaturePending {
		t.Fatalf("downstream feature status = %s, want pending", m.Features[1].Status)
	}
	if m.Status != StatusFailed {
		t.Fatalf("status = %s, want %s", m.Status, StatusFailed)
	}
}
