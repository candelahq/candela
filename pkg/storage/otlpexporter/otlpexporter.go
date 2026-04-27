package otlpexporter

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/candelahq/candela/pkg/storage"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
)

var _ storage.SpanWriter = (*Writer)(nil)

// Config holds OTLP export configuration.
type Config struct {
	Endpoint    string            `yaml:"endpoint"`    // e.g. "http://localhost:4318"
	Protocol    string            `yaml:"protocol"`    // "http" (default) or "grpc"
	Headers     map[string]string `yaml:"headers"`     // optional auth headers
	Insecure    bool              `yaml:"insecure"`    // skip TLS verification
	Compression string            `yaml:"compression"` // "gzip" (default) or "none"
	TimeoutSec  int               `yaml:"timeout_sec"` // per-export timeout (default: 30)
}

// Writer exports Candela spans as OTLP traces to any OTel-compatible backend.
type Writer struct {
	client  otlptrace.Client
	timeout time.Duration
}

// New creates a new OTLP span writer.
func New(ctx context.Context, cfg Config) (*Writer, error) {
	if cfg.Endpoint == "" {
		return nil, fmt.Errorf("otlpexporter: endpoint is required")
	}
	if cfg.Protocol == "" {
		cfg.Protocol = "http"
	}
	if cfg.Compression == "" {
		cfg.Compression = "gzip"
	}
	if cfg.TimeoutSec <= 0 {
		cfg.TimeoutSec = 30
	}

	timeout := time.Duration(cfg.TimeoutSec) * time.Second

	var client otlptrace.Client
	var err error

	switch cfg.Protocol {
	case "http":
		client, err = newHTTPClient(cfg)
	case "grpc":
		return nil, fmt.Errorf("otlpexporter: gRPC support not yet implemented (use protocol: http)")
	default:
		return nil, fmt.Errorf("otlpexporter: unsupported protocol %q (use \"http\" or \"grpc\")", cfg.Protocol)
	}
	if err != nil {
		return nil, fmt.Errorf("otlpexporter: creating client: %w", err)
	}

	// Start the client (establishes connections).
	if err := client.Start(ctx); err != nil {
		return nil, fmt.Errorf("otlpexporter: starting client: %w", err)
	}

	slog.Info("📡 OTLP span exporter initialized",
		"endpoint", cfg.Endpoint,
		"protocol", cfg.Protocol,
		"compression", cfg.Compression,
		"timeout_sec", cfg.TimeoutSec)

	return &Writer{
		client:  client,
		timeout: timeout,
	}, nil
}

// IngestSpans converts Candela spans to OTLP format and exports them.
func (w *Writer) IngestSpans(ctx context.Context, spans []storage.Span) error {
	if len(spans) == 0 {
		return nil
	}

	// Apply per-export timeout to avoid blocking the processor fan-out.
	ctx, cancel := context.WithTimeout(ctx, w.timeout)
	defer cancel()

	resourceSpans := spansToResourceSpans(spans)

	if err := w.client.UploadTraces(ctx, resourceSpans); err != nil {
		return fmt.Errorf("otlpexporter: upload failed: %w", err)
	}

	slog.Debug("otlp: exported spans", "count", len(spans))
	return nil
}

// Close shuts down the OTLP client, flushing any pending data.
func (w *Writer) Close() error {
	if w.client == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return w.client.Stop(ctx)
}

// newHTTPClient creates an OTLP/HTTP trace client.
func newHTTPClient(cfg Config) (otlptrace.Client, error) {
	opts := []otlptracehttp.Option{
		otlptracehttp.WithEndpointURL(cfg.Endpoint),
	}

	if cfg.Insecure {
		opts = append(opts, otlptracehttp.WithInsecure())
	}

	if len(cfg.Headers) > 0 {
		opts = append(opts, otlptracehttp.WithHeaders(cfg.Headers))
	}

	switch cfg.Compression {
	case "none":
		opts = append(opts, otlptracehttp.WithCompression(otlptracehttp.NoCompression))
	case "gzip":
		opts = append(opts, otlptracehttp.WithCompression(otlptracehttp.GzipCompression))
	default:
		slog.Warn("otlpexporter: unsupported compression type, defaulting to gzip", "type", cfg.Compression)
		opts = append(opts, otlptracehttp.WithCompression(otlptracehttp.GzipCompression))
	}

	return otlptracehttp.NewClient(opts...), nil
}
