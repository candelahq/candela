package proxy

import (
	"testing"
)

// ── Tests for audit v2 fixes ──

func TestGenerateTraceID_Length(t *testing.T) {
	id := generateTraceID()
	if len(id) != 32 {
		t.Errorf("generateTraceID() length = %d, want 32", len(id))
	}
}

func TestGenerateSpanID_Length(t *testing.T) {
	id := generateSpanID()
	if len(id) != 16 {
		t.Errorf("generateSpanID() length = %d, want 16", len(id))
	}
}

func TestGenerateIDs_Unique(t *testing.T) {
	seen := make(map[string]bool, 1000)
	for range 1000 {
		id := generateTraceID()
		if seen[id] {
			t.Fatalf("collision detected after <1000 IDs: %s", id)
		}
		seen[id] = true
	}
}
