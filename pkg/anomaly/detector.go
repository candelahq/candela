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
	// "cost_usd_2sigma" or "latency_ms_2sigma".
	AttrAnomalyReason = "candela.anomaly_reason"
)

// Config controls detector behaviour.
type Config struct {
	// WindowDays is the rolling lookback used to build the baseline.
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

// groupKey identifies a unique (tenant, model) baseline group.
type groupKey struct {
	tenantID  string
	model     string
	projectID string
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
// and returns anomalous spans. It stamps anomaly attributes onto copies of
// flagged spans so callers can re-ingest or surface them downstream.
//
// Spans are grouped by (tenant, model) before querying so the batch issues one
// SearchSpans call per unique group, not one per span.
//
// Only spans with Kind == SpanKindLLM and a non-nil GenAI payload are evaluated.
func (d *Detector) Detect(ctx context.Context, spans []storage.Span) ([]Result, error) {
	// Partition LLM spans by (tenant, model, project) to avoid N+1 queries.
	groups := make(map[groupKey][]storage.Span)
	for _, span := range spans {
		if span.Kind != storage.SpanKindLLM || span.GenAI == nil {
			continue
		}
		k := groupKey{tenantID: span.TenantID, model: span.GenAI.Model, projectID: span.ProjectID}
		groups[k] = append(groups[k], span)
	}

	var results []Result
	for key, group := range groups {
		// Use the earliest StartTime in the group as the window anchor so all
		// spans in the group are covered by the same historical query.
		windowEnd := group[0].StartTime
		for _, s := range group[1:] {
			if s.StartTime.Before(windowEnd) {
				windowEnd = s.StartTime
			}
		}
		windowStart := windowEnd.UTC().Add(-time.Duration(d.cfg.WindowDays) * 24 * time.Hour)

		historical, err := d.reader.SearchSpans(ctx, storage.SpanQuery{
			ProjectID: key.projectID,
			StartTime: windowStart,
			EndTime:   windowEnd,
			Kind:      storage.SpanKindLLM,
			Model:     key.model,
			TenantID:  key.tenantID,
			PageSize:  10000,
		})
		if err != nil {
			return nil, err
		}
		if historical == nil {
			return nil, fmt.Errorf("received nil span result from reader")
		}

		for _, span := range group {
			rs := d.checkGroup(span, historical.Spans)
			results = append(results, rs...)
		}
	}
	return results, nil
}

// checkGroup evaluates a single span against a pre-fetched history slice.
func (d *Detector) checkGroup(span storage.Span, history []storage.Span) []Result {
	var results []Result

	if r, ok := d.checkMetric(span, history, "cost_usd", func(s storage.Span) float64 {
		if s.GenAI == nil {
			return 0
		}
		return s.GenAI.CostUSD
	}); ok {
		results = append(results, r)
	}

	if r, ok := d.checkMetric(span, history, "latency_ms", func(s storage.Span) float64 {
		return float64(s.Duration.Milliseconds())
	}); ok {
		results = append(results, r)
	}

	return results
}

// checkMetric computes the rolling mean and stddev for a single metric and
// returns a Result if the span's value exceeds the sigma threshold.
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
	// when all historical values are identical.
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
	// Require the absolute delta to exceed 10% of the mean to filter noise
	// when stddev is near-zero.
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
