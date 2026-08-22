package spendoutbox

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

// UserStore is the subset of storage.UserStore needed for spend retry.
type UserStore interface {
	DeductSpend(ctx context.Context, userID string, costUSD float64, tokens int64) error
	DeductTaskSpend(ctx context.Context, taskID string, costUSD float64) error
}

// SpendSyncWorker periodically retries failed DeductSpend calls stored
// in the spend outbox. It follows the same lifecycle pattern as
// pkg/proxy/worker.SyncWorker.
type SpendSyncWorker struct {
	outbox   *Outbox
	users    UserStore
	interval time.Duration
	maxAge   time.Duration
	done     chan struct{}
	wg       sync.WaitGroup
	ctx      context.Context
	cancel   context.CancelFunc
}

// NewSpendSyncWorker creates a new background worker for retrying failed
// spend deductions. Defaults: interval 10s, maxAge 24h.
func NewSpendSyncWorker(outbox *Outbox, users UserStore, interval time.Duration) *SpendSyncWorker {
	if interval <= 0 {
		interval = 10 * time.Second
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &SpendSyncWorker{
		outbox:   outbox,
		users:    users,
		interval: interval,
		maxAge:   24 * time.Hour,
		done:     make(chan struct{}),
		ctx:      ctx,
		cancel:   cancel,
	}
}

// Start begins the periodic retry loop.
func (w *SpendSyncWorker) Start() {
	w.wg.Add(1)
	go w.syncLoop()
	slog.Info("SpendSyncWorker started", "interval", w.interval)
}

// Stop gracefully shuts down the worker.
func (w *SpendSyncWorker) Stop() {
	w.cancel()
	close(w.done)
	w.wg.Wait()
	slog.Info("SpendSyncWorker stopped")
}

func (w *SpendSyncWorker) syncLoop() {
	defer w.wg.Done()
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	for {
		select {
		case <-w.done:
			return
		case <-ticker.C:
			w.processRetries()
		}
	}
}

func (w *SpendSyncWorker) processRetries() {
	ctx, cancel := context.WithTimeout(w.ctx, 30*time.Second)
	defer cancel()

	records, err := w.outbox.Peek(ctx, 100)
	if err != nil {
		slog.Error("SpendSyncWorker failed to peek outbox", "error", err)
		return
	}
	if len(records) == 0 {
		return
	}

	now := time.Now()
	var deleteIDs []string

	for _, rec := range records {
		// Permanent failure: too old.
		if now.Sub(rec.CreatedAt) > w.maxAge {
			deleteIDs = append(deleteIDs, rec.ID)
			slog.Error("SpendSyncWorker permanent failure, dropping record",
				"id", rec.ID,
				"user_id", rec.UserID,
				"cost_usd", rec.CostUSD,
				"tokens", rec.Tokens,
				"attempt_count", rec.AttemptCount,
				"age", now.Sub(rec.CreatedAt).String(),
			)
			continue
		}

		// Attempt task budget deduction first (best-effort).
		if rec.TaskID != "" {
			if taskErr := w.users.DeductTaskSpend(ctx, rec.TaskID, rec.CostUSD); taskErr != nil {
				slog.Warn("spend_outbox: task deduction failed (best-effort)",
					"id", rec.ID,
					"task_id", rec.TaskID,
					"cost_usd", rec.CostUSD,
					"error", taskErr)
			}
		}

		// Attempt the DeductSpend retry for user budget.
		if err := w.users.DeductSpend(ctx, rec.UserID, rec.CostUSD, rec.Tokens); err != nil {
			if retryErr := w.outbox.RetryLater(ctx, rec.ID, rec.AttemptCount); retryErr != nil {
				slog.Error("SpendSyncWorker failed to schedule retry", "error", retryErr)
			}
			slog.Warn("spend_outbox: retry scheduled",
				"id", rec.ID,
				"user_id", rec.UserID,
				"task_id", rec.TaskID,
				"cost_usd", rec.CostUSD,
				"attempt", rec.AttemptCount+1,
				"next_retry_in", backoffDelay(rec.AttemptCount),
			)
		} else {
			deleteIDs = append(deleteIDs, rec.ID)
		}
	}

	if len(deleteIDs) > 0 {
		if err := w.outbox.Delete(ctx, deleteIDs); err != nil {
			slog.Error("SpendSyncWorker failed to delete records", "error", err)
		}
	}
}
