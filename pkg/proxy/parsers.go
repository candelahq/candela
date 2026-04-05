package proxy

import (
	"encoding/json"
	"strings"
)

// ProviderParser extracts LLM request/response data for a specific provider.
// Implement this interface to add support for a new LLM provider.
type ProviderParser interface {
	// IsStreaming returns true if the request body indicates streaming.
	IsStreaming(body []byte) bool

	// ParseRequest extracts the model name and input content from a request body.
	ParseRequest(body []byte) (model, content string)

	// ParseResponse extracts output content and token usage from a standard response.
	ParseResponse(body []byte) (content string, inputTokens, outputTokens int64)

	// ParseStreamingResponse extracts output content and token usage from SSE stream data.
	ParseStreamingResponse(data []byte) (content string, inputTokens, outputTokens int64)
}

// parserRegistry maps provider names to their parsers.
var parserRegistry = map[string]ProviderParser{
	"openai":     &openaiParser{},
	"gemini-oai": &openaiParser{}, // Gemini OpenAI-compat returns standard OpenAI format.
	"anthropic":  &anthropicParser{},
	"google":     &googleParser{},
}

// getParser returns the parser for a provider, or a no-op fallback.
func getParser(provider string) ProviderParser {
	if p, ok := parserRegistry[provider]; ok {
		return p
	}
	return &fallbackParser{}
}

// ──────────────────────────────────────────
// OpenAI
// ──────────────────────────────────────────

type openaiParser struct{}

func (p *openaiParser) IsStreaming(body []byte) bool {
	var req map[string]interface{}
	if err := json.Unmarshal(body, &req); err != nil {
		return false
	}
	v, _ := req["stream"].(bool)
	return v
}

func (p *openaiParser) ParseRequest(body []byte) (model, content string) {
	var req map[string]interface{}
	if err := json.Unmarshal(body, &req); err != nil {
		return "", ""
	}
	model, _ = req["model"].(string)
	if messages, ok := req["messages"].([]interface{}); ok {
		b, _ := json.Marshal(messages)
		content = string(b)
	}
	return
}

func (p *openaiParser) ParseResponse(body []byte) (content string, inputTokens, outputTokens int64) {
	var resp map[string]interface{}
	if err := json.Unmarshal(body, &resp); err != nil {
		return "", 0, 0
	}

	if usage, ok := resp["usage"].(map[string]interface{}); ok {
		inputTokens = toInt64(usage["prompt_tokens"])
		outputTokens = toInt64(usage["completion_tokens"])
	}
	if choices, ok := resp["choices"].([]interface{}); ok && len(choices) > 0 {
		if choice, ok := choices[0].(map[string]interface{}); ok {
			if msg, ok := choice["message"].(map[string]interface{}); ok {
				content, _ = msg["content"].(string)
			}
		}
	}
	return
}

func (p *openaiParser) ParseStreamingResponse(data []byte) (content string, inputTokens, outputTokens int64) {
	var contentBuilder strings.Builder

	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		payload := strings.TrimPrefix(line, "data: ")
		if payload == "[DONE]" {
			continue
		}

		var chunk map[string]interface{}
		if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
			continue
		}

		if choices, ok := chunk["choices"].([]interface{}); ok && len(choices) > 0 {
			if choice, ok := choices[0].(map[string]interface{}); ok {
				if delta, ok := choice["delta"].(map[string]interface{}); ok {
					if c, ok := delta["content"].(string); ok {
						contentBuilder.WriteString(c)
					}
				}
			}
		}
		if usage, ok := chunk["usage"].(map[string]interface{}); ok {
			inputTokens = toInt64(usage["prompt_tokens"])
			outputTokens = toInt64(usage["completion_tokens"])
		}
	}

	content = contentBuilder.String()
	return
}

// ──────────────────────────────────────────
// Anthropic
// ──────────────────────────────────────────

type anthropicParser struct{}

func (p *anthropicParser) IsStreaming(body []byte) bool {
	var req map[string]interface{}
	if err := json.Unmarshal(body, &req); err != nil {
		return false
	}
	v, _ := req["stream"].(bool)
	return v
}

