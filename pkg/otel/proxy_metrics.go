package otel

import (
	"go.opentelemetry.io/otel/metric"
)

// ProxyMetrics holds OTel metric instruments for the proxy.
type ProxyMetrics struct {
	RequestDuration metric.Float64Histogram
	RequestTotal    metric.Int64Counter
	ActiveRequests  metric.Int64UpDownCounter
	TokensProcessed metric.Int64Counter
	CostUSD         metric.Float64Counter
	DroppedSpans    metric.Int64Counter
	DroppedAsync    metric.Int64Counter
}

// NewProxyMetrics creates OTel metric instruments for the proxy.
// It requires an explicit MeterProvider so callers control the lifecycle
// and tests can use isolated providers instead of the global default.
func NewProxyMetrics(mp metric.MeterProvider) (*ProxyMetrics, error) {
	m := mp.Meter("candela.proxy")

	duration, err := m.Float64Histogram("candela.proxy.request.duration",
		metric.WithDescription("Proxy request duration in seconds"),
		metric.WithUnit("s"),
	)
	if err != nil {
		return nil, err
	}

	total, err := m.Int64Counter("candela.proxy.request.total",
		metric.WithDescription("Total proxy requests"),
	)
	if err != nil {
		return nil, err
	}

	active, err := m.Int64UpDownCounter("candela.proxy.request.active",
		metric.WithDescription("Currently active proxy requests"),
	)
	if err != nil {
		return nil, err
	}

	tokens, err := m.Int64Counter("candela.proxy.tokens.processed",
		metric.WithDescription("Total tokens processed"),
	)
	if err != nil {
		return nil, err
	}

	cost, err := m.Float64Counter("candela.proxy.cost.usd",
		metric.WithDescription("Total cost in USD"),
		metric.WithUnit("USD"),
	)
	if err != nil {
		return nil, err
	}

	droppedSpans, err := m.Int64Counter("candela.proxy.spans.dropped",
		metric.WithDescription("Spans dropped due to semaphore backpressure"),
	)
	if err != nil {
		return nil, err
	}

	droppedAsync, err := m.Int64Counter("candela.proxy.async.dropped",
		metric.WithDescription("Async operations dropped due to semaphore backpressure"),
	)
	if err != nil {
		return nil, err
	}

	return &ProxyMetrics{
		RequestDuration: duration,
		RequestTotal:    total,
		ActiveRequests:  active,
		TokensProcessed: tokens,
		CostUSD:         cost,
		DroppedSpans:    droppedSpans,
		DroppedAsync:    droppedAsync,
	}, nil
}
