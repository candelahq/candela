package hubbleaudit

import (
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func TestNewGRPCFlowSource_EmptyAddr(t *testing.T) {
	_, err := NewGRPCFlowSource(GRPCFlowSourceConfig{})
	if err == nil {
		t.Fatal("expected error for empty address")
	}
}

func TestNewGRPCFlowSource_ValidAddr(t *testing.T) {
	src, err := NewGRPCFlowSource(GRPCFlowSourceConfig{
		Addr:     "localhost:4245",
		DialOpts: []grpc.DialOption{grpc.WithTransportCredentials(insecure.NewCredentials())},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer func() { _ = src.Close() }()

	if src.Addr() != "localhost:4245" {
		t.Errorf("addr = %q, want localhost:4245", src.Addr())
	}
}

func TestGRPCFlowSource_HealthDefault(t *testing.T) {
	src, err := NewGRPCFlowSource(GRPCFlowSourceConfig{Addr: "localhost:4245"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer func() { _ = src.Close() }()

	h := src.Health()
	if h.Connected {
		t.Error("expected connected=false by default")
	}
	if !h.LastFlowAt.IsZero() {
		t.Error("expected zero LastFlowAt by default")
	}
}

func TestGRPCFlowSource_SetConnected(t *testing.T) {
	src, err := NewGRPCFlowSource(GRPCFlowSourceConfig{Addr: "localhost:4245"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer func() { _ = src.Close() }()

	src.SetConnected(true)
	if !src.Health().Connected {
		t.Error("expected connected=true after SetConnected(true)")
	}

	src.SetConnected(false)
	if src.Health().Connected {
		t.Error("expected connected=false after SetConnected(false)")
	}
}

func TestRetryConfig_Defaults(t *testing.T) {
	rc := RetryConfig{}.withDefaults()

	if rc.InitialDelay != 1_000_000_000 { // 1s
		t.Errorf("InitialDelay = %v", rc.InitialDelay)
	}
	if rc.MaxDelay != 30_000_000_000 { // 30s
		t.Errorf("MaxDelay = %v", rc.MaxDelay)
	}
	if rc.Multiplier != 2.0 {
		t.Errorf("Multiplier = %v", rc.Multiplier)
	}
}

func TestBackoff_ExponentialGrowth(t *testing.T) {
	rc := RetryConfig{}.withDefaults()

	d0 := backoff(0, rc)
	d1 := backoff(1, rc)
	d2 := backoff(2, rc)

	// With ±25% jitter, d1 should generally be > d0 and d2 > d1.
	// We use a generous range to account for jitter.
	if d0 < 500_000_000 || d0 > 1_500_000_000 {
		t.Errorf("attempt 0 backoff = %v, want ~1s ±25%%", d0)
	}
	if d1 < 1_000_000_000 || d1 > 3_000_000_000 {
		t.Errorf("attempt 1 backoff = %v, want ~2s ±25%%", d1)
	}
	if d2 < 2_000_000_000 || d2 > 6_000_000_000 {
		t.Errorf("attempt 2 backoff = %v, want ~4s ±25%%", d2)
	}
}

func TestBackoff_MaxCap(t *testing.T) {
	rc := RetryConfig{}.withDefaults()

	d := backoff(100, rc) // Very high attempt — should be capped.
	if d > rc.MaxDelay+rc.MaxDelay/4 {
		t.Errorf("attempt 100 backoff = %v, exceeds max %v + 25%% jitter", d, rc.MaxDelay)
	}
}
