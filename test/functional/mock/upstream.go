// mock/upstream.go — Minimal mock LLM server for Candela functional tests.
//
// Mimics the response shapes of OpenAI, Anthropic (direct), and Vertex AI
// so Hurl tests can run without real API keys.
//
// Usage:
//
//	go run test/functional/mock/upstream.go [--port 9999]
//
// Endpoints:
//
//	POST /v1/chat/completions                                       → OpenAI chat response
//	POST /v1/messages                                               → Anthropic messages response
//	POST /v1/projects/*/locations/*/publishers/anthropic/models/*:rawPredict        → Vertex AI response
//	POST /v1/projects/*/locations/*/publishers/anthropic/models/*:streamRawPredict  → Vertex AI streaming SSE
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"
)

func main() {
	port := flag.Int("port", 9999, "port to listen on")
	flag.Parse()

	mux := http.NewServeMux()
	mux.HandleFunc("/", handler)

	addr := fmt.Sprintf(":%d", *port)
	log.Printf("🤖 mock LLM upstream listening on %s", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatal(err)
	}
}

func handler(w http.ResponseWriter, r *http.Request) {
	log.Printf("%s %s", r.Method, r.URL.Path)

	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	path := r.URL.Path

	switch {
	case path == "/v1/chat/completions":
		// OpenAI Chat Completions
		serveOpenAI(w, r)

	case path == "/v1/messages":
		// Anthropic Messages (direct)
		serveAnthropic(w, r)

	case strings.Contains(path, "/publishers/anthropic/models/") && strings.HasSuffix(path, ":rawPredict"):
		// Vertex AI rawPredict (non-streaming)
		serveAnthropic(w, r)

	case strings.Contains(path, "/publishers/anthropic/models/") && strings.HasSuffix(path, ":streamRawPredict"):
		// Vertex AI streamRawPredict (SSE)
		serveAnthropicStream(w, r)

	default:
		http.Error(w, fmt.Sprintf("unknown mock endpoint: %s", path), http.StatusNotFound)
	}
}

func serveOpenAI(w http.ResponseWriter, r *http.Request) {
	// Extract model from request body so we echo it back.
	var req struct {
		Model string `json:"model"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	if req.Model == "" {
		req.Model = "gpt-4o"
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"id":      "chatcmpl-mock123",
		"object":  "chat.completion",
		"created": time.Now().Unix(),
		"model":   req.Model,
		"choices": []map[string]any{
			{
				"index": 0,
				"message": map[string]string{
					"role":    "assistant",
					"content": "Hello from the mock upstream!",
				},
				"finish_reason": "stop",
			},
		},
		"usage": map[string]any{
			"prompt_tokens":     15,
			"completion_tokens": 8,
			"total_tokens":      23,
		},
	})
}

func serveAnthropic(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"id":   "msg_mock123",
		"type": "message",
		"role": "assistant",
		"content": []map[string]any{
			{"type": "text", "text": "Hello from the mock upstream!"},
		},
		"stop_reason": "end_turn",
		"usage": map[string]any{
			"input_tokens":  15,
			"output_tokens": 8,
		},
	})
}

func serveAnthropicStream(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	events := []string{
		`data: {"type":"message_start","message":{"id":"msg_mock123","type":"message","role":"assistant","content":[],"usage":{"input_tokens":15,"output_tokens":0}}}`,
		`data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`,
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Hello from "}}`,
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"the mock upstream!"}}`,
		`data: {"type":"content_block_stop","index":0}`,
		`data: {"type":"message_delta","delta":{"stop_reason":"end_turn","stop_sequence":null},"usage":{"output_tokens":8}}`,
		`data: {"type":"message_stop"}`,
	}

	for _, event := range events {
		_, _ = fmt.Fprintf(w, "%s\n\n", event)
		flusher.Flush()
		time.Sleep(5 * time.Millisecond)
	}
}
