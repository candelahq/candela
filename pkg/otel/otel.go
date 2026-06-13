package otel

import (
	"context"
	"fmt"

	promclient "github.com/prometheus/client_golang/prometheus"
	"go.opentelemetry.io/otel"
	otelprom "go.opentelemetry.io/otel/exporters/prometheus"
	"go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

// Config holds OpenTelemetry configuration.
type Config struct {
	ServiceName    string
	ServiceVersion string
	Registry       promclient.Registerer // optional; nil uses the default registry
}

// Setup initializes the OpenTelemetry SDK with a Prometheus metric exporter.
// Returns a shutdown function that must be called on exit.
func Setup(ctx context.Context, cfg Config) (func(context.Context) error, error) {
	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceName(cfg.ServiceName),
			semconv.ServiceVersion(cfg.ServiceVersion),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("otel: creating resource: %w", err)
	}

	var promOpts []otelprom.Option
	if cfg.Registry != nil {
		promOpts = append(promOpts, otelprom.WithRegisterer(cfg.Registry))
	}
	promExporter, err := otelprom.New(promOpts...)
	if err != nil {
		return nil, fmt.Errorf("otel: creating prometheus exporter: %w", err)
	}

	mp := sdkmetric.NewMeterProvider(
		sdkmetric.WithResource(res),
		sdkmetric.WithReader(promExporter),
	)
	otel.SetMeterProvider(mp)

	shutdown := func(ctx context.Context) error {
		return mp.Shutdown(ctx)
	}
	return shutdown, nil
}

// Meter returns a named meter for creating instruments.
func Meter(name string) metric.Meter {
	return otel.Meter(name)
}
