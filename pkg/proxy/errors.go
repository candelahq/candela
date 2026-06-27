package proxy

import (
	"encoding/json"
	"net/http"
	"strconv"
)

// openAIErrorResponse is the top-level error envelope matching the OpenAI API format.
type openAIErrorResponse struct {
	Error openAIErrorDetail `json:"error"`
}

// openAIErrorDetail holds the error payload fields.
type openAIErrorDetail struct {
	Message string `json:"message"`
	Type    string `json:"type"`
	Code    string `json:"code"`
}

// ProxyErrorResponse writes an OpenAI-format JSON error response.
//
//	{"error":{"message":"...","type":"...","code":"400"}}
//
// This ensures all proxy error responses are machine-parseable JSON with
// a consistent structure, matching what OpenAI-compatible clients expect.
// Exported so cmd/candela (lm_handler) can reuse the same format.
func ProxyErrorResponse(w http.ResponseWriter, status int, message, errorType string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(openAIErrorResponse{
		Error: openAIErrorDetail{
			Message: message,
			Type:    errorType,
			Code:    strconv.Itoa(status),
		},
	})
}
