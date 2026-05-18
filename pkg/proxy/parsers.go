package proxy

import (
	"bytes"
	"encoding/json"
	"strings"
)

// NOTE: All cache normalization has been moved to costcalc.Calculator.
// Parsers return RAW token counts; the proxy applies model-aware and
// provider-aware cache discounts at the call site via
// Calculator.NormalizeCachedInput(provider, model, rawInput, cacheRead, cacheCreate).

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
	"openai":           &openaiParser{},
	"gemini-oai":       &openaiParser{}, // Gemini OpenAI-compat returns standard OpenAI format.
	"anthropic":        &anthropicParser{},
	"anthropic-direct": &anthropicParser{}, // Same wire format, just no Vertex AI translation.
	"anthropic-vertex": &anthropicParser{}, // Native Anthropic format routed via Vertex AI.
	"google":           &googleParser{},
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
		// Return RAW prompt_tokens — cache normalization is applied at
		// the call site via Calculator.NormalizeCachedInput.
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
			// Return RAW prompt_tokens — same as ParseResponse.
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
		// Return RAW input_tokens — cache normalization is applied at
		// the call site via Calculator.NormalizeCachedInput.
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
				// Return RAW input_tokens — same as ParseResponse.
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
		// Return RAW promptTokenCount — cache normalization is applied at
		// the call site (createSpan/deductBudget) where the model is known,
		// since Gemini 2.5+ and 2.0 have different cache discount rates.
		inputTokens = toInt64(meta["promptTokenCount"])
		// Gemini 2.5 "thinking" models report reasoning tokens separately.
		// These are billed at the output rate but NOT included in candidatesTokenCount.
		// Without this, thinking-heavy responses undercount output by 2-10×.
		outputTokens = toInt64(meta["candidatesTokenCount"]) +
			toInt64(meta["thoughtsTokenCount"])
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
	// Google streaming (streamGenerateContent) returns newline-delimited JSON
	// objects, not SSE. The usage metadata appears in the LAST chunk only.
	// Try parsing the accumulated data as a single response first (works if
	// only one chunk was buffered), then fall back to scanning for the last
	// usageMetadata block in the concatenated stream.
	content, inputTokens, outputTokens = p.ParseResponse(data)
	if inputTokens > 0 || outputTokens > 0 {
		return
	}

	// Google streaming (streamGenerateContent) returns a JSON array:
	// [{chunk1},{chunk2},...] where objects can span multiple lines.
	// Use json.Decoder to correctly parse multi-line JSON objects.
	var lastMeta map[string]interface{}
	var contentBuilder strings.Builder

	// First try: parse as a JSON array of objects.
	var chunks []map[string]interface{}
	if err := json.Unmarshal(data, &chunks); err == nil && len(chunks) > 0 {
		for _, chunk := range chunks {
			if meta, ok := chunk["usageMetadata"].(map[string]interface{}); ok {
				lastMeta = meta
			}
			if candidates, ok := chunk["candidates"].([]interface{}); ok && len(candidates) > 0 {
				if c, ok := candidates[0].(map[string]interface{}); ok {
					if cont, ok := c["content"].(map[string]interface{}); ok {
						if parts, ok := cont["parts"].([]interface{}); ok && len(parts) > 0 {
							if part, ok := parts[0].(map[string]interface{}); ok {
								if text, ok := part["text"].(string); ok {
									contentBuilder.WriteString(text)
								}
							}
						}
					}
				}
			}
		}
	} else {
		// Fallback: try json.Decoder for newline-delimited JSON objects.
		dec := json.NewDecoder(bytes.NewReader(data))
		for dec.More() {
			var chunk map[string]interface{}
			if err := dec.Decode(&chunk); err != nil {
				break
			}
			if meta, ok := chunk["usageMetadata"].(map[string]interface{}); ok {
				lastMeta = meta
			}
			if candidates, ok := chunk["candidates"].([]interface{}); ok && len(candidates) > 0 {
				if c, ok := candidates[0].(map[string]interface{}); ok {
					if cont, ok := c["content"].(map[string]interface{}); ok {
						if parts, ok := cont["parts"].([]interface{}); ok && len(parts) > 0 {
							if part, ok := parts[0].(map[string]interface{}); ok {
								if text, ok := part["text"].(string); ok {
									contentBuilder.WriteString(text)
								}
							}
						}
					}
				}
			}
		}
	}

	content = contentBuilder.String()
	if lastMeta != nil {
		// Return RAW promptTokenCount — same rationale as ParseResponse.
		inputTokens = toInt64(lastMeta["promptTokenCount"])
		outputTokens = toInt64(lastMeta["candidatesTokenCount"]) +
			toInt64(lastMeta["thoughtsTokenCount"])
	}
	return
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

// ──────────────────────────────────────────
// Model extraction from response body
// ──────────────────────────────────────────

