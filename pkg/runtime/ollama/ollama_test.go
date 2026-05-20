package ollama

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

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
	// Override the httpClient to use the default (no custom timeout for tests).
	rt.httpClient = &http.Client{Timeout: 5 * time.Second}
	return rt
}

// --- Name / Endpoint ---

func TestName(t *testing.T) {
	rt, _ := New(runtime.Config{})
	if got := rt.Name(); got != "ollama" {
		t.Errorf("Name() = %q, want %q", got, "ollama")
	}
}

func TestEndpoint(t *testing.T) {
	rt, _ := New(runtime.Config{Host: "192.168.1.100", Port: 11434})
	if got := rt.Endpoint(); got != "http://192.168.1.100:11434/v1" {
		t.Errorf("Endpoint() = %q, want http://192.168.1.100:11434/v1", got)
	}
}

func TestEndpoint_Defaults(t *testing.T) {
	rt, _ := New(runtime.Config{})
	want := "http://127.0.0.1:11434/v1"
	if got := rt.Endpoint(); got != want {
		t.Errorf("Endpoint() = %q, want %q", got, want)
	}
}

func TestBaseURL(t *testing.T) {
	rt, _ := New(runtime.Config{Host: "10.0.0.5", Port: 7777})
	want := "http://10.0.0.5:7777"
	if got := rt.baseURL(); got != want {
		t.Errorf("baseURL() = %q, want %q", got, want)
	}
}

// --- Health ---

func TestHealth_Running(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("/api/tags", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"models": []any{}})
	})
	mux.HandleFunc("/api/ps", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"models": []any{}})
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

func TestHealth_RunningWithModels(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("/api/tags", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"models":[{"name":"llama3:8b","size":4700000000,"details":{"family":"llama","parameter_size":"8B","quantization_level":"Q4_0"}}]}`))
	})
	mux.HandleFunc("/api/ps", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"models":[{"name":"llama3:8b"}]}`))
	})

	rt := testServer(t, mux)

	h, err := rt.Health(context.Background())
	if err != nil {
		t.Fatalf("Health() error: %v", err)
	}
	if h.Status != runtime.StatusRunning {
		t.Errorf("Status = %q, want %q", h.Status, runtime.StatusRunning)
	}
	if len(h.Models) != 1 {
		t.Fatalf("got %d models, want 1", len(h.Models))
	}
	if !h.Models[0].Loaded {
		t.Error("model reported by /api/ps should be marked Loaded")
	}
}

func TestHealth_Stopped(t *testing.T) {
	rt, _ := New(runtime.Config{Host: "127.0.0.1", Port: 19999})

	h, err := rt.Health(context.Background())
	if err != nil {
		t.Fatalf("Health() error: %v", err)
	}
	if h.Status != runtime.StatusStopped {
		t.Errorf("Status = %q, want %q", h.Status, runtime.StatusStopped)
	}
}

// --- ListModels ---

func TestListModels(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/tags", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{
			"models": [
				{
					"name": "llama3.2:8b",
					"model": "llama3.2:8b",
					"size": 4700000000,
					"details": {"family": "llama", "parameter_size": "8B", "quantization_level": "Q4_0"}
				},
				{
					"name": "codellama:13b",
					"model": "codellama:13b",
					"size": 7300000000,
					"details": {"family": "llama", "parameter_size": "13B", "quantization_level": "Q4_K_M"}
				}
			]
		}`))
	})
	mux.HandleFunc("/api/ps", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"models":[{"name":"llama3.2:8b"}]}`))
	})

	rt := testServer(t, mux)

	models, err := rt.ListModels(context.Background())
	if err != nil {
		t.Fatalf("ListModels() error: %v", err)
	}
	if len(models) != 2 {
		t.Fatalf("got %d models, want 2", len(models))
	}
	if models[0].ID != "llama3.2:8b" {
		t.Errorf("models[0].ID = %q, want %q", models[0].ID, "llama3.2:8b")
	}
	if models[0].SizeBytes != 4_700_000_000 {
		t.Errorf("models[0].SizeBytes = %d, want 4700000000", models[0].SizeBytes)
	}
	if models[0].Family != "llama" {
		t.Errorf("models[0].Family = %q, want %q", models[0].Family, "llama")
	}
	if models[0].Parameters != "8B" {
		t.Errorf("models[0].Parameters = %q, want %q", models[0].Parameters, "8B")
	}
	if !models[0].Loaded {
		t.Error("llama3.2:8b should be loaded (reported by /api/ps)")
	}
	if models[1].ID != "codellama:13b" {
		t.Errorf("models[1].ID = %q, want %q", models[1].ID, "codellama:13b")
	}
	if models[1].Loaded {
		t.Error("codellama:13b should NOT be loaded (not in /api/ps)")
	}
}

