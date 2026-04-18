package runtime

import (
	"context"
	"fmt"
	"net/http"
	"time"
)

// WaitHealthy polls the given URL until it returns HTTP 200 or the timeout
// expires. This is shared across all runtime implementations that need to
// wait for a server to become ready after launch.
func WaitHealthy(ctx context.Context, url string, interval, timeout time.Duration) error {
	deadline := time.After(timeout)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline:
			return fmt.Errorf("timed out waiting for %s to be healthy", url)
		case <-ticker.C:
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
			if err != nil {
				continue
			}
			resp, err := http.DefaultClient.Do(req)
			if err == nil {
				_ = resp.Body.Close()
				if resp.StatusCode == http.StatusOK {
					return nil
				}
			}
		}
	}
}

// DefaultHost returns the host or "127.0.0.1" if empty.
func DefaultHost(host string) string {
	if host == "" {
		return "127.0.0.1"
	}
	return host
}

// DefaultPort returns the port or the fallback if zero.
func DefaultPort(port, fallback int) int {
	if port == 0 {
		return fallback
	}
	return port
}

// ConfigBinary extracts the "binary" string from Args, or returns the fallback.
func ConfigBinary(args map[string]any, fallback string) string {
	if b, ok := args["binary"].(string); ok && b != "" {
		return b
	}
	return fallback
}

// ConfigString extracts a string value from Args by key, converting numeric
// types (int, float64) to their string representation. This handles YAML
// values that may be parsed as either strings or numbers depending on
// whether they are quoted (e.g., gpu_memory_utilization: 0.9 vs "0.9").
func ConfigString(args map[string]any, key string) (string, bool) {
	v, ok := args[key]
	if !ok {
		return "", false
	}
	switch val := v.(type) {
	case string:
		return val, val != ""
	case int, int64, float64:
		return fmt.Sprint(val), true
	default:
		return "", false
	}
}
