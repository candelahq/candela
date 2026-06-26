package proxy

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestProxyErrorResponse(t *testing.T) {
	tests := []struct {
		name      string
		status    int
		message   string
		errorType string
	}{
		{
			name:      "bad request",
			status:    http.StatusBadRequest,
			message:   "invalid proxy path",
			errorType: "invalid_request_error",
		},
		{
			name:      "internal server error",
			status:    http.StatusInternalServerError,
			message:   "failed to create upstream request",
			errorType: "server_error",
		},
		{
			name:      "bad gateway",
			status:    http.StatusBadGateway,
			message:   "upstream provider unavailable",
			errorType: "upstream_error",
		},
		{
			name:      "service unavailable",
			status:    http.StatusServiceUnavailable,
			message:   "rate limit check unavailable — try again shortly",
			errorType: "service_error",
		},
		{
			name:      "entity too large",
			status:    http.StatusRequestEntityTooLarge,
			message:   "request body too large",
			errorType: "invalid_request_error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			ProxyErrorResponse(w, tt.status, tt.message, tt.errorType)

			// Verify status code.
			if w.Code != tt.status {
				t.Errorf("status = %d, want %d", w.Code, tt.status)
			}

			// Verify Content-Type is application/json.
			ct := w.Header().Get("Content-Type")
			if ct != "application/json" {
				t.Errorf("Content-Type = %q, want \"application/json\"", ct)
			}

			// Verify body is valid JSON in OpenAI format.
			var parsed struct {
				Error struct {
					Message string `json:"message"`
					Type    string `json:"type"`
					Code    int    `json:"code"`
				} `json:"error"`
			}
			if err := json.Unmarshal(w.Body.Bytes(), &parsed); err != nil {
				t.Fatalf("body is not valid JSON: %v\nbody: %s", err, w.Body.String())
			}

			if parsed.Error.Message != tt.message {
				t.Errorf("error.message = %q, want %q", parsed.Error.Message, tt.message)
			}
			if parsed.Error.Type != tt.errorType {
				t.Errorf("error.type = %q, want %q", parsed.Error.Type, tt.errorType)
			}
			if parsed.Error.Code != tt.status {
				t.Errorf("error.code = %d, want %d", parsed.Error.Code, tt.status)
			}
		})
	}
}
