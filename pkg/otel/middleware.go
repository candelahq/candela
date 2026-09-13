package otel

import (
	"fmt"
	"net/http"
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
	if !rec.written {
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
		spanName := fmt.Sprintf("HTTP %s %s", r.Method, r.URL.Path)
		attrs := []attribute.KeyValue{
			semconv.HTTPRequestMethodKey.String(r.Method),
			semconv.URLPathKey.String(r.URL.Path),
			semconv.HTTPRouteKey.String(r.URL.Path),
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

		// 4. Wrap response writer to capture status code.
		rec := newResponseRecorder(w)
		start := time.Now()

		next.ServeHTTP(rec, r.WithContext(ctx))

		// 5. Measure duration and record status code and duration attributes.
		duration := time.Since(start)
		span.SetAttributes(
			semconv.HTTPResponseStatusCodeKey.Int(rec.statusCode),
			attribute.Int64("http.request.duration_ms", duration.Milliseconds()),
		)

		if rec.statusCode >= 500 {
			span.SetStatus(codes.Error, fmt.Sprintf("HTTP %d", rec.statusCode))
		} else {
			span.SetStatus(codes.Ok, "")
		}
	})
}
