package otel

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"
)

// MiddlewareOption allows customizing the HTTP middleware.
type MiddlewareOption func(*middlewareConfig)

type middlewareConfig struct {
	tracerName string
	filter     func(r *http.Request) bool
}

// WithTracerName sets the tracer name used for HTTP server spans.
func WithTracerName(name string) MiddlewareOption {
	return func(c *middlewareConfig) {
		if name != "" {
			c.tracerName = name
		}
	}
}

// WithFilter sets a predicate function to filter requests.
// If filter returns false, the request is not traced.
func WithFilter(filter func(r *http.Request) bool) MiddlewareOption {
	return func(c *middlewareConfig) {
		c.filter = filter
	}
}

// responseRecorder captures the status code and written flag for HTTP responses.
type responseRecorder struct {
	http.ResponseWriter
	statusCode int
	written    bool
}

func newResponseRecorder(w http.ResponseWriter) *responseRecorder {
	return &responseRecorder{
		ResponseWriter: w,
		statusCode:     http.StatusOK, // Default status code if WriteHeader is not explicitly called
	}
}

func (rec *responseRecorder) WriteHeader(code int) {
	if !rec.written && (code == http.StatusSwitchingProtocols || (code >= 200 && code < 600)) {
		rec.statusCode = code
		rec.written = true
	}
	rec.ResponseWriter.WriteHeader(code)
}

func (rec *responseRecorder) Write(b []byte) (int, error) {
	if !rec.written {
		rec.statusCode = http.StatusOK
		rec.written = true
	}
	return rec.ResponseWriter.Write(b)
}

func (rec *responseRecorder) Flush() {
	if f, ok := rec.ResponseWriter.(http.Flusher); ok {
		if !rec.written {
			rec.statusCode = http.StatusOK
			rec.written = true
		}
		f.Flush()
	}
}

func (rec *responseRecorder) Unwrap() http.ResponseWriter {
	return rec.ResponseWriter
}

// HTTPMiddleware creates OpenTelemetry spans for incoming HTTP requests.
// It extracts trace context from incoming W3C Traceparent/Tracestate headers,
// records method, path, status code, and duration attributes, and sets the span status.
func HTTPMiddleware(next http.Handler, opts ...MiddlewareOption) http.Handler {
	cfg := middlewareConfig{
		tracerName: "candela-http",
	}
	for _, opt := range opts {
		opt(&cfg)
	}

	tracer := otel.GetTracerProvider().Tracer(cfg.tracerName)
	propagator := otel.GetTextMapPropagator()

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if cfg.filter != nil && !cfg.filter(r) {
			next.ServeHTTP(w, r)
			return
		}

		// 1. Propagate trace context from incoming Traceparent / W3C headers.
		ctx := propagator.Extract(r.Context(), propagation.HeaderCarrier(r.Header))

		// 2. Start server span with standard HTTP attributes.
		// Avoid high-cardinality span names by using the HTTP method as fallback.
		spanName := r.Method
		attrs := []attribute.KeyValue{
			semconv.HTTPRequestMethodKey.String(r.Method),
			semconv.URLPathKey.String(r.URL.Path),
			semconv.NetworkProtocolVersionKey.String(r.Proto),
		}
		if ua := r.UserAgent(); ua != "" {
			attrs = append(attrs, semconv.UserAgentOriginalKey.String(ua))
		}

		ctx, span := tracer.Start(
			ctx,
			spanName,
			trace.WithSpanKind(trace.SpanKindServer),
			trace.WithAttributes(attrs...),
		)
		defer span.End()

		// 3. Inject trace ID into response header for correlation.
		if spanCtx := span.SpanContext(); spanCtx.IsValid() {
			w.Header().Set("X-Trace-Id", spanCtx.TraceID().String())
		}

		rec := newResponseRecorder(w)
		start := time.Now()

		reqWithCtx := r.WithContext(ctx)
		next.ServeHTTP(rec, reqWithCtx)

		// If a route pattern was matched during handler execution (e.g., Go 1.22+ ServeMux),
		// update the span name and set http.route attribute to avoid high cardinality.
		if reqWithCtx.Pattern != "" {
			route := reqWithCtx.Pattern
			if strings.HasPrefix(route, r.Method+" ") {
				route = strings.TrimPrefix(route, r.Method+" ")
			}
			span.SetName(fmt.Sprintf("%s %s", r.Method, route))
			span.SetAttributes(semconv.HTTPRouteKey.String(route))
		}

		// 5. Measure duration and record status code and duration attributes.
		duration := time.Since(start)
		span.SetAttributes(
			semconv.HTTPResponseStatusCodeKey.Int(rec.statusCode),
			attribute.Int64("http.request.duration_ms", duration.Milliseconds()),
		)

		if rec.statusCode >= 500 {
			span.SetStatus(codes.Error, "")
		}
	})
}
