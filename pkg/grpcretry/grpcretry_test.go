package grpcretry

import (
	"context"
	"errors"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestDo(t *testing.T) {
	transientErrs := []error{
		status.Error(codes.Unavailable, "unavailable"),
		status.Error(codes.DeadlineExceeded, "deadline"),
		status.Error(codes.ResourceExhausted, "exhausted"),
	}
	permanentErrs := []error{
		status.Error(codes.InvalidArgument, "invalid"),
		status.Error(codes.NotFound, "not found"),
		status.Error(codes.PermissionDenied, "denied"),
	}

	tests := []struct {
		name        string
		config      Config
		setupCtx    func() (context.Context, context.CancelFunc)
		mockInvoker func() func(context.Context) error
		wantErr     error
		wantCalls   int
	}{
		{
			name:   "success on first try",
			config: Config{MaxAttempts: 3},
			setupCtx: func() (context.Context, context.CancelFunc) {
				return context.WithTimeout(context.Background(), 5*time.Second)
			},
			mockInvoker: func() func(context.Context) error {
				return func(ctx context.Context) error {
					return nil
				}
			},
			wantErr:   nil,
			wantCalls: 1,
		},
		{
			name:   "success after transient errors",
			config: Config{MaxAttempts: 3, InitialDelay: time.Millisecond},
			setupCtx: func() (context.Context, context.CancelFunc) {
				return context.WithTimeout(context.Background(), 5*time.Second)
			},
			mockInvoker: func() func(context.Context) error {
				calls := 0
				return func(ctx context.Context) error {
					calls++
					if calls <= 2 {
						return transientErrs[0]
					}
					return nil
				}
			},
			wantErr:   nil,
			wantCalls: 3,
		},
		{
			name:   "exhaust max attempts",
			config: Config{MaxAttempts: 2, InitialDelay: time.Millisecond},
			setupCtx: func() (context.Context, context.CancelFunc) {
				return context.WithTimeout(context.Background(), 5*time.Second)
			},
			mockInvoker: func() func(context.Context) error {
				return func(ctx context.Context) error {
					return transientErrs[1]
				}
			},
			wantErr:   transientErrs[1],
			wantCalls: 3, // Initial call (0) + 2 retries
		},
		{
			name:   "no retry on permanent error",
			config: Config{MaxAttempts: 3},
			setupCtx: func() (context.Context, context.CancelFunc) {
				return context.WithTimeout(context.Background(), 5*time.Second)
			},
			mockInvoker: func() func(context.Context) error {
				calls := 0
				return func(ctx context.Context) error {
					calls++
					if calls == 1 {
						return transientErrs[0]
					}
					return permanentErrs[0]
				}
			},
			wantErr:   permanentErrs[0],
			wantCalls: 2,
		},
		{
			name:   "zero retries config",
			config: Config{MaxAttempts: 0},
			setupCtx: func() (context.Context, context.CancelFunc) {
				return context.WithTimeout(context.Background(), 5*time.Second)
			},
			mockInvoker: func() func(context.Context) error {
				return func(ctx context.Context) error {
					return transientErrs[2]
				}
			},
			wantErr:   transientErrs[2],
			wantCalls: 1,
		},
		{
			name:   "context cancellation stops retries",
			config: Config{MaxAttempts: 3, InitialDelay: time.Millisecond},
			setupCtx: func() (context.Context, context.CancelFunc) {
				ctx, cancel := context.WithCancel(context.Background())
				// Cancel immediately so the first retry fails to wait
				cancel()
				return ctx, cancel
			},
			mockInvoker: func() func(context.Context) error {
				return func(ctx context.Context) error {
					return transientErrs[0]
				}
			},
			wantErr:   context.Canceled,
			wantCalls: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := tt.setupCtx()
			defer cancel()

			calls := 0
			mockFn := tt.mockInvoker()
			operation := func(ctx context.Context) error {
				calls++
				return mockFn(ctx)
			}

			err := Do(ctx, tt.config, operation)
			if !errors.Is(err, tt.wantErr) && err != tt.wantErr {
				t.Errorf("Do() error = %v, wantErr %v", err, tt.wantErr)
			}
			if calls != tt.wantCalls {
				t.Errorf("Do() calls = %d, wantCalls %d", calls, tt.wantCalls)
			}
		})
	}
}
