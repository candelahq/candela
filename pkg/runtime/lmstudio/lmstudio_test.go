package lmstudio

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

func TestHealth_Stopped(t *testing.T) {
	rt, _ := New(runtime.Config{Host: "127.0.0.1", Port: 19997})

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
				{"id": "lmstudio-community/Meta-Llama-3-8B-Instruct-GGUF", "owned_by": "lmstudio"},
				{"id": "TheBloke/Mistral-7B-Instruct-v0.2-GGUF", "owned_by": "lmstudio"}
			]
		}`))
	})

	rt := testServer(t, mux)

	models, err := rt.ListModels(context.Background())
	if err != nil {
		t.Fatalf("ListModels() error: %v", err)
	}
	if len(models) != 2 {
		t.Fatalf("got %d models, want 2", len(models))
	}
	if models[0].ID != "lmstudio-community/Meta-Llama-3-8B-Instruct-GGUF" {
		t.Errorf("models[0].ID = %q", models[0].ID)
	}
	if !models[0].Loaded {
		t.Error("LM Studio listed models should be marked as Loaded")
	}
}

func TestPullModel_Success(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/models/download", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("PullModel method = %s, want POST", r.Method)
		}
		var body map[string]string
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["model"] != "test-model" {
			t.Errorf("pull body model = %q, want %q", body["model"], "test-model")
		}
		w.WriteHeader(http.StatusOK)
	})

	rt := testServer(t, mux)

	progress := make(chan runtime.PullProgress, 1)
	err := rt.PullModel(context.Background(), "test-model", progress)
	if err != nil {
		t.Fatalf("PullModel() error: %v", err)
	}

	p := <-progress
	if p.Status != "done" {
		t.Errorf("PullProgress.Status = %q, want %q", p.Status, "done")
	}
}

func TestPullModel_ServerError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/models/download", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})

	rt := testServer(t, mux)

	err := rt.PullModel(context.Background(), "bad-model", nil)
	if err == nil {
		t.Fatal("PullModel() should return error on 500")
	}
}
