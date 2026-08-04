package engine

import (
	"context"
	"sync"
	"testing"
	"time"
)

type testContainerExecutor struct{}

func (testContainerExecutor) Exec(context.Context, string, time.Duration) (string, error) {
	return "", nil
}

func (testContainerExecutor) Running() bool { return true }

func TestToolServiceContainerStateIsSafeDuringAsyncRetry(t *testing.T) {
	service := NewToolService(nil)
	executor := testContainerExecutor{}

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			for j := 0; j < 1000; j++ {
				if worker%2 == 0 {
					service.SetContainerRequired(j%2 == 0)
				} else {
					service.SetContainerExecutor(executor)
				}
				_ = service.ContainerRequired()
				_ = service.ContainerExecutor()
			}
		}(i)
	}
	wg.Wait()

	if service.ContainerExecutor() == nil {
		t.Fatal("container executor should remain configured after concurrent updates")
	}
	if service.ContainerExecutor() != executor {
		t.Fatal("configured executor should be the executor supplied by the retry path")
	}
}
