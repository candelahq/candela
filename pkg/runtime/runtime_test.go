package runtime_test

import (
	"testing"

	"github.com/candelahq/candela/pkg/runtime"

	// Import implementations to trigger init() registration.
	_ "github.com/candelahq/candela/pkg/runtime/lmstudio"
	_ "github.com/candelahq/candela/pkg/runtime/ollama"
	_ "github.com/candelahq/candela/pkg/runtime/vllm"
)

func TestRegistryNames(t *testing.T) {
	names := runtime.Names()
	if len(names) != 3 {
		t.Fatalf("expected 3 registered runtimes, got %d: %v", len(names), names)
	}

	want := []string{"lmstudio", "ollama", "vllm"}
	for i, name := range names {
		if name != want[i] {
			t.Errorf("names[%d] = %q, want %q", i, name, want[i])
		}
	}
}

func TestNewOllama(t *testing.T) {
	rt, err := runtime.New("ollama", runtime.Config{
		Host: "127.0.0.1",
		Port: 11434,
	})
	if err != nil {
		t.Fatalf("New(ollama) error: %v", err)
	}
	if rt.Name() != "ollama" {
		t.Errorf("Name() = %q, want %q", rt.Name(), "ollama")
	}
	if got := rt.Endpoint(); got != "http://127.0.0.1:11434/v1" {
		t.Errorf("Endpoint() = %q, want http://127.0.0.1:11434/v1", got)
	}
}

func TestNewVLLM(t *testing.T) {
	rt, err := runtime.New("vllm", runtime.Config{
		Host: "127.0.0.1",
		Port: 8000,
		Args: map[string]any{"model": "meta-llama/Llama-3.2-8B-Instruct"},
	})
	if err != nil {
		t.Fatalf("New(vllm) error: %v", err)
	}
	if rt.Name() != "vllm" {
		t.Errorf("Name() = %q, want %q", rt.Name(), "vllm")
	}
	if got := rt.Endpoint(); got != "http://127.0.0.1:8000/v1" {
		t.Errorf("Endpoint() = %q, want http://127.0.0.1:8000/v1", got)
	}
}

func TestNewLMStudio(t *testing.T) {
	rt, err := runtime.New("lmstudio", runtime.Config{
		Host: "127.0.0.1",
		Port: 1234,
	})
	if err != nil {
		t.Fatalf("New(lmstudio) error: %v", err)
	}
	if rt.Name() != "lmstudio" {
		t.Errorf("Name() = %q, want %q", rt.Name(), "lmstudio")
	}
	if got := rt.Endpoint(); got != "http://127.0.0.1:1234/v1" {
		t.Errorf("Endpoint() = %q, want http://127.0.0.1:1234/v1", got)
	}
}

func TestNewUnknown(t *testing.T) {
	_, err := runtime.New("doesnotexist", runtime.Config{})
	if err == nil {
		t.Fatal("New(doesnotexist) should return error")
	}
}

func TestNewDefaults(t *testing.T) {
	// Verify sensible defaults when no host/port specified.
	rt, err := runtime.New("ollama", runtime.Config{})
	if err != nil {
		t.Fatalf("New(ollama) with defaults error: %v", err)
	}
	if got := rt.Endpoint(); got != "http://127.0.0.1:11434/v1" {
		t.Errorf("Endpoint() with defaults = %q, want http://127.0.0.1:11434/v1", got)
	}

	rt, err = runtime.New("vllm", runtime.Config{})
	if err != nil {
		t.Fatalf("New(vllm) with defaults error: %v", err)
	}
	if got := rt.Endpoint(); got != "http://127.0.0.1:8000/v1" {
		t.Errorf("Endpoint() with defaults = %q, want http://127.0.0.1:8000/v1", got)
	}

	rt, err = runtime.New("lmstudio", runtime.Config{})
	if err != nil {
		t.Fatalf("New(lmstudio) with defaults error: %v", err)
	}
	if got := rt.Endpoint(); got != "http://127.0.0.1:1234/v1" {
		t.Errorf("Endpoint() with defaults = %q, want http://127.0.0.1:1234/v1", got)
	}
}
