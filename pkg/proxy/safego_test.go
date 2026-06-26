package proxy

import (
	"sync"
	"testing"
)

func TestSafeGo_NormalExecution(t *testing.T) {
	var wg sync.WaitGroup
	wg.Add(1)
	var ran bool
	safeGo(func() {
		defer wg.Done()
		ran = true
	})
	wg.Wait()
	if !ran {
		t.Fatal("safeGo did not execute the function")
	}
}

func TestSafeGo_RecoversPanic(t *testing.T) {
	var wg sync.WaitGroup
	wg.Add(1)
	// This must not crash the test process.
	safeGo(func() {
		defer wg.Done()
		panic("test panic")
	})
	wg.Wait()
	// If we reach here, the panic was recovered.
}
