package firestoredb

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"cloud.google.com/go/firestore"
	"github.com/candelahq/candela/pkg/billing"
	"github.com/candelahq/candela/pkg/storage"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Firestore collection and document constants for task budgets.
const (
	tasksCol        = "tasks"
	taskBudgetDocID = "budget"
)

// CreateTaskBudget creates an ephemeral budget for a single agent task.
// The budget document is stored at tasks/{taskID}/budget.
// Returns an error if a budget already exists for this task.
func (s *Store) CreateTaskBudget(ctx context.Context, budget *storage.TaskBudget) error {
	if budget == nil {
		return fmt.Errorf("firestoredb: CreateTaskBudget: budget is nil")
	}
	if err := budget.Validate(); err != nil {
		return fmt.Errorf("firestoredb: CreateTaskBudget: %w", err)
	}

	now := time.Now().UTC()
	if budget.CreatedAt.IsZero() {
		budget.CreatedAt = now
	}

	ref := s.taskBudgetRef(budget.TaskID)
	_, err := ref.Create(ctx, map[string]any{
		"task_id":    budget.TaskID,
		"user_id":    budget.UserID,
		"limit_usd":  budget.LimitUSD,
		"spent_usd":  0.0,
		"created_at": budget.CreatedAt,
		"expires_at": budget.ExpiresAt,
	})
	if err != nil {
		if status.Code(err) == codes.AlreadyExists {
			return fmt.Errorf("firestoredb: CreateTaskBudget: budget already exists for task %q", budget.TaskID)
		}
		return fmt.Errorf("firestoredb: CreateTaskBudget: %w", err)
	}
	return nil
}

// GetTaskBudget returns the task budget for a given task ID.
func (s *Store) GetTaskBudget(ctx context.Context, taskID string) (*storage.TaskBudget, error) {
	if taskID == "" {
		return nil, fmt.Errorf("firestoredb: GetTaskBudget: task_id is required")
	}

	ref := s.taskBudgetRef(taskID)
	snap, err := ref.Get(ctx)
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return nil, storage.ErrNotFound
		}
		return nil, fmt.Errorf("firestoredb: GetTaskBudget: %w", err)
	}
	return snapToTaskBudget(snap)
}

// DeleteTaskBudget removes a task budget document.
func (s *Store) DeleteTaskBudget(ctx context.Context, taskID string) error {
	if taskID == "" {
		return fmt.Errorf("firestoredb: DeleteTaskBudget: task_id is required")
	}

	ref := s.taskBudgetRef(taskID)
	_, err := ref.Delete(ctx)
	if err != nil {
		return fmt.Errorf("firestoredb: DeleteTaskBudget: %w", err)
	}
	return nil
}

// CheckTaskBudget evaluates whether an estimated cost is within the task's budget.
// This is a read-only pre-flight check.
func (s *Store) CheckTaskBudget(ctx context.Context, taskID string, estimatedCostUSD float64) (*billing.TaskBudgetCheckResult, error) {
	if taskID == "" {
		return nil, fmt.Errorf("firestoredb: CheckTaskBudget: task_id is required")
	}

	budget, err := s.GetTaskBudget(ctx, taskID)
	if err != nil {
		return nil, fmt.Errorf("firestoredb: CheckTaskBudget: %w", err)
	}

	// Check expiry.
	if budget.IsExpired() {
		return &billing.TaskBudgetCheckResult{
			Allowed:      false,
			Reason:       billing.ReasonTaskBudgetExpired,
			RemainingUSD: 0,
			LimitUSD:     budget.LimitUSD,
			SpentUSD:     budget.SpentUSD,
		}, nil
	}

	remaining := budget.Remaining()
	if remaining < estimatedCostUSD {
		return &billing.TaskBudgetCheckResult{
			Allowed:      false,
			Reason:       billing.ReasonTaskBudgetExhausted,
			RemainingUSD: remaining,
			LimitUSD:     budget.LimitUSD,
			SpentUSD:     budget.SpentUSD,
		}, nil
	}

	return &billing.TaskBudgetCheckResult{
		Allowed:      true,
		RemainingUSD: remaining,
		LimitUSD:     budget.LimitUSD,
		SpentUSD:     budget.SpentUSD,
	}, nil
}

// DeductTaskSpend atomically increments the spent_usd on a task budget.
// Uses a Firestore transaction to prevent concurrent spend races.
func (s *Store) DeductTaskSpend(ctx context.Context, taskID string, costUSD float64) error {
	if taskID == "" {
		return fmt.Errorf("firestoredb: DeductTaskSpend: task_id is required")
	}
	if costUSD < 0 {
		return fmt.Errorf("firestoredb: DeductTaskSpend: cost must be non-negative, got %.6f", costUSD)
	}
	if costUSD == 0 {
		return nil
	}

	now := time.Now().UTC()
	txCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	ref := s.taskBudgetRef(taskID)

	return s.client.RunTransaction(txCtx, func(_ context.Context, tx *firestore.Transaction) error {
		snap, err := tx.Get(ref)
		if err != nil {
			if status.Code(err) == codes.NotFound {
				return fmt.Errorf("firestoredb: DeductTaskSpend: no budget for task %q", taskID)
			}
			return fmt.Errorf("firestoredb: DeductTaskSpend: read: %w", err)
		}

		budget, err := snapToTaskBudget(snap)
		if err != nil {
			return err
		}

		// Check expiry — don't deduct from expired budgets.
		if !budget.ExpiresAt.IsZero() && now.After(budget.ExpiresAt) {
			slog.Warn("firestoredb: DeductTaskSpend: task budget expired",
				"task_id", taskID,
				"expires_at", budget.ExpiresAt,
				"cost_usd", costUSD)
			return fmt.Errorf("firestoredb: DeductTaskSpend: task budget expired for %q", taskID)
		}

		newSpent := budget.SpentUSD + costUSD
		if err := tx.Update(ref, []firestore.Update{
			{Path: "spent_usd", Value: newSpent},
		}); err != nil {
			return fmt.Errorf("firestoredb: DeductTaskSpend: update: %w", err)
		}

		return nil
	})
}

// taskBudgetRef returns the Firestore document reference for a task's budget.
// Path: tasks/{taskID}/budget
func (s *Store) taskBudgetRef(taskID string) *firestore.DocumentRef {
	return s.client.Collection(tasksCol).Doc(taskID).Collection(budgetsCol).Doc(taskBudgetDocID)
}

// snapToTaskBudget converts a Firestore document snapshot to a TaskBudget.
func snapToTaskBudget(snap *firestore.DocumentSnapshot) (*storage.TaskBudget, error) {
	data := snap.Data()
	b := &storage.TaskBudget{
		TaskID:   stringField(data, "task_id"),
		UserID:   stringField(data, "user_id"),
		LimitUSD: firestoreFloat(data["limit_usd"]),
		SpentUSD: firestoreFloat(data["spent_usd"]),
	}
	if t, ok := data["created_at"].(time.Time); ok {
		b.CreatedAt = t
	}
	if t, ok := data["expires_at"].(time.Time); ok {
		b.ExpiresAt = t
	}
	return b, nil
}

// stringField safely extracts a string value from Firestore data.
func stringField(data map[string]any, key string) string {
	if v, ok := data[key].(string); ok {
		return v
	}
	return ""
}
