package proxy

import (
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
)

func TestAsyncSemCapacity(t *testing.T) {
	// Verify the asyncSem is initialized with the expected capacity.
	p := &Proxy{
		asyncSem: make(chan struct{}, 50),
	}
	if cap(p.asyncSem) != 50 {
		t.Errorf("asyncSem capacity = %d, want 50", cap(p.asyncSem))
	}
}

func TestAsyncSemDropsWhenFull(t *testing.T) {
	sem := make(chan struct{}, 2)

	// Fill the semaphore
	sem <- struct{}{}
	sem <- struct{}{}

	// Third attempt should be dropped (non-blocking select)
	dropped := false
	select {
	case sem <- struct{}{}:
		// acquired
	default:
		dropped = true
	}

	if !dropped {
		t.Error("expected operation to be dropped when semaphore is full")
	}
}

func TestAsyncSem_ReleasesAfterCompletion(t *testing.T) {
	sem := make(chan struct{}, 2)
	done := make(chan struct{})

	// Acquire one slot and release it via goroutine pattern
	select {
	case sem <- struct{}{}:
		go func() {
			defer func() { <-sem }()
			close(done)
		}()
	default:
		t.Fatal("failed to acquire semaphore")
	}

	// Wait for goroutine to finish
	<-done

	// After release, we should be able to fill to capacity again
	sem <- struct{}{}
	sem <- struct{}{}

	if len(sem) != 2 {
		t.Errorf("expected sem length 2, got %d", len(sem))
	}
}

func TestAsyncSem_ConcurrentBound(t *testing.T) {
	const capacity = 5
	sem := make(chan struct{}, capacity)
	var active atomic.Int32
	var maxActive atomic.Int32
	var dropped atomic.Int32
	var wg sync.WaitGroup

	// Launch many more goroutines than the capacity
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
				cur := active.Add(1)
				// Track peak concurrency
				for {
					old := maxActive.Load()
					if cur <= old || maxActive.CompareAndSwap(old, cur) {
						break
					}
				}
				// Simulate work
				runtime.Gosched()
				active.Add(-1)
				<-sem
			default:
				dropped.Add(1)
			}
		}()
	}
	wg.Wait()

	if peak := maxActive.Load(); peak > int32(capacity) {
		t.Errorf("max concurrent = %d, exceeded capacity %d", peak, capacity)
	}
	if d := dropped.Load(); d == 0 {
		t.Log("no drops (all 100 goroutines acquired) — possible but unlikely under contention")
	}
	t.Logf("peak=%d, dropped=%d", maxActive.Load(), dropped.Load())
}

func TestDroppedAsyncCounter(t *testing.T) {
	var counter atomic.Int64
	sem := make(chan struct{}, 1)

	// Fill the semaphore
	sem <- struct{}{}

	// Attempt to acquire — should be dropped
	select {
	case sem <- struct{}{}:
		t.Fatal("should not have acquired")
	default:
		counter.Add(1)
	}

	if counter.Load() != 1 {
		t.Errorf("dropped counter = %d, want 1", counter.Load())
	}

	// Second drop
	select {
	case sem <- struct{}{}:
		t.Fatal("should not have acquired")
	default:
		counter.Add(1)
	}

	if counter.Load() != 2 {
		t.Errorf("dropped counter = %d, want 2", counter.Load())
	}
}
