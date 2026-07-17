package notify

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/candelahq/candela/pkg/storage"
)

// testAlert returns a BudgetAlert with the given threshold for testing.
func testAlert(threshold float64) storage.BudgetAlert {
	return storage.BudgetAlert{
		UserID:    "user-123",
		Email:     "alice@example.com",
		Threshold: threshold,
		SpentUSD:  80.00,
		LimitUSD:  100.00,
		PeriodKey: "2026-07",
		SentAt:    time.Date(2026, 7, 16, 12, 0, 0, 0, time.UTC),
	}
}

func TestSlackNotifier_PayloadStructure(t *testing.T) {
	var received slackPayload
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("expected Content-Type application/json, got %s", ct)
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		if err := json.Unmarshal(body, &received); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	n := NewSlackNotifier(srv.URL)
	alert := testAlert(0.8)

	if err := n.NotifyBudgetThreshold(context.Background(), alert); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if received.Text == "" {
		t.Fatal("expected non-empty text payload")
	}
	if !strings.Contains(received.Text, "alice@example.com") {
		t.Error("payload missing user email")
	}
	if !strings.Contains(received.Text, "$80.00") {
		t.Error("payload missing spent amount")
	}
	if !strings.Contains(received.Text, "$100.00") {
		t.Error("payload missing limit amount")
	}
	if !strings.Contains(received.Text, "2026-07") {
		t.Error("payload missing period key")
	}
	if !strings.Contains(received.Text, "80%") {
		t.Error("payload missing threshold percentage")
	}
}

func TestSlackNotifier_ThresholdEmoji(t *testing.T) {
	tests := []struct {
		name      string
		threshold float64
		emoji     string
	}{
		{"80% warning", 0.8, "⚠️"},
		{"90% caution", 0.9, "🔶"},
		{"100% critical", 1.0, "🚨"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var received slackPayload
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, _ := io.ReadAll(r.Body)
				if err := json.Unmarshal(body, &received); err != nil {
					t.Errorf("unmarshal: %v", err)
				}
				w.WriteHeader(http.StatusOK)
			}))
			defer srv.Close()

			n := NewSlackNotifier(srv.URL)
			alert := testAlert(tt.threshold)

			if err := n.NotifyBudgetThreshold(context.Background(), alert); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if !strings.Contains(received.Text, tt.emoji) {
				t.Errorf("expected emoji %s in payload, got: %s", tt.emoji, received.Text)
			}
		})
	}
}

func TestSlackNotifier_NonOKResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	n := NewSlackNotifier(srv.URL)
	err := n.NotifyBudgetThreshold(context.Background(), testAlert(0.8))

	if err == nil {
		t.Fatal("expected error for non-200 response")
	}
	if !strings.Contains(err.Error(), "HTTP 403") {
		t.Errorf("expected HTTP 403 in error, got: %v", err)
	}
}

func TestSlackNotifier_ContextCancellation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Block until client cancels — avoids goroutine leak from hard sleep.
		select {
		case <-r.Context().Done():
		case <-time.After(5 * time.Second):
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	n := NewSlackNotifier(srv.URL)
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately.

	err := n.NotifyBudgetThreshold(ctx, testAlert(0.8))
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
}

func TestSlackNotifier_Timeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(500 * time.Millisecond):
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	n := NewSlackNotifier(srv.URL)
	// Override client with a very short timeout.
	n.client = &http.Client{Timeout: 50 * time.Millisecond}

	err := n.NotifyBudgetThreshold(context.Background(), testAlert(0.8))
	if err == nil {
		t.Fatal("expected timeout error")
	}
}

func TestThresholdEmoji(t *testing.T) {
	tests := []struct {
		threshold float64
		want      string
	}{
		{0.5, "⚠️"},
		{0.8, "⚠️"},
		{0.89, "⚠️"},
		{0.9, "🔶"},
		{0.99, "🔶"},
		{1.0, "🚨"},
		{1.5, "🚨"},
	}
	for _, tt := range tests {
		got := thresholdEmoji(tt.threshold)
		if got != tt.want {
			t.Errorf("thresholdEmoji(%.2f) = %s, want %s", tt.threshold, got, tt.want)
		}
	}
}
