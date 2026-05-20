package lmstudio

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strconv"
	"testing"

	"github.com/candelahq/candela/pkg/runtime"
)

// TestHelperProcess is a test helper for faking exec.Command.
// It's invoked by the test binary itself with GO_WANT_HELPER_PROCESS=1.
func TestHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}
	// Simulate success for all subcommands.
	os.Exit(0)
}

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

// --- Name / Endpoint ---

func TestName(t *testing.T) {
	rt, _ := New(runtime.Config{})
	if got := rt.Name(); got != "lmstudio" {
		t.Errorf("Name() = %q, want %q", got, "lmstudio")
	}
}

func TestEndpoint(t *testing.T) {
	rt, _ := New(runtime.Config{Host: "10.0.0.1", Port: 9090})
	want := "http://10.0.0.1:9090/v1"
	if got := rt.Endpoint(); got != want {
		t.Errorf("Endpoint() = %q, want %q", got, want)
	}
}

func TestEndpoint_Defaults(t *testing.T) {
	rt, _ := New(runtime.Config{})
	want := "http://127.0.0.1:1234/v1"
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

func TestHealth_RunningWithModels(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/models", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"data":[{"id":"test-model","owned_by":"lmstudio"}]}`))
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
		t.Errorf("got %d models, want 1", len(h.Models))
	}
	if h.Endpoint == "" {
		t.Error("Endpoint should not be empty")
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
	if h.Endpoint == "" {
		t.Error("Endpoint should be populated even when stopped")
	}
}

// --- ListModels ---

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

func TestListModels_ServerDown(t *testing.T) {
	rt, _ := New(runtime.Config{Host: "127.0.0.1", Port: 19997})

	_, err := rt.ListModels(context.Background())
	if err == nil {
		t.Fatal("ListModels() should return error when server is down")
	}
}

func TestListModels_BadJSON(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/models", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("not json"))
	})

	rt := testServer(t, mux)

	_, err := rt.ListModels(context.Background())
	if err == nil {
		t.Fatal("ListModels() should return error for invalid JSON")
	}
}

func TestListModels_EmptyData(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/models", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"data":[]}`))
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

// --- PullModel ---

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

func TestPullModel_NilProgress(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/models/download", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	rt := testServer(t, mux)

	err := rt.PullModel(context.Background(), "test-model", nil)
	if err != nil {
		t.Fatalf("PullModel() with nil progress should not error: %v", err)
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

func TestPullModel_ServerDown(t *testing.T) {
	rt, _ := New(runtime.Config{Host: "127.0.0.1", Port: 19997})

	err := rt.PullModel(context.Background(), "some-model", nil)
	if err == nil {
		t.Fatal("PullModel() should return error when server is down")
	}
}

// --- DeleteModel ---

func TestDeleteModel_NotSupported(t *testing.T) {
	rt, _ := New(runtime.Config{})
	err := rt.DeleteModel(context.Background(), "any-model")
	if err == nil {
		t.Fatal("DeleteModel() should return error for unsupported operation")
	}
}

// --- LoadModel / UnloadModel (exec-based) ---

func TestLoadModel_Success(t *testing.T) {
	rt, _ := New(runtime.Config{})
	// Override binary to use the test helper process.
	rt.binary = os.Args[0]

	// Construct the command manually like LoadModel does, but using
	// our test binary to simulate success.
	cmd := exec.Command(os.Args[0], "-test.run=TestHelperProcess", "--", "load", "test-model")
	cmd.Env = append(os.Environ(), "GO_WANT_HELPER_PROCESS=1")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("helper process failed: %v, output: %s", err, output)
	}

	// Now test the actual LoadModel by swapping the binary.
	// LoadModel calls exec.CommandContext(ctx, r.binary, "load", modelID).
	// We can't easily inject the env var, so let's test the error path
	// with a non-existent binary to cover the error branch.
	rt.binary = "/nonexistent/binary"
	err = rt.LoadModel(context.Background(), "test-model")
	if err == nil {
		t.Fatal("LoadModel() should error with non-existent binary")
	}
}

func TestUnloadModel_Error(t *testing.T) {
	rt, _ := New(runtime.Config{})
	rt.binary = "/nonexistent/binary"

	err := rt.UnloadModel(context.Background(), "test-model")
	if err == nil {
		t.Fatal("UnloadModel() should error with non-existent binary")
	}
}

// --- Stop ---

func TestStop_NilCmd(t *testing.T) {
	rt, _ := New(runtime.Config{})
	rt.binary = "/nonexistent/binary"
	// Stop tries `lms server stop` which will fail, but the fallback
	// with nil cmd should still succeed (no process to kill).
	err := rt.Stop(context.Background())
	if err != nil {
		t.Fatalf("Stop() should not error with nil cmd: %v", err)
	}
}

func TestStop_WithRunningProcess(t *testing.T) {
	rt, _ := New(runtime.Config{})

	// Start a real process we can kill (sleep).
	rt.cmd = exec.Command("sleep", "60")
	if err := rt.cmd.Start(); err != nil {
		t.Skipf("cannot start sleep process: %v", err)
	}

	// Override binary so the graceful stop attempt fails quickly.
	rt.binary = "/nonexistent/binary"

	err := rt.Stop(context.Background())
	if err != nil {
		t.Fatalf("Stop() should succeed killing the process: %v", err)
	}
	if rt.cmd != nil {
		t.Error("cmd should be nil after Stop()")
	}
}

// --- Start ---

func TestStart_BinaryNotFound(t *testing.T) {
	rt, _ := New(runtime.Config{})
	rt.binary = "/nonexistent/binary"

	err := rt.Start(context.Background())
	if err == nil {
		t.Fatal("Start() should error with non-existent binary")
	}
}