func TestListModels_ServerDown(t *testing.T) {
	rt, _ := New(runtime.Config{Host: "127.0.0.1", Port: 19999})

	_, err := rt.ListModels(context.Background())
	if err == nil {
		t.Fatal("ListModels() should return error when server is down")
	}
}

func TestListModels_BadJSON(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/tags", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("not json"))
	})

	rt := testServer(t, mux)

	_, err := rt.ListModels(context.Background())
	if err == nil {
		t.Fatal("ListModels() should return error for invalid JSON")
	}
}

func TestListModels_EmptyModels(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/tags", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"models":[]}`))
	})
	mux.HandleFunc("/api/ps", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"models":[]}`))
	})

	rt := testServer(t, mux)

	models, err := rt.ListModels(context.Background())
	if err != nil {
		t.Fatalf("ListModels() error: %v", err)
	}
	if len(models) != 0 {
		t.Errorf("got %d models, want 0", len(models))
	}
}

// --- runningModels ---

func TestRunningModels_ServerDown(t *testing.T) {
	rt, _ := New(runtime.Config{Host: "127.0.0.1", Port: 19999})

	m := rt.runningModels(context.Background())
	if m != nil {
		t.Errorf("runningModels() should return nil when server is down, got %v", m)
	}
}

func TestRunningModels_BadJSON(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/ps", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("not json"))
	})

	rt := testServer(t, mux)

	m := rt.runningModels(context.Background())
	if m != nil {
		t.Errorf("runningModels() should return nil for invalid JSON, got %v", m)
	}
}

func TestRunningModels_MultipleModels(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/ps", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"models":[{"name":"model-a"},{"name":"model-b"}]}`))
	})

	rt := testServer(t, mux)

	m := rt.runningModels(context.Background())
	if len(m) != 2 {
		t.Fatalf("runningModels() returned %d, want 2", len(m))
	}
	if !m["model-a"] || !m["model-b"] {
		t.Errorf("expected both model-a and model-b to be present, got %v", m)
	}
}

// --- LoadModel / UnloadModel ---

func TestLoadModel(t *testing.T) {
	var receivedModel string
	var receivedKeepAlive float64

	mux := http.NewServeMux()
	mux.HandleFunc("/api/generate", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode error: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		receivedModel, _ = body["model"].(string)
		receivedKeepAlive, _ = body["keep_alive"].(float64)
		w.WriteHeader(http.StatusOK)
	})

	rt := testServer(t, mux)

	if err := rt.LoadModel(context.Background(), "llama3.2:8b"); err != nil {
		t.Fatalf("LoadModel() error: %v", err)
	}
	if receivedModel != "llama3.2:8b" {
		t.Errorf("model = %q, want %q", receivedModel, "llama3.2:8b")
	}
	if receivedKeepAlive != -1 {
		t.Errorf("keep_alive = %v, want -1", receivedKeepAlive)
	}
}

func TestUnloadModel(t *testing.T) {
	var receivedModel string
	var receivedKeepAlive float64

	mux := http.NewServeMux()
	mux.HandleFunc("/api/generate", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode error: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		receivedModel, _ = body["model"].(string)
		receivedKeepAlive, _ = body["keep_alive"].(float64)
		w.WriteHeader(http.StatusOK)
	})

	rt := testServer(t, mux)

	if err := rt.UnloadModel(context.Background(), "llama3.2:8b"); err != nil {
		t.Fatalf("UnloadModel() error: %v", err)
	}
	if receivedModel != "llama3.2:8b" {
		t.Errorf("model = %q, want %q", receivedModel, "llama3.2:8b")
	}
	if receivedKeepAlive != 0 {
		t.Errorf("keep_alive = %v, want 0", receivedKeepAlive)
	}
}

func TestLoadModel_ServerDown(t *testing.T) {
	rt, _ := New(runtime.Config{Host: "127.0.0.1", Port: 19999})

	err := rt.LoadModel(context.Background(), "llama3.2:8b")
	if err == nil {
		t.Fatal("LoadModel() should return error when server is down")
	}
}

func TestUnloadModel_ServerDown(t *testing.T) {
	rt, _ := New(runtime.Config{Host: "127.0.0.1", Port: 19999})

	err := rt.UnloadModel(context.Background(), "llama3.2:8b")
	if err == nil {
		t.Fatal("UnloadModel() should return error when server is down")
	}
}

func TestLoadModel_ServerError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/generate", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})

	rt := testServer(t, mux)

	err := rt.LoadModel(context.Background(), "llama3.2:8b")
	if err == nil {
		t.Fatal("LoadModel() should return error on 500")
	}
}

