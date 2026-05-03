package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sort"

	"cloud.google.com/go/pubsub/v2"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	typespb "github.com/candelahq/candela/gen/go/candela/types"
	"github.com/candelahq/candela/pkg/storage"
)

// PubSubWriter implements storage.SpanWriter by publishing spans to a
// Google Cloud Pub/Sub topic. Supports proto (default) and JSON formats.
type PubSubWriter struct {
	publisher *pubsub.Publisher
	client    *pubsub.Client
	format    string // "proto" or "json"
}

// NewPubSubWriter creates a new Pub/Sub span writer.
// format must be "proto" (default) or "json".
func NewPubSubWriter(ctx context.Context, projectID, topicID, format string) (*PubSubWriter, error) {
	if format == "" {
		format = "proto"
	}
	if format != "proto" && format != "json" {
		return nil, fmt.Errorf("unsupported span format %q (use \"proto\" or \"json\")", format)
	}

	client, err := pubsub.NewClient(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("pubsub.NewClient: %w", err)
	}

	publisher := client.Publisher(topicID)

	return &PubSubWriter{
		publisher: publisher,
		client:    client,
		format:    format,
	}, nil
}

// IngestSpans publishes a batch of spans to Pub/Sub.
func (w *PubSubWriter) IngestSpans(ctx context.Context, spans []storage.Span) error {
	var results []*pubsub.PublishResult

	for _, span := range spans {
		data, err := w.marshal(span)
		if err != nil {
			slog.Error("failed to marshal span for pubsub",
				"span_id", span.SpanID, "error", err)
			continue
		}

		msg := &pubsub.Message{
			Data: data,
			Attributes: map[string]string{
				"trace_id":   span.TraceID,
				"project_id": span.ProjectID,
				"format":     w.format,
			},
		}
		results = append(results, w.publisher.Publish(ctx, msg))
	}

	// Wait for all publishes to complete.
	var firstErr error
	for _, r := range results {
		if _, err := r.Get(ctx); err != nil && firstErr == nil {
			firstErr = err
		}
	}

	if firstErr != nil {
		return fmt.Errorf("pubsub publish: %w", firstErr)
	}

	slog.Debug("published spans to pubsub", "count", len(spans), "format", w.format)
	return nil
}

// Close flushes pending messages and closes the client.
func (w *PubSubWriter) Close() error {
	w.publisher.Stop()
	return w.client.Close()
}

// marshal serializes a span in the configured format.
func (w *PubSubWriter) marshal(span storage.Span) ([]byte, error) {
	switch w.format {
	case "proto":
		return w.marshalProto(span)
	case "json":
		return json.Marshal(span)
	default:
		return nil, fmt.Errorf("unsupported format: %s", w.format)
	}
}

// marshalProto converts a storage.Span to its protobuf wire format.
func (w *PubSubWriter) marshalProto(span storage.Span) ([]byte, error) {
	pb := &typespb.Span{
		SpanId:       span.SpanID,
		TraceId:      span.TraceID,
		ParentSpanId: span.ParentSpanID,
		Name:         span.Name,
		Kind:         typespb.SpanKind(span.Kind),
		Status:       typespb.SpanStatus(span.Status),
		StartTime:    timestamppb.New(span.StartTime),
		EndTime:      timestamppb.New(span.EndTime),
		Duration:     durationpb.New(span.Duration),
		ProjectId:    span.ProjectID,
		Environment:  span.Environment,
		ServiceName:  span.ServiceName,
		UserId:       span.UserID,
	}

	if span.GenAI != nil {
		pb.GenAi = &typespb.GenAIAttributes{
			Model:         span.GenAI.Model,
			Provider:      span.GenAI.Provider,
			InputTokens:   span.GenAI.InputTokens,
			OutputTokens:  span.GenAI.OutputTokens,
			TotalTokens:   span.GenAI.TotalTokens,
			CostUsd:       span.GenAI.CostUSD,
			Temperature:   span.GenAI.Temperature,
			MaxTokens:     span.GenAI.MaxTokens,
			TopP:          span.GenAI.TopP,
			InputContent:  span.GenAI.InputContent,
			OutputContent: span.GenAI.OutputContent,
		}
	}

	if len(span.Attributes) > 0 {
		keys := make([]string, 0, len(span.Attributes))
		for k := range span.Attributes {
			keys = append(keys, k)
		}
		sort.Strings(keys)

		for _, k := range keys {
			pb.Attributes = append(pb.Attributes, &typespb.Attribute{
				Key:   k,
				Value: &typespb.Attribute_StringValue{StringValue: span.Attributes[k]},
			})
		}
	}

	return proto.Marshal(pb)
}
