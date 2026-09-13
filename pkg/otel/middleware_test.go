package otel_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"

	candelaotel "github.com/candelahq/candela/pkg/otel"
)

func setupTestTracer(t *testing.T) (*tracetest.InMemoryExporter, *sdktrace.TracerProvider) {
	t.Helper()
	exporter := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSyncer(exporter),
	)
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))
	t.Cleanup(func() {
		_ = tp.Shutdown(context.Background())
		otel.SetTracerProvider(noop.NewTracerProvider())
	})
	return exporter, tp
}

func TestHTTPMiddleware_CreatesSpanWithAttributes(t *testing.T) {
	exporter, _ := setupTestTracer(t)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(10 * time.Millisecond) // Ensure duration > 0
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})

	wrapped := candelaotel.HTTPMiddleware(handler)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/models", nil)
	req.Header.Set("User-Agent", "CandelaClient/1.0")
	rec := httptest.NewRecorder()

	wrapped.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	// Verify X-Trace-Id header is injected
	traceID := rec.Header().Get("X-Trace-Id")
	if traceID == "" {
		t.Errorf("expected X-Trace-Id response header, got empty")
	}

	spans := exporter.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}

	span := spans[0]
	if span.Name != "HTTP GET /api/v1/models" {
		t.Errorf("span.Name = %q, want 'HTTP GET /api/v1/models'", span.Name)
	}
	if span.SpanKind != trace.SpanKindServer {
		t.Errorf("span.SpanKind = %v, want Server (%v)", span.SpanKind, trace.SpanKindServer)
	}

	attrs := make(map[string]any)
	for _, attr := range span.Attributes {
		attrs[string(attr.Key)] = attr.Value.AsInterface()
	}

	if attrs["http.request.method"] != "GET" {
		t.Errorf("http.request.method = %v, want GET", attrs["http.request.method"])
	}
	if attrs["url.path"] != "/api/v1/models" {
		t.Errorf("url.path = %v, want /api/v1/models", attrs["url.path"])
	}
	if attrs["http.response.status_code"] != int64(200) {
		t.Errorf("http.response.status_code = %v, want 200", attrs["http.response.status_code"])
	}
	if attrs["user_agent.original"] != "CandelaClient/1.0" {
		t.Errorf("user_agent.original = %v, want CandelaClient/1.0", attrs["user_agent.original"])
	}

	durationMs, ok := attrs["http.request.duration_ms"].(int64)
	if !ok || durationMs < 0 {
		t.Errorf("http.request.duration_ms = %v, want non-negative int64", attrs["http.request.duration_ms"])
	}

	if span.Status.Code != codes.Ok {
		t.Errorf("span.Status.Code = %v, want %v", span.Status.Code, codes.Ok)
	}
}

func TestHTTPMiddleware_TraceparentPropagation(t *testing.T) {
	exporter, _ := setupTestTracer(t)

	var innerSpanCtx trace.SpanContext
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		innerSpanCtx = trace.SpanContextFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	})

	wrapped := candelaotel.HTTPMiddleware(handler)

	// Incoming W3C Traceparent header
	// Format: version-traceid-parentid-traceflags
	incomingTraceID := "4bf92f3577b34da6a3ce929d0e0e4736"
	incomingParentID := "00f067aa0ba902b7"
	traceparent := fmt.Sprintf("00-%s-%s-01", incomingTraceID, incomingParentID)

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	req.Header.Set("Traceparent", traceparent)
	rec := httptest.NewRecorder()

	wrapped.ServeHTTP(rec, req)

	// Check that inner request context extracted the incoming trace ID
	if !innerSpanCtx.IsValid() {
		t.Fatal("expected valid span context in request context")
	}
	if innerSpanCtx.TraceID().String() != incomingTraceID {
		t.Errorf("inner trace ID = %s, want %s", innerSpanCtx.TraceID().String(), incomingTraceID)
	}

	// Check response header matches incoming trace ID
	if got := rec.Header().Get("X-Trace-Id"); got != incomingTraceID {
		t.Errorf("X-Trace-Id header = %s, want %s", got, incomingTraceID)
	}

	// Check exported span inherits the parent trace ID
	spans := exporter.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	span := spans[0]
	if span.SpanContext.TraceID().String() != incomingTraceID {
		t.Errorf("span trace ID = %s, want %s", span.SpanContext.TraceID().String(), incomingTraceID)
	}
	if span.Parent.SpanID().String() != incomingParentID {
		t.Errorf("span parent ID = %s, want %s", span.Parent.SpanID().String(), incomingParentID)
	}
}

func TestHTTPMiddleware_ErrorStatusCode(t *testing.T) {
	exporter, _ := setupTestTracer(t)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal database failure", http.StatusInternalServerError)
	})

	wrapped := candelaotel.HTTPMiddleware(handler)

	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rec := httptest.NewRecorder()

	wrapped.ServeHTTP(rec, req)

	spans := exporter.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	span := spans[0]

	attrs := make(map[string]any)
	for _, attr := range span.Attributes {
		attrs[string(attr.Key)] = attr.Value.AsInterface()
	}

	if attrs["http.response.status_code"] != int64(500) {
		t.Errorf("http.response.status_code = %v, want 500", attrs["http.response.status_code"])
	}
	if span.Status.Code != codes.Error {
		t.Errorf("span.Status.Code = %v, want %v (Error)", span.Status.Code, codes.Error)
	}
	if span.Status.Description != "HTTP 500" {
		t.Errorf("span.Status.Description = %q, want 'HTTP 500'", span.Status.Description)
	}
}

func TestHTTPMiddleware_FilterOption(t *testing.T) {
	exporter, _ := setupTestTracer(t)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// Filter out /healthz
	filter := func(r *http.Request) bool {
		return r.URL.Path != "/healthz"
	}

	wrapped := candelaotel.HTTPMiddleware(handler, candelaotel.WithFilter(filter))

	// Request to /healthz should be ignored
	req1 := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec1 := httptest.NewRecorder()
	wrapped.ServeHTTP(rec1, req1)

	if len(exporter.GetSpans()) != 0 {
		t.Errorf("expected 0 spans for filtered /healthz, got %d", len(exporter.GetSpans()))
	}

	// Request to /api/v1/traces should be traced
	req2 := httptest.NewRequest(http.MethodGet, "/api/v1/traces", nil)
	rec2 := httptest.NewRecorder()
	wrapped.ServeHTTP(rec2, req2)

	if len(exporter.GetSpans()) != 1 {
		t.Errorf("expected 1 span for /api/v1/traces, got %d", len(exporter.GetSpans()))
	}
}