// extractModelFromResponse extracts the model name from a provider's response
// body. This is the primary source for Google (which has modelVersion in the
// response but NOT in the request body), and a fallback for other providers.
func extractModelFromResponse(provider string, body []byte) string {
	var resp map[string]interface{}
	if err := json.Unmarshal(body, &resp); err != nil {
		return ""
	}

	switch provider {
	case "google":
		// Google returns modelVersion at the response top level.
		// e.g. "gemini-2.0-flash-lite-001" or "gemini-2.5-pro-preview-05-06"
		if mv, ok := resp["modelVersion"].(string); ok && mv != "" {
			return mv
		}
	case "openai", "gemini-oai":
		if m, ok := resp["model"].(string); ok && m != "" {
			return m
		}
	case "anthropic", "anthropic-direct", "anthropic-vertex":
		if m, ok := resp["model"].(string); ok && m != "" {
			return m
		}
	}
	return ""
}

// extractModelFromStreamingResponse extracts the model name from streaming
// response data. Scans SSE lines for the model field in any chunk.
func extractModelFromStreamingResponse(provider string, data []byte) string {
	switch provider {
	case "google":
		// Google streaming: look for modelVersion in any JSON chunk.
		// It's typically in the last chunk alongside usageMetadata.
		var arr []map[string]interface{}
		if json.Unmarshal(data, &arr) == nil {
			for _, chunk := range arr {
				if mv, ok := chunk["modelVersion"].(string); ok && mv != "" {
					return mv
				}
			}
		}
		// Try as single JSON object.
		var single map[string]interface{}
		if json.Unmarshal(data, &single) == nil {
			if mv, ok := single["modelVersion"].(string); ok && mv != "" {
				return mv
			}
		}

	default:
		// OpenAI/Anthropic SSE: scan for "model" in any data line.
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
			if json.Unmarshal([]byte(payload), &chunk) == nil {
				if m, ok := chunk["model"].(string); ok && m != "" {
					return m
				}
			}
		}
	}
	return ""
}

// ──────────────────────────────────────────
// Cache token extraction (all providers)
// ──────────────────────────────────────────

// CacheTokens holds the raw prompt caching breakdown from the provider API.
// These are the unmodified counts — not cost-normalized.
type CacheTokens struct {
	CacheReadTokens     int64
	CacheCreationTokens int64 // Anthropic-only (cache writes); 0 for OpenAI/Google
}

// extractCacheTokens extracts raw cache token counts from a standard response.
func extractCacheTokens(provider string, body []byte) CacheTokens {
	var resp map[string]interface{}
	if err := json.Unmarshal(body, &resp); err != nil {
		return CacheTokens{}
	}

	switch provider {
	case "anthropic", "anthropic-direct", "anthropic-vertex":
		if usage, ok := resp["usage"].(map[string]interface{}); ok {
			return CacheTokens{
				CacheReadTokens:     toInt64(usage["cache_read_input_tokens"]),
				CacheCreationTokens: toInt64(usage["cache_creation_input_tokens"]),
			}
		}

	case "openai", "gemini-oai":
		// OpenAI reports cached tokens inside usage.prompt_tokens_details.cached_tokens
		if usage, ok := resp["usage"].(map[string]interface{}); ok {
			if details, ok := usage["prompt_tokens_details"].(map[string]interface{}); ok {
				return CacheTokens{
					CacheReadTokens: toInt64(details["cached_tokens"]),
				}
			}
		}

	case "google":
		// Google reports cached tokens in usageMetadata.cachedContentTokenCount
		if meta, ok := resp["usageMetadata"].(map[string]interface{}); ok {
			return CacheTokens{
				CacheReadTokens: toInt64(meta["cachedContentTokenCount"]),
			}
		}
	}

	return CacheTokens{}
}

// extractStreamingCacheTokens extracts raw cache token counts from SSE stream data.
func extractStreamingCacheTokens(provider string, data []byte) CacheTokens {
	switch provider {
	case "anthropic", "anthropic-direct", "anthropic-vertex":
		return extractAnthropicStreamingCache(data)

	case "openai", "gemini-oai":
		return extractOpenAIStreamingCache(data)

	case "google":
		return extractGoogleStreamingCache(data)
	}

	return CacheTokens{}
}

func extractAnthropicStreamingCache(data []byte) CacheTokens {
	var ct CacheTokens
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
		if err := json.Unmarshal([]byte(payload), &chunk); err != nil || chunk == nil {
			continue
		}
		// message_start event: cache tokens in chunk.message.usage
		if msg, ok := chunk["message"].(map[string]interface{}); ok {
			if usage, ok := msg["usage"].(map[string]interface{}); ok {
				if v := toInt64(usage["cache_read_input_tokens"]); v > 0 {
					ct.CacheReadTokens = v
				}
				if v := toInt64(usage["cache_creation_input_tokens"]); v > 0 {
					ct.CacheCreationTokens = v
				}
			}
		}
		// message_delta event: cache tokens in chunk.usage (top-level)
		if usage, ok := chunk["usage"].(map[string]interface{}); ok {
			if v := toInt64(usage["cache_read_input_tokens"]); v > 0 {
				ct.CacheReadTokens = v
			}
			if v := toInt64(usage["cache_creation_input_tokens"]); v > 0 {
				ct.CacheCreationTokens = v
			}
		}
	}
	return ct
}

