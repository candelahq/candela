package notify

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"math/rand"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/candelahq/candela/pkg/storage"
)

// WebhookConfig configures the webhook notifier.
type WebhookConfig struct {
	Enabled  bool     `yaml:"enabled"`
	Endpoint string   `yaml:"endpoint"`
	Secret   string   `yaml:"secret"` // HMAC-SHA256 key; supports ${ENV_VAR} expansion
	Events   []string `yaml:"events"` // e.g. ["budget.exhausted", "budget.threshold"]
}

// WebhookNotifier sends budget alerts to an external HTTP endpoint with
// HMAC-SHA256 request signing and retry with exponential backoff.
//
// It implements storage.Notifier so it plugs directly into the existing
// BudgetChecker notification pipeline alongside LogNotifier and SlackNotifier.
type WebhookNotifier struct {
	endpoint string
	secret   []byte // empty = no signing
	events   map[string]bool
	client   *http.Client
}

// webhookPayload is the JSON body sent to the webhook endpoint.
type webhookPayload struct {
	Event     string  `json:"event"`
	UserID    string  `json:"user_id"`
	Email     string  `json:"email,omitempty"`
	TaskID    string  `json:"task_id,omitempty"`
	BudgetUSD float64 `json:"budget_usd"`
	SpentUSD  float64 `json:"spent_usd"`
	Threshold float64 `json:"threshold"`
	PeriodKey string  `json:"period_key,omitempty"`
	Timestamp string  `json:"timestamp"`
}

// NewWebhookNotifier creates a notifier that POSTs JSON events to the
// configured endpoint. The secret supports ${ENV_VAR} expansion for
// Kubernetes-style secret injection.
func NewWebhookNotifier(cfg WebhookConfig) *WebhookNotifier {
	secret := expandEnvVars(cfg.Secret)

	events := make(map[string]bool, len(cfg.Events))
	for _, e := range cfg.Events {
		events[e] = true
	}

	return &WebhookNotifier{
		endpoint: cfg.Endpoint,
		secret:   []byte(secret),
		events:   events,
		client:   &http.Client{Timeout: 10 * time.Second},
	}
}

// NotifyBudgetThreshold sends a webhook event for the given budget alert.
// Events are filtered by the configured event types list. If the event type
// is not in the list, the notification is silently skipped.
func (n *WebhookNotifier) NotifyBudgetThreshold(ctx context.Context, alert storage.BudgetAlert) error {
	eventType := n.resolveEventType(alert)
	if eventType == "" {
		return nil // no matching event type configured
	}

	payload := webhookPayload{
		Event:     eventType,
		UserID:    alert.UserID,
		Email:     alert.Email,
		TaskID:    alert.TaskID,
		BudgetUSD: alert.LimitUSD,
		SpentUSD:  alert.SpentUSD,
		Threshold: alert.Threshold,
		PeriodKey: alert.PeriodKey,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("webhook: marshal payload: %w", err)
	}

	return n.sendWithRetry(ctx, body)
}

// resolveEventType maps a BudgetAlert to the most specific matching event
// type from the configured events list. Task-scoped events take precedence.
func (n *WebhookNotifier) resolveEventType(alert storage.BudgetAlert) string {
	isExhausted := alert.Threshold >= 1.0
	hasTask := alert.TaskID != ""

	// Try most specific first, then fall back to general.
	candidates := make([]string, 0, 2)
	if hasTask {
		if isExhausted {
			candidates = append(candidates, "task.budget.exhausted")
		} else {
			candidates = append(candidates, "task.budget.threshold")
		}
	}
	if isExhausted {
		candidates = append(candidates, "budget.exhausted")
	} else {
		candidates = append(candidates, "budget.threshold")
	}

	// If no events filter is configured, accept all.
	if len(n.events) == 0 {
		if len(candidates) > 0 {
			return candidates[0]
		}
		return ""
	}

	for _, c := range candidates {
		if n.events[c] {
			return c
		}
	}
	return ""
}

// sendWithRetry delivers the webhook payload with up to 3 attempts using
// exponential backoff (1s, 2s, 4s) with ±25% jitter.
func (n *WebhookNotifier) sendWithRetry(ctx context.Context, body []byte) error {
	const maxAttempts = 3

	var lastErr error
	for attempt := 0; attempt < maxAttempts; attempt++ {
		if attempt > 0 {
			backoff := time.Duration(1<<uint(attempt-1)) * time.Second
			jitter := time.Duration(float64(backoff) * (0.75 + rand.Float64()*0.5))
			select {
			case <-ctx.Done():
				return fmt.Errorf("webhook: context cancelled during retry: %w", ctx.Err())
			case <-time.After(jitter):
			}
		}

		lastErr = n.send(ctx, body)
		if lastErr == nil {
			return nil
		}
		slog.Warn("webhook: delivery attempt failed",
			"attempt", attempt+1,
			"max_attempts", maxAttempts,
			"endpoint", n.endpoint,
			"error", lastErr)
	}
	return fmt.Errorf("webhook: all %d attempts exhausted: %w", maxAttempts, lastErr)
}

// send performs a single HTTP POST with optional HMAC-SHA256 signing.
func (n *WebhookNotifier) send(ctx context.Context, body []byte) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, n.endpoint, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("webhook: create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Candela-Webhook/1.0")

	// HMAC-SHA256 signing.
	if len(n.secret) > 0 {
		mac := hmac.New(sha256.New, n.secret)
		mac.Write(body)
		sig := hex.EncodeToString(mac.Sum(nil))
		req.Header.Set("X-Candela-Signature", "sha256="+sig)
	}

	resp, err := n.client.Do(req)
	if err != nil {
		return fmt.Errorf("webhook: send request: %w", err)
	}
	defer func() {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("webhook: endpoint returned HTTP %d: %s",
			resp.StatusCode, string(bytes.TrimSpace(bodyBytes)))
	}

	return nil
}

// expandEnvVars expands ${VAR_NAME} references in a string using os.Getenv.
// This supports Kubernetes-style secret injection where the config file
// contains "${WEBHOOK_SECRET}" and the actual value is in an env var.
func expandEnvVars(s string) string {
	return os.Expand(s, os.Getenv)
}

// Ensure WebhookNotifier implements the Notifier interface at compile time.
// This uses storage.Notifier (which is an alias for billing.Notifier) to
// match the type used by BudgetChecker.
var _ storage.Notifier = (*WebhookNotifier)(nil)

// redactEndpoint returns a redacted version of the endpoint URL for logging,
// showing only the host portion.
func redactEndpoint(endpoint string) string {
	if idx := strings.Index(endpoint, "://"); idx >= 0 {
		rest := endpoint[idx+3:]
		if slashIdx := strings.Index(rest, "/"); slashIdx >= 0 {
			return endpoint[:idx+3] + rest[:slashIdx] + "/***"
		}
	}
	return endpoint
}

// RedactedEndpoint returns the endpoint with the path redacted for safe logging.
func (n *WebhookNotifier) RedactedEndpoint() string {
	return redactEndpoint(n.endpoint)
}
