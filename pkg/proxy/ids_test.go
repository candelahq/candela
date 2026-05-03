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

// ── parseTraceparent tests ──

func TestParseTraceparent_Valid(t *testing.T) {
	tc := parseTraceparent("00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01")
	if tc == nil {
		t.Fatal("expected non-nil traceContext for valid header")
	}
	if tc.traceID != "4bf92f3577b34da6a3ce929d0e0e4736" {
		t.Errorf("traceID = %q, want %q", tc.traceID, "4bf92f3577b34da6a3ce929d0e0e4736")
	}
	if tc.parentSpanID != "00f067aa0ba902b7" {
		t.Errorf("parentSpanID = %q, want %q", tc.parentSpanID, "00f067aa0ba902b7")
	}
}

func TestParseTraceparent_NotSampled(t *testing.T) {
	tc := parseTraceparent("00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-00")
	if tc == nil {
		t.Fatal("expected non-nil traceContext even when not sampled")
	}
	if tc.traceID != "4bf92f3577b34da6a3ce929d0e0e4736" {
		t.Errorf("traceID = %q", tc.traceID)
	}
}

func TestParseTraceparent_EmptyString(t *testing.T) {
	if tc := parseTraceparent(""); tc != nil {
		t.Error("expected nil for empty string")
	}
}

func TestParseTraceparent_Malformed(t *testing.T) {
	cases := []struct {
		name  string
		input string
	}{
		{"too few parts", "00-abc-def"},
		{"short trace ID", "00-abc123-00f067aa0ba902b7-01"},
		{"short span ID", "00-4bf92f3577b34da6a3ce929d0e0e4736-abc-01"},
		{"short flags", "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-1"},
		{"long version", "000-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01"},
		{"garbage", "not-a-valid-traceparent-header"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if result := parseTraceparent(tc.input); result != nil {
				t.Errorf("parseTraceparent(%q) = %+v, want nil", tc.input, result)
			}
		})
	}
}

func TestParseTraceparent_AllZeroTraceID(t *testing.T) {
	// All-zero trace-id is invalid per W3C spec.
	if tc := parseTraceparent("00-00000000000000000000000000000000-00f067aa0ba902b7-01"); tc != nil {
		t.Error("expected nil for all-zero trace-id")
	}
}

func TestParseTraceparent_AllZeroSpanID(t *testing.T) {
	// All-zero parent-id is invalid per W3C spec.
	if tc := parseTraceparent("00-4bf92f3577b34da6a3ce929d0e0e4736-0000000000000000-01"); tc != nil {
		t.Error("expected nil for all-zero span-id")
	}
}

func TestParseTraceparent_FutureVersion(t *testing.T) {
	// Future versions (e.g. "01") should still parse — the spec requires
	// forward compatibility for 2-char version strings.
	tc := parseTraceparent("01-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01")
	if tc == nil {
		t.Fatal("expected non-nil for future version")
	}
}

func TestParseTraceparent_VersionFF_Rejected(t *testing.T) {
	// Version "ff" is explicitly invalid per W3C Trace Context spec.
	if tc := parseTraceparent("ff-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01"); tc != nil {
		t.Error("expected nil for version ff")
	}
}

// ── Security tests ──

func TestParseTraceparent_NonHexCharacters(t *testing.T) {
	// Non-hex characters in trace/span IDs could be used for injection
	// attacks when stored in databases or logged.
	cases := []struct {
		name  string
		input string
	}{
		{"trace ID with special chars", "00-4bf92f3577b34da6a3ce929d0e0e47zz-00f067aa0ba902b7-01"},
		{"span ID with special chars", "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba9zzzz-01"},
		{"SQL injection in trace ID", "00-4bf92f3577b34da6a3ce929d0e' OR -00f067aa0ba902b7-01"},
		{"null bytes in span ID", "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa\x00ba902b-01"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if result := parseTraceparent(tc.input); result != nil {
				t.Errorf("accepted non-hex input %q", tc.input)
			}
		})
	}
}

func TestParseTraceparent_CaseNormalization(t *testing.T) {
	// W3C spec requires case-insensitive parsing. Without normalization,
	// "AA..." and "aa..." would create separate traces for the same flow.
	tc := parseTraceparent("00-4BF92F3577B34DA6A3CE929D0E0E4736-00F067AA0BA902B7-01")
	if tc == nil {
		t.Fatal("expected non-nil for uppercase input")
	}
	if tc.traceID != "4bf92f3577b34da6a3ce929d0e0e4736" {
		t.Errorf("traceID not normalized to lowercase: got %q", tc.traceID)
	}
	if tc.parentSpanID != "00f067aa0ba902b7" {
		t.Errorf("parentSpanID not normalized to lowercase: got %q", tc.parentSpanID)
	}
}

func TestParseTraceparent_MixedCase(t *testing.T) {
	tc := parseTraceparent("00-4Bf92f3577B34da6A3ce929D0e0e4736-00F067aA0bA902B7-01")
	if tc == nil {
		t.Fatal("expected non-nil for mixed-case input")
	}
	if tc.traceID != "4bf92f3577b34da6a3ce929d0e0e4736" {
		t.Errorf("traceID not normalized: got %q", tc.traceID)
	}
}
