package tetragonaudit

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"go.opentelemetry.io/collector/pdata/plog"
)

func TestOTelSink_EmitContextCancelled(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		// Slow server — should not be reached.
		time.Sleep(5 * time.Second)
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	sink, err := NewOTelSink(OTelSinkConfig{Endpoint: ts.URL, TimeoutSec: 5})
	if err != nil {
		t.Fatalf("NewOTelSink: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately.

	err = sink.Emit(ctx, AuditRecord{Severity: "INFO"})
	if err == nil {
		t.Error("expected error for cancelled context")
	}
}

func TestOTelSink_DefaultTimeout(t *testing.T) {
	sink, err := NewOTelSink(OTelSinkConfig{Endpoint: "http://localhost:1"})
	if err != nil {
		t.Fatalf("NewOTelSink: %v", err)
	}
	if sink.client.Timeout != 10*time.Second {
		t.Errorf("expected default timeout 10s, got %v", sink.client.Timeout)
	}

	// Explicit timeout.
	sink2, err := NewOTelSink(OTelSinkConfig{Endpoint: "http://localhost:1", TimeoutSec: 30})
	if err != nil {
		t.Fatalf("NewOTelSink: %v", err)
	}
	if sink2.client.Timeout != 30*time.Second {
		t.Errorf("expected 30s timeout, got %v", sink2.client.Timeout)
	}
}

func TestOTelSink_ConcurrentEmit(t *testing.T) {
	var count atomic.Int64

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		count.Add(1)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("{}"))
	}))
	defer ts.Close()

	sink, err := NewOTelSink(OTelSinkConfig{Endpoint: ts.URL})
	if err != nil {
		t.Fatalf("NewOTelSink: %v", err)
	}

	const goroutines = 10
	var wg sync.WaitGroup
	wg.Add(goroutines)
	errs := make(chan error, goroutines)

	for i := range goroutines {
		go func(i int) {
			defer wg.Done()
			record := AuditRecord{
				Severity:   "INFO",
				Binary:     fmt.Sprintf("/bin/test-%d", i),
				DstAddr:    "1.2.3.4",
				DstPort:    443,
				PolicyName: "test-policy",
			}
			if err := sink.Emit(context.Background(), record); err != nil {
				errs <- fmt.Errorf("goroutine %d: %w", i, err)
			}
		}(i)
	}

	wg.Wait()
	close(errs)

	for err := range errs {
		t.Error(err)
	}

	if got := count.Load(); got != goroutines {
		t.Errorf("server received %d requests, want %d", got, goroutines)
	}
}

func TestBuildLogPayload_IntAttributes(t *testing.T) {
	record := AuditRecord{
		Severity: "INFO",
		DstPort:  8080,
		SrcPort:  54321,
		UID:      1000,
		DstAddr:  "10.0.0.1",
		SrcAddr:  "10.0.0.2",
	}
	logs := BuildLogPayload(record)
	attrs := logs.ResourceLogs().At(0).ScopeLogs().At(0).LogRecords().At(0).Attributes()

	assertIntAttr := func(key string, want int64) {
		t.Helper()
		v, ok := attrs.Get(key)
		if !ok {
			t.Errorf("attribute %q not found", key)
			return
		}
		if v.Int() != want {
			t.Errorf("attribute %q = %d, want %d", key, v.Int(), want)
		}
	}

	assertIntAttr("net.dst.port", 8080)
	assertIntAttr("net.src.port", 54321)
	assertIntAttr("process.uid", 1000)
}

func TestBuildLogPayload_BodyFormat(t *testing.T) {
	record := AuditRecord{
		Severity:     "CRITICAL",
		Action:       "SIGKILL",
		Binary:       "/usr/bin/curl",
		SrcAddr:      "10.0.0.5",
		DstAddr:      "104.18.6.192",
		DstPort:      443,
		FunctionName: "tcp_connect",
	}

	logs := BuildLogPayload(record)
	body := logs.ResourceLogs().At(0).ScopeLogs().At(0).LogRecords().At(0).Body().Str()

	// Body format: [action] binary src→dst:port (func)
	for _, want := range []string{"[SIGKILL]", "/usr/bin/curl", "10.0.0.5", "104.18.6.192", "443", "tcp_connect"} {
		if !strings.Contains(body, want) {
			t.Errorf("body %q does not contain %q", body, want)
		}
	}
}

func TestOTelSink_EmitNetworkError(t *testing.T) {
	// Point at a closed port — should get a connection refused error.
	sink, err := NewOTelSink(OTelSinkConfig{
		Endpoint:   "http://127.0.0.1:1", // port 1 is always closed
		TimeoutSec: 1,
	})
	if err != nil {
		t.Fatalf("NewOTelSink: %v", err)
	}

	err = sink.Emit(context.Background(), AuditRecord{Severity: "INFO"})
	if err == nil {
		t.Error("expected error for unreachable endpoint")
	}
	if !strings.Contains(err.Error(), "OTLP export") {
		t.Errorf("error should mention OTLP export, got: %v", err)
	}
}

func TestBuildLogPayload_SeverityERROR(t *testing.T) {
	// "ERROR" severity should map to INFO (default) since it's not a
	// recognized elevated severity level.
	logs := BuildLogPayload(AuditRecord{Severity: "ERROR"})
	lr := logs.ResourceLogs().At(0).ScopeLogs().At(0).LogRecords().At(0)
	if lr.SeverityNumber() != plog.SeverityNumberInfo {
		t.Errorf("unexpected severity number for ERROR: %v", lr.SeverityNumber())
	}
}
