package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/candelahq/candela/pkg/storage"
)

// SlackNotifier sends budget alerts to a Slack channel via incoming webhook.
type SlackNotifier struct {
	WebhookURL string
	client     *http.Client
}

// NewSlackNotifier creates a notifier that posts to the given Slack webhook URL.
func NewSlackNotifier(webhookURL string) *SlackNotifier {
	return &SlackNotifier{
		WebhookURL: webhookURL,
		client:     &http.Client{Timeout: 10 * time.Second},
	}
}

// slackPayload is the JSON body sent to the Slack webhook.
type slackPayload struct {
	Text string `json:"text"`
}

// thresholdEmoji returns an emoji appropriate for the alert severity.
func thresholdEmoji(threshold float64) string {
	switch {
	case threshold >= 1.0:
		return "🚨"
	case threshold >= 0.9:
		return "🔶"
	default:
		return "⚠️"
	}
}

// NotifyBudgetThreshold posts a budget alert message to Slack.
func (n *SlackNotifier) NotifyBudgetThreshold(ctx context.Context, alert storage.BudgetAlert) error {
	emoji := thresholdEmoji(alert.Threshold)
	pct := int(alert.Threshold * 100)

	text := fmt.Sprintf(
		"%s *Budget Alert — %d%% threshold reached*\n"+
			"• *User:* %s\n"+
			"• *Spent:* $%.2f / $%.2f\n"+
			"• *Period:* %s",
		emoji, pct, alert.Email, alert.SpentUSD, alert.LimitUSD, alert.PeriodKey,
	)

	payload := slackPayload{Text: text}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("slack: marshal payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, n.WebhookURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("slack: create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := n.client.Do(req)
	if err != nil {
		return fmt.Errorf("slack: send request: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("slack: webhook returned HTTP %d", resp.StatusCode)
	}

	return nil
}
