package otel

import (
	"context"
	"fmt"

	promclient "github.com/prometheus/client_golang/prometheus"
	"go.opentelemetry.io/otel"
	otelprom "go.opentelemetry.io/otel/exporters/prometheus"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/propagation"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"
)

func init() {
	// Set default composite propagator so W3C Traceparent and Baggage are extracted.
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))
}

// Config holds OpenTelemetry configuration.
type Config struct {
	ServiceName    string
	ServiceVersion string
	Registry       promclient.Registerer // optional; nil uses the default registry
}

// Setup initializes the OpenTelemetry SDK with a Prometheus metric exporter
// and a TracerProvider. Returns a shutdown function that must be called on exit.
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

	// Register default W3C propagator.
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

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

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithResource(res),
	)
	otel.SetTracerProvider(tp)

	shutdown := func(ctx context.Context) error {
		errM := mp.Shutdown(ctx)
		errT := tp.Shutdown(ctx)
		if errM != nil {
			return errM
		}
		return errT
	}
	return shutdown, nil
}

// Meter returns a named meter for creating instruments.
func Meter(name string) metric.Meter {
	return otel.Meter(name)
}

// Tracer returns a named tracer for creating spans.
func Tracer(name string) trace.Tracer {
	return otel.Tracer(name)
}