func extractOpenAIStreamingCache(data []byte) CacheTokens {
	var ct CacheTokens
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
		// OpenAI includes usage in the final chunk when stream_options.include_usage is set.
		if usage, ok := chunk["usage"].(map[string]interface{}); ok {
			if details, ok := usage["prompt_tokens_details"].(map[string]interface{}); ok {
				ct.CacheReadTokens = toInt64(details["cached_tokens"])
			}
		}
	}
	return ct
}

func extractGoogleStreamingCache(data []byte) CacheTokens {
	var ct CacheTokens
	// Google streaming returns JSON array or newline-delimited JSON.
	// The last chunk contains usageMetadata. Try JSON array first.
	var chunks []map[string]interface{}
	if err := json.Unmarshal(data, &chunks); err == nil {
		for _, chunk := range chunks {
			if meta, ok := chunk["usageMetadata"].(map[string]interface{}); ok {
				ct.CacheReadTokens = toInt64(meta["cachedContentTokenCount"])
			}
		}
	} else {
		// Fallback: newline-delimited JSON.
		dec := json.NewDecoder(bytes.NewReader(data))
		for dec.More() {
			var chunk map[string]interface{}
			if err := dec.Decode(&chunk); err != nil {
				break
			}
			if meta, ok := chunk["usageMetadata"].(map[string]interface{}); ok {
				ct.CacheReadTokens = toInt64(meta["cachedContentTokenCount"])
			}
		}
	}
	return ct
}

// injectStreamUsageOption ensures "stream_options": {"include_usage": true} is
// present in an OpenAI-format request body. Without this, OpenAI and Gemini-OAI
// streaming responses omit usage data from the final SSE chunk, making token
// counting impossible for streaming requests.
//
// This is a no-op if stream_options is already set or if the body is not valid JSON.
func injectStreamUsageOption(provider string, body []byte) []byte {
	switch provider {
	case "openai", "gemini-oai":
		// Only inject for OpenAI-compatible providers.
	default:
		return body
	}

	var req map[string]interface{}
	if err := json.Unmarshal(body, &req); err != nil {
		return body // Not valid JSON — pass through unchanged.
	}

	// Don't override if the client already set stream_options.
	if _, exists := req["stream_options"]; exists {
		return body
	}

	req["stream_options"] = map[string]interface{}{
		"include_usage": true,
	}

	modified, err := json.Marshal(req)
	if err != nil {
		return body // Marshal failed — pass through unchanged.
	}
	return modified
}

// isAnthropicProvider returns true if the provider name corresponds to an
// Anthropic provider (direct, vertex, or canonical). Used to restrict
// Anthropic-specific processing (like cache TTL body inspection) to only
// relevant traffic, avoiding unnecessary overhead for other providers.
func isAnthropicProvider(provider string) bool {
	switch provider {
	case "anthropic", "anthropic-direct", "anthropic-vertex":
		return true
	default:
		return strings.HasPrefix(provider, "anthropic-")
	}
}

// extractAnthropicCacheTTL inspects an Anthropic request body for cache_control
// blocks that specify a 1-hour TTL. Returns true if ANY content block in the
// system prompt or messages contains {"cache_control": {"ttl": "1h"}}.
//
// This is used for passthrough Anthropic routes (anthropic-direct,
// anthropic-vertex) where the client sets cache_control directly — the proxy
// doesn't inject it via FormatTranslator and thus has no TTL state.
//
// For translated requests (OpenAI → Anthropic), the proxy already knows the
// TTL from the AnthropicFormatTranslator config.
func extractAnthropicCacheTTL(body []byte) bool {
	// Fast-path: skip full JSON parsing if the body doesn't contain "ttl"
	// at all. This avoids expensive unmarshal for the common case where no
	// extended TTL is set.
	if !bytes.Contains(body, []byte(`"ttl"`)) {
		return false
	}

	var req map[string]interface{}
	if err := json.Unmarshal(body, &req); err != nil {
		return false
	}

	// Check system prompt (can be string or []content_block).
	if sys, ok := req["system"].([]interface{}); ok {
		for _, block := range sys {
			if hasCacheTTL1h(block) {
				return true
			}
		}
	}

	// Check messages[].content blocks.
	if messages, ok := req["messages"].([]interface{}); ok {
		for _, msg := range messages {
			msgMap, ok := msg.(map[string]interface{})
			if !ok {
				continue
			}
			if content, ok := msgMap["content"].([]interface{}); ok {
				for _, block := range content {
					if hasCacheTTL1h(block) {
						return true
					}
				}
			}
		}
	}

	return false
}

// hasCacheTTL1h checks if a content block has cache_control with ttl "1h".
func hasCacheTTL1h(block interface{}) bool {
	blockMap, ok := block.(map[string]interface{})
	if !ok {
		return false
	}
	cc, ok := blockMap["cache_control"].(map[string]interface{})
	if !ok {
		return false
	}
	ttl, _ := cc["ttl"].(string)
	return ttl == "1h"
}