func TestUnloadModel_ServerError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/generate", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})

	rt := testServer(t, mux)

	err := rt.UnloadModel(context.Background(), "llama3.2:8b")
	if err == nil {
		t.Fatal("UnloadModel() should return error on 500")
	}
}

// --- DeleteModel ---

func TestDeleteModel(t *testing.T) {
	var receivedMethod, receivedModel string

	mux := http.NewServeMux()
	mux.HandleFunc("/api/delete", func(w http.ResponseWriter, r *http.Request) {
		receivedMethod = r.Method
		var body struct {
			Name string `json:"name"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		receivedModel = body.Name
		w.WriteHeader(http.StatusOK)
	})

	rt := testServer(t, mux)

	if err := rt.DeleteModel(context.Background(), "llama3.2:8b"); err != nil {
		t.Fatalf("DeleteModel() error: %v", err)
	}
	if receivedMethod != http.MethodDelete {
		t.Errorf("method = %q, want DELETE", receivedMethod)
	}
	if receivedModel != "llama3.2:8b" {
		t.Errorf("model = %q, want %q", receivedModel, "llama3.2:8b")
	}
}

func TestDeleteModel_NotFound(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/delete", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"model 'nonexistent' not found"}`))
	})

	rt := testServer(t, mux)

	err := rt.DeleteModel(context.Background(), "nonexistent")
	if err == nil {
		t.Fatal("DeleteModel() should return error for 404")
	}
	if !strings.Contains(err.Error(), "nonexistent") {
		t.Errorf("error should contain model name, got: %v", err)
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error should contain response body, got: %v", err)
	}
}

func TestDeleteModel_ServerDown(t *testing.T) {
	rt, _ := New(runtime.Config{Host: "127.0.0.1", Port: 19999})

	err := rt.DeleteModel(context.Background(), "llama3.2:8b")
	if err == nil {
		t.Fatal("DeleteModel() should return error when server is down")
	}
}

// --- PullModel ---

func TestPullModel_StreamingProgress(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/pull", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("PullModel method = %s, want POST", r.Method)
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["name"] != "test-model" {
			t.Errorf("pull model = %q, want %q", body["name"], "test-model")
		}
		w.WriteHeader(http.StatusOK)
		// Stream NDJSON progress lines.
		lines := []string{
			`{"status":"pulling","completed":50,"total":100}`,
			`{"status":"downloading","completed":100,"total":100}`,
		}
		for _, line := range lines {
			_, _ = fmt.Fprintln(w, line)
		}
	})

	rt := testServer(t, mux)

	progress := make(chan runtime.PullProgress, 10)
	err := rt.PullModel(context.Background(), "test-model", progress)
	if err != nil {
		t.Fatalf("PullModel() error: %v", err)
	}

	// Collect progress updates.
	var updates []runtime.PullProgress
	close(progress)
	for p := range progress {
		updates = append(updates, p)
	}

	if len(updates) < 1 {
		t.Fatalf("expected at least 1 progress update, got %d", len(updates))
	}
}

func TestPullModel_ServerError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/pull", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("internal error"))
	})

	rt := testServer(t, mux)

	err := rt.PullModel(context.Background(), "bad-model", nil)
	if err == nil {
		t.Fatal("PullModel() should return error on 500")
	}
}

func TestPullModel_ServerDown(t *testing.T) {
	rt, _ := New(runtime.Config{Host: "127.0.0.1", Port: 19999})

	err := rt.PullModel(context.Background(), "some-model", nil)
	if err == nil {
		t.Fatal("PullModel() should return error when server is down")
	}
}

func TestPullModel_NilProgress(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/pull", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprintln(w, `{"status":"done","completed":100,"total":100}`)
	})

	rt := testServer(t, mux)

	err := rt.PullModel(context.Background(), "test-model", nil)
	if err != nil {
		t.Fatalf("PullModel() with nil progress should not error: %v", err)
	}
}

func TestPullModel_ProgressWithZeroTotal(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/pull", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprintln(w, `{"status":"pulling","completed":0,"total":0}`)
	})

	rt := testServer(t, mux)

	progress := make(chan runtime.PullProgress, 10)
	err := rt.PullModel(context.Background(), "test-model", progress)
	if err != nil {
		t.Fatalf("PullModel() error: %v", err)
	}

	p := <-progress
	if p.Percent != 0 {
		t.Errorf("expected 0%% for zero total, got %f", p.Percent)
	}
}

// --- Stop ---

func TestStop_NilCmd(t *testing.T) {
	rt, _ := New(runtime.Config{})
	err := rt.Stop(context.Background())
	if err != nil {
		t.Fatalf("Stop() should not error when cmd is nil: %v", err)
	}
}
