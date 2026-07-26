package storage_test

import (
	"testing"
	"time"

	"github.com/candelahq/candela/pkg/storage"
)

func TestPageCursor_RoundTrip(t *testing.T) {
	original := storage.PageCursor{
		Timestamp: time.Date(2026, 7, 26, 10, 0, 0, 0, time.UTC),
		ID:        "trace-abc-123",
	}
	token := storage.EncodePageCursor(original)
	if token == "" {
		t.Fatal("expected non-empty token")
	}

	decoded, err := storage.DecodePageCursor(token)
	if err != nil {
		t.Fatalf("decode failed: %v", err)
	}
	if !decoded.Timestamp.Equal(original.Timestamp) {
		t.Errorf("timestamp mismatch: %v != %v", decoded.Timestamp, original.Timestamp)
	}
	if decoded.ID != original.ID {
		t.Errorf("ID mismatch: %q != %q", decoded.ID, original.ID)
	}
}

func TestPageCursor_EmptyToken(t *testing.T) {
	c, err := storage.DecodePageCursor("")
	if err != nil {
		t.Fatalf("empty token should not error: %v", err)
	}
	if c.ID != "" {
		t.Errorf("expected empty ID for empty token, got %q", c.ID)
	}
}

func TestPageCursor_InvalidToken(t *testing.T) {
	_, err := storage.DecodePageCursor("not-valid-base64!!!")
	if err == nil {
		t.Fatal("expected error for invalid token")
	}

	_, err = storage.DecodePageCursor("bm90LWpzb24") // base64 of "not-json"
	if err == nil {
		t.Fatal("expected error for non-JSON token")
	}
}
