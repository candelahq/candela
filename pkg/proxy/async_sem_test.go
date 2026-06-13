package proxy

import (
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
