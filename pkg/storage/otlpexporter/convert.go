// Package otlpexporter implements storage.SpanWriter for exporting Candela spans
// as standard OpenTelemetry traces via OTLP/HTTP or OTLP/gRPC.
//
// This is a write-only export sink — it does not replace the primary storage
// backend (DuckDB/BigQuery). It enables forwarding LLM observability data
// to any OTel-compatible collector or backend (Datadog, Grafana Tempo,
// Jaeger, Elastic, Honeycomb, etc.).
package otlpexporter

import (
	"encoding/hex"
	"log/slog"

	"github.com/candelahq/candela/pkg/storage"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	resourcepb "go.opentelemetry.io/proto/otlp/resource/v1"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
)

// spansToResourceSpans groups Candela spans by (ProjectID, ServiceName)
// and builds the OTLP ResourceSpans hierarchy.
func spansToResourceSpans(spans []storage.Span) []*tracepb.ResourceSpans {
	// Group spans by resource identity.
	type resourceKey struct {
		ProjectID   string
		ServiceName string
		Environment string
	}

	groups := make(map[resourceKey][]storage.Span)
	// Preserve insertion order for deterministic output.
	var keys []resourceKey

	for _, s := range spans {
		svcName := s.ServiceName
		if svcName == "" {
			svcName = "candela"
		}
		k := resourceKey{
			ProjectID:   s.ProjectID,
			ServiceName: svcName,
			Environment: s.Environment,
		}
		if _, exists := groups[k]; !exists {
			keys = append(keys, k)
		}
		groups[k] = append(groups[k], s)
	}

	result := make([]*tracepb.ResourceSpans, 0, len(keys))
	for _, k := range keys {
		group := groups[k]
		protoSpans := make([]*tracepb.Span, len(group))
		for i, s := range group {
			protoSpans[i] = convertSpan(s)
		}

		result = append(result, &tracepb.ResourceSpans{
			Resource: buildResource(k.ProjectID, k.ServiceName, k.Environment),
			ScopeSpans: []*tracepb.ScopeSpans{{
				Scope: &commonpb.InstrumentationScope{
					Name:    "candela",
					Version: "1.0.0",
				},
				Spans: protoSpans,
			}},
		})
	}

	return result
}

// convertSpan maps a storage.Span to an OTLP tracepb.Span.
func convertSpan(s storage.Span) *tracepb.Span {
	return &tracepb.Span{
		TraceId:                hexToBytes(s.TraceID, 16),
		SpanId:                 hexToBytes(s.SpanID, 8),
		ParentSpanId:           hexToBytes(s.ParentSpanID, 8),
		Name:                   s.Name,
		Kind:                   mapSpanKind(s.Kind),
		StartTimeUnixNano:      uint64(s.StartTime.UnixNano()),
		EndTimeUnixNano:        uint64(s.EndTime.UnixNano()),
		Status:                 mapStatus(s.Status, s.StatusMessage),
		Attributes:             buildAttributes(s),
		DroppedAttributesCount: 0,
	}
}

// buildResource creates an OTLP Resource from project/service metadata.
func buildResource(projectID, serviceName, environment string) *resourcepb.Resource {
	var attrs []*commonpb.KeyValue

	if serviceName != "" {
		attrs = append(attrs, stringAttr("service.name", serviceName))
	}
	if projectID != "" {
		attrs = append(attrs, stringAttr("service.namespace", projectID))
	}
	if environment != "" {
		attrs = append(attrs, stringAttr("deployment.environment", environment))
	}

	return &resourcepb.Resource{Attributes: attrs}
}

