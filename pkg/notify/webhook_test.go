package notify

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/candelahq/candela/pkg/storage"
)

func TestWebhookNotifier_HappyPath(t *testing.T) {
	var received webhookPayload
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("expected Content-Type application/json, got %s", ct)
		}
		body, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(body, &received); err != nil {
			t.Errorf("failed to unmarshal payload: %v", err)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	n := NewWebhookNotifier(WebhookConfig{
		Enabled:  true,
		Endpoint: srv.URL,
		Events:   []string{"budget.exhausted"},
	})

	err := n.NotifyBudgetThreshold(context.Background(), storage.BudgetAlert{
		UserID:    "alice@example.com",
		Email:     "alice@example.com",
		Threshold: 1.0,
		SpentUSD:  100.50,
		LimitUSD:  100.00,
		PeriodKey: "2026-08-21",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if received.Event != "budget.exhausted" {
		t.Errorf("event = %q, want %q", received.Event, "budget.exhausted")
	}
	if received.UserID != "alice@example.com" {
		t.Errorf("user_id = %q, want %q", received.UserID, "alice@example.com")
	}
	if received.BudgetUSD != 100.00 {
		t.Errorf("budget_usd = %v, want 100.00", received.BudgetUSD)
	}
	if received.SpentUSD != 100.50 {
		t.Errorf("spent_usd = %v, want 100.50", received.SpentUSD)
	}
	if received.Timestamp == "" {
		t.Error("timestamp should not be empty")
	}
}

func TestWebhookNotifier_HMACSigning(t *testing.T) {
	secret := "test-webhook-secret-key"
	var receivedSig string
	var receivedBody []byte

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedSig = r.Header.Get("X-Candela-Signature")
		receivedBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	n := NewWebhookNotifier(WebhookConfig{
		Enabled:  true,
		Endpoint: srv.URL,
		Secret:   secret,
		Events:   []string{"budget.threshold"},
	})

	err := n.NotifyBudgetThreshold(context.Background(), storage.BudgetAlert{
		UserID:    "bob@example.com",
		Threshold: 0.9,
		SpentUSD:  90.00,
		LimitUSD:  100.00,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify HMAC signature.
	if receivedSig == "" {
		t.Fatal("X-Candela-Signature header missing")
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(receivedBody)
	expectedSig := "sha256=" + hex.EncodeToString(mac.Sum(nil))
	if receivedSig != expectedSig {
		t.Errorf("signature mismatch:\n  got:  %s\n  want: %s", receivedSig, expectedSig)
	}
}

func TestWebhookNotifier_NoSecret_NoSignature(t *testing.T) {
	var hasSig bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hasSig = r.Header.Get("X-Candela-Signature") != ""
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	n := NewWebhookNotifier(WebhookConfig{
		Enabled:  true,
		Endpoint: srv.URL,
		Events:   []string{"budget.exhausted"},
	})

	err := n.NotifyBudgetThreshold(context.Background(), storage.BudgetAlert{
		Threshold: 1.0,
		SpentUSD:  50,
		LimitUSD:  50,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if hasSig {
		t.Error("expected no X-Candela-Signature header when secret is empty")
	}
}

func TestWebhookNotifier_RetryOn500(t *testing.T) {
	var attempts atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := attempts.Add(1)
		if n < 3 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	n := NewWebhookNotifier(WebhookConfig{
		Enabled:  true,
		Endpoint: srv.URL,
		Events:   []string{"budget.exhausted"},
	})

	err := n.NotifyBudgetThreshold(context.Background(), storage.BudgetAlert{
		Threshold: 1.0,
		SpentUSD:  100,
		LimitUSD:  100,
	})
	if err != nil {
		t.Fatalf("expected success after retries, got: %v", err)
	}
	if got := attempts.Load(); got != 3 {
		t.Errorf("expected 3 attempts, got %d", got)
	}
}

func TestWebhookNotifier_AllRetriesExhausted(t *testing.T) {
	var attempts atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer srv.Close()

	n := NewWebhookNotifier(WebhookConfig{
		Enabled:  true,
		Endpoint: srv.URL,
		Events:   []string{"budget.exhausted"},
	})

	err := n.NotifyBudgetThreshold(context.Background(), storage.BudgetAlert{
		Threshold: 1.0,
		SpentUSD:  100,
		LimitUSD:  100,
	})
	if err == nil {
		t.Fatal("expected error after all retries exhausted")
	}
	if got := attempts.Load(); got != 3 {
		t.Errorf("expected 3 attempts, got %d", got)
	}
}

func TestWebhookNotifier_EventFiltering(t *testing.T) {
	tests := []struct {
		name      string
		events    []string
		alert     storage.BudgetAlert
		wantSkip  bool
		wantEvent string
	}{
		{
			name:   "exhausted event not in filter",
			events: []string{"budget.threshold"},
			alert: storage.BudgetAlert{
				Threshold: 1.0,
			},
			wantSkip: true,
		},
		{
			name:   "threshold event matches",
			events: []string{"budget.threshold"},
			alert: storage.BudgetAlert{
				Threshold: 0.9,
				SpentUSD:  90,
				LimitUSD:  100,
			},
			wantEvent: "budget.threshold",
		},
		{
			name:   "task exhausted takes precedence",
			events: []string{"task.budget.exhausted", "budget.exhausted"},
			alert: storage.BudgetAlert{
				TaskID:    "job-123",
				Threshold: 1.0,
				SpentUSD:  50,
				LimitUSD:  50,
			},
			wantEvent: "task.budget.exhausted",
		},
		{
			name:   "task alert falls back to general",
			events: []string{"budget.exhausted"},
			alert: storage.BudgetAlert{
				TaskID:    "job-123",
				Threshold: 1.0,
				SpentUSD:  50,
				LimitUSD:  50,
			},
			wantEvent: "budget.exhausted",
		},
		{
			name:   "empty filter accepts all",
			events: nil,
			alert: storage.BudgetAlert{
				Threshold: 0.8,
				SpentUSD:  80,
				LimitUSD:  100,
			},
			wantEvent: "budget.threshold",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var received webhookPayload
			var called bool
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				called = true
				body, _ := io.ReadAll(r.Body)
				_ = json.Unmarshal(body, &received)
				w.WriteHeader(http.StatusOK)
			}))
			defer srv.Close()

			n := NewWebhookNotifier(WebhookConfig{
				Enabled:  true,
				Endpoint: srv.URL,
				Events:   tt.events,
			})

			err := n.NotifyBudgetThreshold(context.Background(), tt.alert)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if tt.wantSkip {
				if called {
					t.Error("expected webhook to be skipped, but it was called")
				}
				return
			}

			if !called {
				t.Fatal("expected webhook to be called")
			}
			if received.Event != tt.wantEvent {
				t.Errorf("event = %q, want %q", received.Event, tt.wantEvent)
			}
		})
	}
}

func TestWebhookNotifier_TaskContext(t *testing.T) {
	var received webhookPayload
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &received)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	n := NewWebhookNotifier(WebhookConfig{
		Enabled:  true,
		Endpoint: srv.URL,
		Events:   []string{"task.budget.exhausted"},
	})

	err := n.NotifyBudgetThreshold(context.Background(), storage.BudgetAlert{
		UserID:    "alice@example.com",
		TaskID:    "my-experiment-42",
		Threshold: 1.0,
		SpentUSD:  5.01,
		LimitUSD:  5.00,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if received.TaskID != "my-experiment-42" {
		t.Errorf("task_id = %q, want %q", received.TaskID, "my-experiment-42")
	}
	if received.Event != "task.budget.exhausted" {
		t.Errorf("event = %q, want %q", received.Event, "task.budget.exhausted")
	}
}

func TestRedactEndpoint(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"http://diverge-controller:8080/webhooks/candela", "http://diverge-controller:8080/***"},
		{"https://example.com/hook", "https://example.com/***"},
		{"https://example.com", "https://example.com"},
		{"not-a-url", "not-a-url"},
	}
	for _, tt := range tests {
		if got := redactEndpoint(tt.input); got != tt.want {
			t.Errorf("redactEndpoint(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
