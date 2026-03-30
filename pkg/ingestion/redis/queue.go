// Package redis implements the ingestion.Queue interface using Redis streams.
package redis

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/candelahq/candela/pkg/ingestion"
	"github.com/candelahq/candela/pkg/storage"
	goredis "github.com/redis/go-redis/v9"
)

const (
	streamKey     = "candela:spans"
	consumerGroup = "candela-workers"
)

// Queue implements ingestion.Queue using Redis Streams.
type Queue struct {
	client     *goredis.Client
	consumerID string
}

var _ ingestion.Queue = (*Queue)(nil)

// New creates a new Redis-backed queue.
func New(addr string, consumerID string) (*Queue, error) {
	client := goredis.NewClient(&goredis.Options{
		Addr: addr,
	})

	q := &Queue{
		client:     client,
		consumerID: consumerID,
	}

	// Create the consumer group if it doesn't exist.
	ctx := context.Background()
	err := client.XGroupCreateMkStream(ctx, streamKey, consumerGroup, "0").Err()
	if err != nil && err.Error() != "BUSYGROUP Consumer Group name already exists" {
		return nil, fmt.Errorf("creating consumer group: %w", err)
	}

	return q, nil
}

func (q *Queue) Push(ctx context.Context, spans []storage.Span) error {
	pipe := q.client.Pipeline()
	for _, span := range spans {
		data, err := json.Marshal(span)
		if err != nil {
			return fmt.Errorf("marshalling span: %w", err)
		}
		pipe.XAdd(ctx, &goredis.XAddArgs{
			Stream: streamKey,
			Values: map[string]interface{}{
				"data": string(data),
			},
		})
	}
	_, err := pipe.Exec(ctx)
	return err
}

func (q *Queue) Pull(ctx context.Context, batchSize int) ([]storage.Span, error) {
	results, err := q.client.XReadGroup(ctx, &goredis.XReadGroupArgs{
		Group:    consumerGroup,
		Consumer: q.consumerID,
		Streams:  []string{streamKey, ">"},
		Count:    int64(batchSize),
		Block:    0, // Block until messages are available
	}).Result()
	if err != nil {
		return nil, fmt.Errorf("reading from stream: %w", err)
	}

	var spans []storage.Span
	var ackIDs []string
	for _, stream := range results {
		for _, msg := range stream.Messages {
			data, ok := msg.Values["data"].(string)
			if !ok {
				continue
			}
			var span storage.Span
			if err := json.Unmarshal([]byte(data), &span); err != nil {
				continue // Skip malformed spans — TODO: dead-letter queue
			}
			spans = append(spans, span)
			ackIDs = append(ackIDs, msg.ID)
		}
	}

	// Acknowledge processed messages.
	if len(ackIDs) > 0 {
		q.client.XAck(ctx, streamKey, consumerGroup, ackIDs...)
	}

	return spans, nil
}

func (q *Queue) Ping(ctx context.Context) error {
	return q.client.Ping(ctx).Err()
}

func (q *Queue) Close() error {
	return q.client.Close()
}