// buildAttributes creates OTLP attributes from a Candela span,
// mapping GenAI fields to OTel GenAI semantic convention keys.
//
// Numeric GenAI fields are always emitted when GenAI is non-nil
// (zero is valid — e.g. Temperature=0.0 for deterministic, CostUSD=0.0 for free models).
// Custom pass-through attributes that collide with explicit keys are skipped.
func buildAttributes(s storage.Span) []*commonpb.KeyValue {
	var attrs []*commonpb.KeyValue
	setKeys := make(map[string]bool)

	add := func(kv *commonpb.KeyValue) {
		attrs = append(attrs, kv)
		setKeys[kv.Key] = true
	}

	// GenAI semantic convention attributes.
	if g := s.GenAI; g != nil {
		if g.Provider != "" {
			add(stringAttr("gen_ai.system", g.Provider))
		}
		if g.Model != "" {
			add(stringAttr("gen_ai.request.model", g.Model))
		}
		// Always emit numeric fields — zero is a valid value.
		add(int64Attr("gen_ai.usage.input_tokens", g.InputTokens))
		add(int64Attr("gen_ai.usage.output_tokens", g.OutputTokens))
		add(int64Attr("gen_ai.usage.total_tokens", g.TotalTokens))
		add(float64Attr("gen_ai.usage.cost", g.CostUSD))
		add(float64Attr("gen_ai.request.temperature", g.Temperature))
		add(int64Attr("gen_ai.request.max_tokens", g.MaxTokens))
		if g.InputContent != "" {
			add(stringAttr("gen_ai.prompt", g.InputContent))
		}
		if g.OutputContent != "" {
			add(stringAttr("gen_ai.completion", g.OutputContent))
		}
	}

	// User identity.
	if s.UserID != "" {
		add(stringAttr("enduser.id", s.UserID))
	}

	// Pass-through custom attributes — skip keys already set above to avoid duplicates.
	for k, v := range s.Attributes {
		if !setKeys[k] {
			attrs = append(attrs, stringAttr(k, v))
		}
	}

	return attrs
}

// --- ID encoding ---

// hexToBytes decodes a hex string to a byte slice of the given size.
// Returns a zero-filled slice if decoding fails.
func hexToBytes(hexStr string, size int) []byte {
	if hexStr == "" {
		return make([]byte, size)
	}

	b, err := hex.DecodeString(hexStr)
	if err != nil {
		slog.Debug("otlpexporter: invalid hex ID, using zero bytes",
			"hex", hexStr, "error", err)
		return make([]byte, size)
	}

	// Pad or truncate to the expected size.
	if len(b) == size {
		return b
	}
	result := make([]byte, size)
	if len(b) < size {
		// Right-align (pad with leading zeros).
		copy(result[size-len(b):], b)
	} else {
		// Truncate to size (take the last `size` bytes).
		copy(result, b[len(b)-size:])
	}
	return result
}

// --- Kind / Status mapping ---

// mapSpanKind converts a Candela SpanKind to an OTLP SpanKind.
func mapSpanKind(k storage.SpanKind) tracepb.Span_SpanKind {
	switch k {
	case storage.SpanKindLLM:
		return tracepb.Span_SPAN_KIND_CLIENT // LLM calls are outbound client calls
	case storage.SpanKindAgent:
		return tracepb.Span_SPAN_KIND_INTERNAL
	case storage.SpanKindTool:
		return tracepb.Span_SPAN_KIND_CLIENT // Tool calls are outbound
	case storage.SpanKindRetrieval:
		return tracepb.Span_SPAN_KIND_CLIENT
	case storage.SpanKindEmbedding:
		return tracepb.Span_SPAN_KIND_CLIENT
	case storage.SpanKindChain:
		return tracepb.Span_SPAN_KIND_INTERNAL
	case storage.SpanKindGeneral:
		return tracepb.Span_SPAN_KIND_INTERNAL
	default:
		return tracepb.Span_SPAN_KIND_UNSPECIFIED
	}
}

// mapStatus converts a Candela SpanStatus to an OTLP Status.
func mapStatus(s storage.SpanStatus, message string) *tracepb.Status {
	switch s {
	case storage.SpanStatusOK:
		return &tracepb.Status{Code: tracepb.Status_STATUS_CODE_OK}
	case storage.SpanStatusError:
		return &tracepb.Status{
			Code:    tracepb.Status_STATUS_CODE_ERROR,
			Message: message,
		}
	default:
		return &tracepb.Status{Code: tracepb.Status_STATUS_CODE_UNSET}
	}
}

// --- Attribute helpers ---

func stringAttr(key, value string) *commonpb.KeyValue {
	return &commonpb.KeyValue{
		Key:   key,
		Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: value}},
	}
}

func int64Attr(key string, value int64) *commonpb.KeyValue {
	return &commonpb.KeyValue{
		Key:   key,
		Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_IntValue{IntValue: value}},
	}
}

func float64Attr(key string, value float64) *commonpb.KeyValue {
	return &commonpb.KeyValue{
		Key:   key,
		Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_DoubleValue{DoubleValue: value}},
	}
}
