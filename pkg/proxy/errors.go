package proxy

import (
	"encoding/json"
	"net/http"
)

// ProxyErrorResponse writes an OpenAI-format JSON error response.
//
//	{"error":{"message":"...","type":"...","code":400}}
//
// This ensures all proxy error responses are machine-parseable JSON with
// a consistent structure, matching what OpenAI-compatible clients expect.
// Exported so cmd/candela (lm_handler) can reuse the same format.
func ProxyErrorResponse(w http.ResponseWriter, status int, message, errorType string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	body, _ := json.Marshal(map[string]any{
		"error": map[string]any{
			"message": message,
			"type":    errorType,
			"code":    status,
		},
	})
	_, _ = w.Write(body)
}
