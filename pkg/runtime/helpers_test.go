package runtime

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestWaitHealthy_ImmediateSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	err := WaitHealthy(context.Background(), srv.URL, 50*time.Millisecond, 2*time.Second)
	if err != nil {
		t.Fatalf("WaitHealthy() error: %v", err)
	}
}

func TestWaitHealthy_Timeout(t *testing.T) {
	// Point at a port nothing is listening on.
	err := WaitHealthy(context.Background(), "http://127.0.0.1:19111", 50*time.Millisecond, 200*time.Millisecond)
	if err == nil {
		t.Fatal("WaitHealthy() should return error on timeout")
	}
}

func TestWaitHealthy_ContextCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	err := WaitHealthy(ctx, "http://127.0.0.1:19111", 50*time.Millisecond, 5*time.Second)
	if err == nil {
		t.Fatal("WaitHealthy() should return error when context is canceled")
	}
}

func TestDefaultHost(t *testing.T) {
	if got := DefaultHost(""); got != "127.0.0.1" {
		t.Errorf("DefaultHost('') = %q, want 127.0.0.1", got)
	}
	if got := DefaultHost("10.0.0.1"); got != "10.0.0.1" {
		t.Errorf("DefaultHost('10.0.0.1') = %q, want 10.0.0.1", got)
	}
}

func TestDefaultPort(t *testing.T) {
	if got := DefaultPort(0, 11434); got != 11434 {
		t.Errorf("DefaultPort(0, 11434) = %d, want 11434", got)
	}
	if got := DefaultPort(8080, 11434); got != 8080 {
		t.Errorf("DefaultPort(8080, 11434) = %d, want 8080", got)
	}
}

func TestConfigBinary(t *testing.T) {
	if got := ConfigBinary(nil, "ollama"); got != "ollama" {
		t.Errorf("ConfigBinary(nil) = %q, want ollama", got)
	}
	if got := ConfigBinary(map[string]any{}, "ollama"); got != "ollama" {
		t.Errorf("ConfigBinary({}) = %q, want ollama", got)
	}
	if got := ConfigBinary(map[string]any{"binary": "/usr/local/bin/ollama"}, "ollama"); got != "/usr/local/bin/ollama" {
		t.Errorf("ConfigBinary with override = %q", got)
	}
	if got := ConfigBinary(map[string]any{"binary": ""}, "ollama"); got != "ollama" {
		t.Errorf("ConfigBinary with empty string = %q, want fallback", got)
	}
}

func TestConfigString(t *testing.T) {
	tests := []struct {
		name   string
		args   map[string]any
		key    string
		want   string
		wantOK bool
	}{
		{"string value", map[string]any{"k": "0.9"}, "k", "0.9", true},
		{"int value", map[string]any{"k": 4096}, "k", "4096", true},
		{"float64 value", map[string]any{"k": 0.9}, "k", "0.9", true},
		{"missing key", map[string]any{}, "k", "", false},
		{"nil args", nil, "k", "", false},
		{"empty string", map[string]any{"k": ""}, "k", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := ConfigString(tt.args, tt.key)
			if got != tt.want || ok != tt.wantOK {
				t.Errorf("ConfigString(%v, %q) = (%q, %v), want (%q, %v)",
					tt.args, tt.key, got, ok, tt.want, tt.wantOK)
			}
		})
	}
}
