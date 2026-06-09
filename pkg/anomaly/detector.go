// Package anomaly detects statistical deviations in LLM span cost and latency.
// It maintains a rolling per-(tenant, model) baseline and flags spans that
// exceed a configurable sigma threshold above the mean. Anomaly metadata is
// attached to the span's Attributes map under well-known keys so the dashboard
// and audit trail can surface it without schema changes.
package anomaly

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/candelahq/candela/pkg/storage"
)

const (
	// AttrAnomaly is set to "true" on flagged spans.
	AttrAnomaly = "candela.anomaly"
	// AttrAnomalyReason describes which metric triggered the flag, e.g.
	// "cost_2.5sigma" or "latency_2.5sigma".
	AttrAnomalyReason = "candela.anomaly_reason"
)

// Config controls detector behaviour.
type Config struct {
	// WindowDays is the rolling lookback used to build the baseline.
	// Spans older than WindowDays before the evaluated span are excluded.
	WindowDays int
	// SigmaThreshold is the number of standard deviations above the mean
	// required to flag a span. Lower values = more sensitive.
	SigmaThreshold float64
	// MinSamples is the minimum number of historical spans required before
	// the detector emits any anomaly. Below this the baseline is too noisy.
	MinSamples int
}

// DefaultConfig returns a conservative production-ready configuration.
func DefaultConfig() Config {
	return Config{
		WindowDays:     7,
		SigmaThreshold: 2.0,
		MinSamples:     10,
	}
}

// Result describes a single anomalous span.
type Result struct {
	Span   storage.Span
	Metric string  // "cost_usd" or "latency_ms"
	Value  float64 // observed value
	Mean   float64 // baseline mean
	StdDev float64 // baseline stddev
	Sigma  float64 // how many sigma above mean
}

// Detector checks incoming LLM spans against a rolling baseline.
type Detector struct {
	reader storage.SpanReader
	cfg    Config
}

// New creates a Detector backed by the given SpanReader.
func New(reader storage.SpanReader, cfg Config) *Detector {
	return &Detector{reader: reader, cfg: cfg}
}

// Detect checks each span in the batch against its per-(tenant, model) baseline
// and returns anomalous spans. It also stamps anomaly attributes directly onto
// the span copies in the returned results so callers can re-ingest or surface
// them downstream.
//
// Only spans with Kind == SpanKindLLM and a non-zero cost or duration are
// evaluated; all others pass through silently.
func (d *Detector) Detect(ctx context.Context, spans []storage.Span) ([]Result, error) {
	var results []Result
	for _, span := range spans {
		if span.Kind != storage.SpanKindLLM {
			continue
		}
		if span.GenAI == nil {
			continue
		}
		rs, err := d.checkSpan(ctx, span)
		if err != nil {
			return nil, err
		}
		results = append(results, rs...)
	}
	return results, nil
}

// checkSpan evaluates a single span for cost and latency anomalies.
func (d *Detector) checkSpan(ctx context.Context, span storage.Span) ([]Result, error) {
	windowStart := span.StartTime.UTC().Add(-time.Duration(d.cfg.WindowDays) * 24 * time.Hour)

	historical, err := d.reader.SearchSpans(ctx, storage.SpanQuery{
		ProjectID: span.ProjectID,
		StartTime: windowStart,
		EndTime:   span.StartTime,
		Kind:      storage.SpanKindLLM,
		Model:     span.GenAI.Model,
		TenantID:  span.TenantID,
		PageSize:  10000,
	})
	if err != nil {
		return nil, err
	}

	var results []Result

	if r, ok := d.checkMetric(span, historical.Spans, "cost_usd", func(s storage.Span) float64 {
		if s.GenAI == nil {
			return 0
		}
		return s.GenAI.CostUSD
	}); ok {
		results = append(results, r)
	}

	if r, ok := d.checkMetric(span, historical.Spans, "latency_ms", func(s storage.Span) float64 {
		return float64(s.Duration.Milliseconds())
	}); ok {
		results = append(results, r)
	}

	return results, nil
}

// checkMetric computes the rolling mean and stddev for a single metric across
// the historical window and returns a Result if the span's value exceeds the
// sigma threshold.
func (d *Detector) checkMetric(span storage.Span, history []storage.Span, metric string, extract func(storage.Span) float64) (Result, bool) {
	vals := make([]float64, 0, len(history))
	for _, h := range history {
		if v := extract(h); v > 0 {
			vals = append(vals, v)
		}
	}
	if len(vals) < d.cfg.MinSamples {
		return Result{}, false
	}

	mean, stddev := stats(vals)
	// Treat near-zero stddev (< 0.1% of mean) as zero to avoid false positives
	// from floating-point precision when all historical values are identical.
	if mean > 0 && stddev/mean < 0.001 {
		return Result{}, false
	}
	if stddev == 0 {
		return Result{}, false
	}

	observed := extract(span)
	if observed <= 0 {
		return Result{}, false
	}

	sigma := (observed - mean) / stddev
	if sigma < d.cfg.SigmaThreshold {
		return Result{}, false
	}
	// Require the absolute delta to exceed 10% of the mean to avoid flagging
	// noise when stddev is near-zero (all-identical history).
	if mean > 0 && (observed-mean)/mean < 0.10 {
		return Result{}, false
	}

	flagged := span
	newAttrs := make(map[string]string, len(span.Attributes)+2)
	for k, v := range span.Attributes {
		newAttrs[k] = v
	}
	newAttrs[AttrAnomaly] = "true"
	newAttrs[AttrAnomalyReason] = metric + "_" + formatSigma(d.cfg.SigmaThreshold) + "sigma"
	flagged.Attributes = newAttrs

	return Result{
		Span:   flagged,
		Metric: metric,
		Value:  observed,
		Mean:   mean,
		StdDev: stddev,
		Sigma:  sigma,
	}, true
}

// stats computes the mean and population standard deviation of vals.
func stats(vals []float64) (mean, stddev float64) {
	if len(vals) == 0 {
		return 0, 0
	}
	sum := 0.0
	for _, v := range vals {
		sum += v
	}
	mean = sum / float64(len(vals))
	variance := 0.0
	for _, v := range vals {
		d := v - mean
		variance += d * d
	}
	variance /= float64(len(vals))
	return mean, math.Sqrt(variance)
}

// formatSigma formats the threshold as a short string for the reason attribute.
func formatSigma(sigma float64) string {
	if sigma == float64(int(sigma)) {
		return fmt.Sprintf("%d", int(sigma))
	}
	return fmt.Sprintf("%.1f", sigma)
}
