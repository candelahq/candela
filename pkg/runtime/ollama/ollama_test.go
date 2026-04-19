package ollama

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
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("/api/tags", func(w http.ResponseWriter, _ *http.Request) {
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

func TestListModels(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/tags", func(w http.ResponseWriter, _ *http.Request) {
		// Return raw JSON matching ollama's actual response shape.
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
	if models[1].ID != "codellama:13b" {
		t.Errorf("models[1].ID = %q, want %q", models[1].ID, "codellama:13b")
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

func TestEndpoint(t *testing.T) {
	rt, _ := New(runtime.Config{Host: "192.168.1.100", Port: 11434})
	if got := rt.Endpoint(); got != "http://192.168.1.100:11434/v1" {
		t.Errorf("Endpoint() = %q, want http://192.168.1.100:11434/v1", got)
	}
}

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
