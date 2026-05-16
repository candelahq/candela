package worker

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/candelahq/candela/pkg/storage"
)

// SyncWorker periodically pulls serialized spans from the local outbox
// and pushes them to the upstream proxy (Team Mode).
type SyncWorker struct {
	store     storage.SyncStore
	upstream  storage.SpanWriter
	interval  time.Duration
	keepCount int
	done      chan struct{}
	wg        sync.WaitGroup
	ctx       context.Context
	cancel    context.CancelFunc
}

// NewSyncWorker creates a new background worker for offline syncing.
func NewSyncWorker(store storage.SyncStore, upstream storage.SpanWriter, interval time.Duration) *SyncWorker {
	if interval <= 0 {
		interval = 5 * time.Second
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &SyncWorker{
		store:     store,
		upstream:  upstream,
		interval:  interval,
		keepCount: 100000,
		done:      make(chan struct{}),
		ctx:       ctx,
		cancel:    cancel,
	}
}

// Start begins the periodic sync and prune loops.
func (w *SyncWorker) Start() {
	w.wg.Add(2)
	go w.syncLoop()
	go w.pruneLoop()
	slog.Info("SyncWorker started", "interval", w.interval)
}

// Stop gracefully shuts down the worker.
func (w *SyncWorker) Stop() {
	w.cancel() // abort in-flight requests immediately
	close(w.done)
	w.wg.Wait()
	slog.Info("SyncWorker stopped")
}

func (w *SyncWorker) syncLoop() {
	defer w.wg.Done()
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	for {
		select {
		case <-w.done:
			return
		case <-ticker.C:
			w.processBatch()
		}
	}
}

func (w *SyncWorker) processBatch() {
	ctx, cancel := context.WithTimeout(w.ctx, 30*time.Second)
	defer cancel()

	// 1. Pull batch
	outboxSpans, err := w.store.GetOutboxSpans(ctx, 1000)
	if err != nil {
		slog.Error("SyncWorker failed to pull outbox", "error", err)
		return
	}
	if len(outboxSpans) == 0 {
		return // nothing to do
	}

	var spans []storage.Span
	var spanIDs []string
	var failedIDs []string
	var corruptedIDs []string

	// 2. Deserialize and flag retries
	for _, obs := range outboxSpans {
		var span storage.Span
		if err := json.Unmarshal([]byte(obs.PayloadJSON), &span); err != nil {
			slog.Error("SyncWorker failed to unmarshal outbox span, dropping", "span_id", obs.SpanID, "error", err)
			corruptedIDs = append(corruptedIDs, obs.SpanID)
			continue
		}

		// Optimistic Ingestion with Pessimistic Retries pattern.
		if obs.AttemptCount > 0 {
			if span.Attributes == nil {
				span.Attributes = make(map[string]string)
			}
			span.Attributes["candela.is_retry"] = "true"
		}

		spans = append(spans, span)
		spanIDs = append(spanIDs, obs.SpanID)
	}

	if len(corruptedIDs) > 0 {
		if err := w.store.DeleteOutboxSpans(ctx, corruptedIDs); err != nil {
			slog.Error("SyncWorker failed to delete corrupted outbox spans", "error", err)
		}
	}

	if len(spans) == 0 {
		return
	}

	// 3. Push to upstream with bisection for size errors
	failedIDs = w.pushWithBisection(ctx, spans, spanIDs)

	// 4. Transactional Commit or Retry
	if len(failedIDs) > 0 {
		// Increment attempts so the next run uses is_retry: true
		if err := w.store.IncrementOutboxAttempt(ctx, failedIDs); err != nil {
			slog.Error("SyncWorker failed to increment outbox attempt", "error", err)
		}

		// If some spans succeeded, we still want to delete them from the outbox
		// We can find the successful ones by subtracting failedIDs from spanIDs
		failedMap := make(map[string]bool)
		for _, id := range failedIDs {
			failedMap[id] = true
		}

		var successIDs []string
		for _, id := range spanIDs {
			if !failedMap[id] {
				successIDs = append(successIDs, id)
			}
		}

		if len(successIDs) > 0 {
			if err := w.store.DeleteOutboxSpans(ctx, successIDs); err != nil {
				slog.Error("SyncWorker failed to delete partially synced spans", "error", err)
			}
		}
	} else {
		// Success! Delete all from outbox.
		if err := w.store.DeleteOutboxSpans(ctx, spanIDs); err != nil {
			slog.Error("SyncWorker failed to delete synced spans", "error", err)
		} else {
			slog.Debug("SyncWorker successfully synced and deleted spans", "count", len(spanIDs))
		}
	}
}

// pushWithBisection attempts to push a batch of spans. If it encounters a payload size error,
// it bisects the batch and tries again, returning the IDs of spans that ultimately failed.
func (w *SyncWorker) pushWithBisection(ctx context.Context, spans []storage.Span, spanIDs []string) []string {
	if len(spans) == 0 {
		return nil
	}

	err := w.upstream.IngestSpans(ctx, spans)
	if err == nil {
		return nil // Success
	}

	errStr := err.Error()
	// Only bisect if the error suggests a payload size limit (HTTP 413 or gRPC ResourceExhausted)
	isSizeError := strings.Contains(errStr, "413") ||
		strings.Contains(errStr, "Too Large") ||
		strings.Contains(errStr, "too large") ||
		strings.Contains(errStr, "ResourceExhausted") ||
		strings.Contains(errStr, "message too large")

	if !isSizeError || len(spans) == 1 {
		slog.Warn("SyncWorker upstream push failed", "error", err, "count", len(spans))
		return spanIDs // All failed
	}

	slog.Warn("SyncWorker encountered payload size limit, bisecting batch", "count", len(spans), "error", err)

	mid := len(spans) / 2

	failedLeft := w.pushWithBisection(ctx, spans[:mid], spanIDs[:mid])
	failedRight := w.pushWithBisection(ctx, spans[mid:], spanIDs[mid:])

	return append(failedLeft, failedRight...)
}

func (w *SyncWorker) pruneLoop() {
	defer w.wg.Done()
	// Prune every 1 hour
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	// Initial prune on startup
	w.pruneLocal(w.ctx)

	for {
		select {
		case <-w.done:
			return
		case <-ticker.C:
			w.pruneLocal(w.ctx)
		}
	}
}

func (w *SyncWorker) pruneLocal(ctx context.Context) {
	// Retain only the most recent N local traces to save disk space on laptops
	if err := w.store.PruneLocalSpans(ctx, w.keepCount); err != nil {
		slog.Error("SyncWorker failed to prune local spans", "error", err)
	} else {
		slog.Debug("SyncWorker pruned local spans beyond limit", "keep_count", w.keepCount)
	}
}