func (p *anthropicParser) ParseRequest(body []byte) (model, content string) {
	var req map[string]interface{}
	if err := json.Unmarshal(body, &req); err != nil {
		return "", ""
	}
	model, _ = req["model"].(string)
	if messages, ok := req["messages"].([]interface{}); ok {
		b, _ := json.Marshal(messages)
		content = string(b)
	}
	return
}

func (p *anthropicParser) ParseResponse(body []byte) (content string, inputTokens, outputTokens int64) {
	var resp map[string]interface{}
	if err := json.Unmarshal(body, &resp); err != nil {
		return "", 0, 0
	}

	if usage, ok := resp["usage"].(map[string]interface{}); ok {
		inputTokens = toInt64(usage["input_tokens"])
		outputTokens = toInt64(usage["output_tokens"])
	}
	if contentArr, ok := resp["content"].([]interface{}); ok && len(contentArr) > 0 {
		if block, ok := contentArr[0].(map[string]interface{}); ok {
			content, _ = block["text"].(string)
		}
	}
	return
}

func (p *anthropicParser) ParseStreamingResponse(data []byte) (content string, inputTokens, outputTokens int64) {
	var contentBuilder strings.Builder

	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		payload := strings.TrimPrefix(line, "data: ")
		if payload == "[DONE]" {
			continue
		}

		var chunk map[string]interface{}
		if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
			continue
		}

		if delta, ok := chunk["delta"].(map[string]interface{}); ok {
			if text, ok := delta["text"].(string); ok {
				contentBuilder.WriteString(text)
			}
		}
		if usage, ok := chunk["usage"].(map[string]interface{}); ok {
			if v := toInt64(usage["output_tokens"]); v > 0 {
				outputTokens = v
			}
		}
		if msg, ok := chunk["message"].(map[string]interface{}); ok {
			if usage, ok := msg["usage"].(map[string]interface{}); ok {
				inputTokens = toInt64(usage["input_tokens"])
			}
		}
	}

	content = contentBuilder.String()
	return
}

// ──────────────────────────────────────────
// Google (Vertex AI / Gemini)
// ──────────────────────────────────────────

type googleParser struct{}

func (p *googleParser) IsStreaming(_ []byte) bool {
	// Google uses a different endpoint for streaming, not a body param.
	return false
}

func (p *googleParser) ParseRequest(body []byte) (model, content string) {
	var req map[string]interface{}
	if err := json.Unmarshal(body, &req); err != nil {
		return "", ""
	}
	// Model is in the URL path for Google, not the body.
	if contents, ok := req["contents"].([]interface{}); ok {
		b, _ := json.Marshal(contents)
		content = string(b)
	}
	return
}

func (p *googleParser) ParseResponse(body []byte) (content string, inputTokens, outputTokens int64) {
	var resp map[string]interface{}
	if err := json.Unmarshal(body, &resp); err != nil {
		return "", 0, 0
	}

	if meta, ok := resp["usageMetadata"].(map[string]interface{}); ok {
		inputTokens = toInt64(meta["promptTokenCount"])
		outputTokens = toInt64(meta["candidatesTokenCount"])
	}
	if candidates, ok := resp["candidates"].([]interface{}); ok && len(candidates) > 0 {
		if c, ok := candidates[0].(map[string]interface{}); ok {
			if cont, ok := c["content"].(map[string]interface{}); ok {
				if parts, ok := cont["parts"].([]interface{}); ok && len(parts) > 0 {
					if part, ok := parts[0].(map[string]interface{}); ok {
						content, _ = part["text"].(string)
					}
				}
			}
		}
	}
	return
}

func (p *googleParser) ParseStreamingResponse(data []byte) (content string, inputTokens, outputTokens int64) {
	// Google streaming uses a different endpoint — not SSE-based in the same way.
	// Fall back to standard parsing of the accumulated data.
	return p.ParseResponse(data)
}

// ──────────────────────────────────────────
// Fallback (unknown providers)
// ──────────────────────────────────────────

type fallbackParser struct{}

func (p *fallbackParser) IsStreaming(_ []byte) bool                     { return false }
func (p *fallbackParser) ParseRequest(_ []byte) (string, string)        { return "", "" }
func (p *fallbackParser) ParseResponse(_ []byte) (string, int64, int64) { return "", 0, 0 }
func (p *fallbackParser) ParseStreamingResponse(_ []byte) (string, int64, int64) {
	return "", 0, 0
}
