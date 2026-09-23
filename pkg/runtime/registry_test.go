package runtime_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/candelahq/candela/pkg/runtime"
)

// dummyRuntime implements runtime.Runtime for testing.
type dummyRuntime struct {
	name string
}

func (d *dummyRuntime) Name() string                  { return d.name }
func (d *dummyRuntime) Endpoint() string              { return "http://localhost:1234" }
func (d *dummyRuntime) Start(_ context.Context) error { return nil }
func (d *dummyRuntime) Stop(_ context.Context) error  { return nil }
func (d *dummyRuntime) Health(_ context.Context) (*runtime.Health, error) {
	return &runtime.Health{Status: runtime.StatusRunning}, nil
}
func (d *dummyRuntime) ListModels(_ context.Context) ([]runtime.Model, error) { return nil, nil }
func (d *dummyRuntime) PullModel(_ context.Context, _ string, _ chan<- runtime.PullProgress) error {
	return nil
}
func (d *dummyRuntime) LoadModel(_ context.Context, _ string) error   { return nil }
func (d *dummyRuntime) UnloadModel(_ context.Context, _ string) error { return nil }
func (d *dummyRuntime) DeleteModel(_ context.Context, _ string) error { return nil }

func dummyFactory(name string) runtime.Factory {
	return func(cfg runtime.Config) (runtime.Runtime, error) {
		return &dummyRuntime{name: name}, nil
	}
}

func TestRegister_Success(t *testing.T) {
	const name = "test-backend-register"
	defer runtime.UnregisterForTesting(name)

	err := runtime.Register(name, dummyFactory(name))
	if err != nil {
		t.Fatalf("unexpected error registering: %v", err)
	}

	rt, err := runtime.New(name, runtime.Config{})
	if err != nil {
		t.Fatalf("unexpected error creating runtime: %v", err)
	}
	if rt.Name() != name {
		t.Errorf("got name %q, want %q", rt.Name(), name)
	}
}

func TestRegister_DuplicateReturnsError(t *testing.T) {
	const name = "test-backend-dup"
	defer runtime.UnregisterForTesting(name)

	if err := runtime.Register(name, dummyFactory(name)); err != nil {
		t.Fatalf("first registration failed: %v", err)
	}

	// Second registration must return ErrAlreadyRegistered and NOT panic
	err := runtime.Register(name, dummyFactory(name))
	if err == nil {
		t.Fatal("expected error on duplicate registration, got nil")
	}
	if !errors.Is(err, runtime.ErrAlreadyRegistered) {
		t.Errorf("expected ErrAlreadyRegistered, got %v", err)
	}
}

func TestRegister_NilFactoryReturnsError(t *testing.T) {
	const name = "test-backend-nil"
	defer runtime.UnregisterForTesting(name)

	err := runtime.Register(name, nil)
	if err == nil {
		t.Fatal("expected error registering nil factory, got nil")
	}
	if !strings.Contains(err.Error(), "factory cannot be nil") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestMustRegister_Success(t *testing.T) {
	const name = "test-backend-must"
	defer runtime.UnregisterForTesting(name)

	// MustRegister should not panic on valid first registration
	runtime.MustRegister(name, dummyFactory(name))

	rt, err := runtime.New(name, runtime.Config{})
	if err != nil {
		t.Fatalf("unexpected error creating runtime: %v", err)
	}
	if rt.Name() != name {
		t.Errorf("got name %q, want %q", rt.Name(), name)
	}
}

func TestMustRegister_DuplicatePanics(t *testing.T) {
	const name = "test-backend-must-dup"
	defer runtime.UnregisterForTesting(name)

	runtime.MustRegister(name, dummyFactory(name))

	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected MustRegister to panic on duplicate, but it did not")
		}
		panicMsg := fmt.Sprintf("%v", r)
		if !strings.Contains(panicMsg, "already registered") {
			t.Errorf("unexpected panic message: %s", panicMsg)
		}
	}()

	runtime.MustRegister(name, dummyFactory(name))
}

func TestMustRegister_NilFactoryPanics(t *testing.T) {
	const name = "test-backend-must-nil"
	defer runtime.UnregisterForTesting(name)

	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected MustRegister to panic on nil factory, but it did not")
		}
		panicMsg := fmt.Sprintf("%v", r)
		if !strings.Contains(panicMsg, "factory cannot be nil") {
			t.Errorf("unexpected panic message: %s", panicMsg)
		}
	}()

	runtime.MustRegister(name, nil)
}

func TestNew_UnknownBackend(t *testing.T) {
	rt, err := runtime.New("non-existent-backend-12345", runtime.Config{})
	if rt != nil {
		t.Errorf("expected nil runtime, got %v", rt)
	}
	if err == nil {
		t.Fatal("expected error for unknown backend, got nil")
	}
	if !strings.Contains(err.Error(), "unknown backend") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestNames_Sorted(t *testing.T) {
	const b1 = "zzz-test-names"
	const b2 = "aaa-test-names"
	defer runtime.UnregisterForTesting(b1)
	defer runtime.UnregisterForTesting(b2)

	_ = runtime.Register(b1, dummyFactory(b1))
	_ = runtime.Register(b2, dummyFactory(b2))

	names := runtime.Names()
	for i := 1; i < len(names); i++ {
		if names[i-1] > names[i] {
			t.Errorf("Names() not sorted: %s > %s", names[i-1], names[i])
		}
	}
}

func TestRegister_Concurrent(t *testing.T) {
	const prefix = "test-concurrent-"
	const count = 30
	var wg sync.WaitGroup
	wg.Add(count)

	for i := range count {
		name := fmt.Sprintf("%s%d", prefix, i)
		defer runtime.UnregisterForTesting(name)
		go func(n string) {
			defer wg.Done()
			_ = runtime.Register(n, dummyFactory(n))
		}(name)
	}

	wg.Wait()

	for i := range count {
		name := fmt.Sprintf("%s%d", prefix, i)
		rt, err := runtime.New(name, runtime.Config{})
		if err != nil || rt == nil {
			t.Errorf("failed to retrieve concurrent backend %s: %v", name, err)
		}
	}
}
