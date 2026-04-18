package vllm

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/candelahq/candela/pkg/runtime"
)

func testServer(t *testing.T, handler http.Handler) *Runtime {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	host, portStr, _ := net.SplitHostPort(srv.Listener.Addr().String())
	port, _ := strconv.Atoi(portStr)

	rt, err := New(runtime.Config{Host: host, Port: port})
	if err != nil {
		t.Fatal(err)
	}
	return rt
}

func TestHealth_Running(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/health/ready", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("/v1/models", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"data": []any{}})
	})

	rt := testServer(t, mux)

	h, err := rt.Health(context.Background())
	if err != nil {
		t.Fatalf("Health() error: %v", err)
	}
	if h.Status != runtime.StatusRunning {
		t.Errorf("Status = %q, want %q", h.Status, runtime.StatusRunning)
	}
}

func TestHealth_Starting(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/health/ready", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable) // model still loading
	})

	rt := testServer(t, mux)

	h, err := rt.Health(context.Background())
	if err != nil {
		t.Fatalf("Health() error: %v", err)
	}
	if h.Status != runtime.StatusStarting {
		t.Errorf("Status = %q, want %q", h.Status, runtime.StatusStarting)
	}
}

func TestHealth_Stopped(t *testing.T) {
	rt, _ := New(runtime.Config{Host: "127.0.0.1", Port: 19998})

	h, err := rt.Health(context.Background())
	if err != nil {
		t.Fatalf("Health() error: %v", err)
	}
	if h.Status != runtime.StatusStopped {
		t.Errorf("Status = %q, want %q", h.Status, runtime.StatusStopped)
	}
}

func TestListModels(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/models", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{
			"data": [
				{"id": "meta-llama/Llama-3.2-8B-Instruct", "owned_by": "vllm"}
			]
		}`))
	})

	rt := testServer(t, mux)

	models, err := rt.ListModels(context.Background())
	if err != nil {
		t.Fatalf("ListModels() error: %v", err)
	}
	if len(models) != 1 {
		t.Fatalf("got %d models, want 1", len(models))
	}
	if models[0].ID != "meta-llama/Llama-3.2-8B-Instruct" {
		t.Errorf("ID = %q, want %q", models[0].ID, "meta-llama/Llama-3.2-8B-Instruct")
	}
	if !models[0].Loaded {
		t.Error("vLLM models should always be marked as Loaded")
	}
}

func TestPullModel_NoOp(t *testing.T) {
	rt, _ := New(runtime.Config{})

	progress := make(chan runtime.PullProgress, 1)
	err := rt.PullModel(context.Background(), "some-model", progress)
	if err != nil {
		t.Fatalf("PullModel() error: %v", err)
	}

	p := <-progress
	if p.Status != "done" {
		t.Errorf("PullProgress.Status = %q, want %q", p.Status, "done")
	}
	if p.Percent != 100 {
		t.Errorf("PullProgress.Percent = %f, want 100", p.Percent)
	}
}
