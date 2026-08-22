package billing

import (
	"testing"
	"time"
)

func TestTaskBudget_Remaining(t *testing.T) {
	tests := []struct {
		name    string
		limit   float64
		spent   float64
		wantRem float64
	}{
		{"unspent", 10.0, 0.0, 10.0},
		{"partial", 10.0, 3.5, 6.5},
		{"fully spent", 10.0, 10.0, 0.0},
		{"overspent", 10.0, 12.0, 0.0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := &TaskBudget{LimitUSD: tt.limit, SpentUSD: tt.spent}
			if got := b.Remaining(); got != tt.wantRem {
				t.Errorf("Remaining() = %v, want %v", got, tt.wantRem)
			}
		})
	}
}

func TestTaskBudget_IsExpired(t *testing.T) {
	tests := []struct {
		name      string
		expiresAt time.Time
		want      bool
	}{
		{"zero time (no expiry)", time.Time{}, false},
		{"future", time.Now().UTC().Add(1 * time.Hour), false},
		{"past", time.Now().UTC().Add(-1 * time.Hour), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := &TaskBudget{ExpiresAt: tt.expiresAt}
			if got := b.IsExpired(); got != tt.want {
				t.Errorf("IsExpired() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestTaskBudget_Validate(t *testing.T) {
	tests := []struct {
		name    string
		budget  TaskBudget
		wantErr bool
	}{
		{
			name:    "valid",
			budget:  TaskBudget{TaskID: "job-123", LimitUSD: 5.0, SpentUSD: 0},
			wantErr: false,
		},
		{
			name:    "missing task_id",
			budget:  TaskBudget{LimitUSD: 5.0},
			wantErr: true,
		},
		{
			name:    "zero limit",
			budget:  TaskBudget{TaskID: "job-123", LimitUSD: 0},
			wantErr: true,
		},
		{
			name:    "negative limit",
			budget:  TaskBudget{TaskID: "job-123", LimitUSD: -1.0},
			wantErr: true,
		},
		{
			name:    "negative spent",
			budget:  TaskBudget{TaskID: "job-123", LimitUSD: 5.0, SpentUSD: -0.5},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.budget.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
