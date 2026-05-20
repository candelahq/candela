package processor

import "time"

// SinkHealth reports the health status of a single storage sink.
// Used for observability dashboards and health check endpoints.
type SinkHealth struct {
	Name          string     `json:"name"`
	State         string     `json:"state"` // "closed", "open", "half-open"
	TotalWrites   int64      `json:"total_writes"`
	TotalFailures int64      `json:"total_failures"`
	TotalDropped  int64      `json:"total_dropped"` // Dropped due to bulkhead saturation
	LastSuccess   *time.Time `json:"last_success,omitempty"`
	LastFailure   *time.Time `json:"last_failure,omitempty"`
}
