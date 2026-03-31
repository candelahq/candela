// Package pubsub implements storage.SpanWriter using Google Cloud Pub/Sub.
// This is a write-only sink — spans are serialized to JSON and published
// to a configurable topic. Useful for fan-out to downstream consumers
// (analytics pipelines, alerting, data lakes) without coupling them
// to the primary query store.
package pubsub

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"

	"cloud.google.com/go/pubsub"

	"github.com/candelahq/candela/pkg/storage"
)

// Config holds Pub/Sub sink settings.
type Config struct {
	ProjectID string `yaml:"project_id"` // GCP project ID
	TopicID   string `yaml:"topic"`      // Pub/Sub topic name
}

// Writer implements storage.SpanWriter by publishing spans to Pub/Sub.
type Writer struct {
	client *pubsub.Client
	topic  *pubsub.Topic
	config Config
}

var _ storage.SpanWriter = (*Writer)(nil)

// New creates a Pub/Sub writer. The topic must already exist.
func New(ctx context.Context, cfg Config) (*Writer, error) {
	if cfg.ProjectID == "" {
		return nil, fmt.Errorf("pubsub: project_id is required")
	}
	if cfg.TopicID == "" {
		return nil, fmt.Errorf("pubsub: topic is required")
	}

	client, err := pubsub.NewClient(ctx, cfg.ProjectID)
	if err != nil {
		return nil, fmt.Errorf("pubsub: creating client: %w", err)
	}

	topic := client.Topic(cfg.TopicID)

	// Verify the topic exists.
	exists, err := topic.Exists(ctx)
	if err != nil {
		client.Close()
		return nil, fmt.Errorf("pubsub: checking topic: %w", err)
	}
	if !exists {
		client.Close()
		return nil, fmt.Errorf("pubsub: topic %q does not exist", cfg.TopicID)
	}

	// Enable batching for throughput.
	topic.PublishSettings.CountThreshold = 100
	topic.PublishSettings.ByteThreshold = 1 << 20 // 1MB

	slog.Info("pubsub sink ready",
		"project", cfg.ProjectID,
		"topic", cfg.TopicID)

	return &Writer{
		client: client,
		topic:  topic,
		config: cfg,
	}, nil
}

// spanMessage is the JSON envelope published to Pub/Sub.
type spanMessage struct {
	SpanID        string            `json:"span_id"`
	TraceID       string            `json:"trace_id"`
	ParentSpanID  string            `json:"parent_span_id,omitempty"`
	Name          string            `json:"name"`
	Kind          int               `json:"kind"`
	Status        int               `json:"status"`
	StatusMessage string            `json:"status_message,omitempty"`
	StartTime     string            `json:"start_time"`
	EndTime       string            `json:"end_time"`
	DurationNs    int64             `json:"duration_ns"`
	ProjectID     string            `json:"project_id"`
	Environment   string            `json:"environment,omitempty"`
	ServiceName   string            `json:"service_name,omitempty"`
	GenAI         *genAIMessage     `json:"gen_ai,omitempty"`
	Attributes    map[string]string `json:"attributes,omitempty"`
}

type genAIMessage struct {
	Model        string  `json:"model"`
	Provider     string  `json:"provider"`
	InputTokens  int64   `json:"input_tokens"`
	OutputTokens int64   `json:"output_tokens"`
	TotalTokens  int64   `json:"total_tokens"`
	CostUSD      float64 `json:"cost_usd"`
}

func spanToMessage(span storage.Span) spanMessage {
	msg := spanMessage{
		SpanID:        span.SpanID,
		TraceID:       span.TraceID,
		ParentSpanID:  span.ParentSpanID,
		Name:          span.Name,
		Kind:          int(span.Kind),
		Status:        int(span.Status),
		StatusMessage: span.StatusMessage,
		StartTime:     span.StartTime.Format("2006-01-02T15:04:05.999999999Z07:00"),
		EndTime:       span.EndTime.Format("2006-01-02T15:04:05.999999999Z07:00"),
		DurationNs:    span.Duration.Nanoseconds(),
		ProjectID:     span.ProjectID,
		Environment:   span.Environment,
		ServiceName:   span.ServiceName,
		Attributes:    span.Attributes,
	}

	if span.GenAI != nil {
		msg.GenAI = &genAIMessage{
			Model:        span.GenAI.Model,
			Provider:     span.GenAI.Provider,
			InputTokens:  span.GenAI.InputTokens,
			OutputTokens: span.GenAI.OutputTokens,
			TotalTokens:  span.GenAI.TotalTokens,
			CostUSD:      span.GenAI.CostUSD,
		}
	}

	return msg
}

// IngestSpans publishes each span as a JSON message to Pub/Sub.
// Messages are published asynchronously and results are collected.
func (w *Writer) IngestSpans(ctx context.Context, spans []storage.Span) error {
	if len(spans) == 0 {
		return nil
	}

	var (
		mu     sync.Mutex
		errors []error
		wg     sync.WaitGroup
	)

	for _, span := range spans {
		data, err := json.Marshal(spanToMessage(span))
		if err != nil {
			return fmt.Errorf("pubsub: marshaling span %s: %w", span.SpanID, err)
		}

		result := w.topic.Publish(ctx, &pubsub.Message{
			Data: data,
			Attributes: map[string]string{
				"trace_id":   span.TraceID,
				"span_id":    span.SpanID,
				"project_id": span.ProjectID,
			},
		})

		wg.Add(1)
		go func(spanID string) {
			defer wg.Done()
			if _, err := result.Get(ctx); err != nil {
				mu.Lock()
				errors = append(errors, fmt.Errorf("span %s: %w", spanID, err))
				mu.Unlock()
			}
		}(span.SpanID)
	}

	wg.Wait()

	if len(errors) > 0 {
		return fmt.Errorf("pubsub: %d publish errors (first: %w)", len(errors), errors[0])
	}

	return nil
}

// Ping verifies the topic still exists.
func (w *Writer) Ping(ctx context.Context) error {
	exists, err := w.topic.Exists(ctx)
	if err != nil {
		return fmt.Errorf("pubsub ping: %w", err)
	}
	if !exists {
		return fmt.Errorf("pubsub: topic %q no longer exists", w.config.TopicID)
	}
	return nil
}

// Close flushes pending messages and closes the client.
func (w *Writer) Close() error {
	w.topic.Stop()
	return w.client.Close()
}
