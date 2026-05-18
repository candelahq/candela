package proxy

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"sync/atomic"
	"time"
)

// dateVersionRe matches a trailing date suffix like "-20250514" at the end of a model name.
var dateVersionRe = regexp.MustCompile(`-(\d{8})$`)

// ModelNameInfo holds parsed model name variants for different contexts.
type ModelNameInfo struct {
	// Raw is the model name as received from the client (e.g. "claude-sonnet-4-20250514").
	Raw string
	// VertexAI is the model name formatted for Vertex AI (e.g. "claude-sonnet-4-20250514@20250514").
	VertexAI string
	// Display is the clean model name without the date suffix (e.g. "claude-sonnet-4").
	Display string
}

// ParseModelName extracts date version info from an Anthropic model name.
// Input: "claude-sonnet-4-20250514" → VertexAI: "claude-sonnet-4-20250514@20250514", Display: "claude-sonnet-4"
// Input: "claude-3-5-sonnet-20241022" → VertexAI: "claude-3-5-sonnet@20241022", Display: "claude-3-5-sonnet"
func ParseModelName(raw string) ModelNameInfo {
	info := ModelNameInfo{Raw: raw}

	matches := dateVersionRe.FindStringSubmatch(raw)
	if len(matches) == 2 {
		date := matches[1]
		baseName := raw[:len(raw)-len(date)-1] // strip "-YYYYMMDD"
		info.VertexAI = baseName + "@" + date
		info.Display = baseName
	} else {
		// No date suffix — pass through as-is.
		info.VertexAI = raw
		info.Display = raw
	}

	return info
}

// ====================================================================
// AnthropicFormatTranslator — implements FormatTranslator
// ====================================================================

// CachingMode controls how cache_control breakpoints are injected into
// Anthropic requests. This determines the caching strategy for Vertex AI.
type CachingMode string

const (
	// CachingOff disables cache_control injection entirely.
	CachingOff CachingMode = "off"
	// CachingAuto injects cache_control on both the system prompt and the
	// last user message — Anthropic's recommended two-breakpoint pattern.
	// This is the default for maximum cost savings.
	CachingAuto CachingMode = "auto"
	// CachingSystemOnly injects cache_control on the system prompt only.
	// Useful when conversation history changes frequently.
	CachingSystemOnly CachingMode = "system-only"
)

// CacheTTL controls the time-to-live for Anthropic prompt cache entries.
// Orthogonal to CachingMode: mode controls *where* breakpoints go,
// TTL controls *how long* cache entries live.
type CacheTTL string

const (
	// CacheTTL5m is the default 5-minute cache duration.
	// Cache writes cost 1.25x base input price.
	CacheTTL5m CacheTTL = "5m"
	// CacheTTL1h is the extended 1-hour cache duration.
	// Cache writes cost 2x base input price, but cache reads
	// remain at 0.1x — ideal for long coding sessions.
	CacheTTL1h CacheTTL = "1h"
)

// CacheTTLHeader is the HTTP header name for per-request cache TTL overrides.
// Clients can send X-Candela-Cache-TTL: 5m|1h to override
// the server's default TTL on a per-request basis.
const CacheTTLHeader = "X-Candela-Cache-TTL"

// CachingHeader is the HTTP header name for per-request caching overrides.
// Clients can send X-Candela-Caching: off|auto|system-only to override
// the server's default caching mode on a per-request basis.
const CachingHeader = "X-Candela-Caching"

// ParseCachingMode converts a config string to a CachingMode.
// Supports backward compatibility: "true"/"1" → auto, "false"/"0"/"" → off.
func ParseCachingMode(s string) CachingMode {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "auto", "true", "1":
		return CachingAuto
	case "system-only", "system_only", "system":
		return CachingSystemOnly
	case "off", "false", "0", "":
		return CachingOff
	default:
		slog.Warn("unrecognized caching mode, defaulting to off",
			"input", s,
			"valid_modes", "off, auto, system-only")
		return CachingOff
	}
}

// ParseCacheTTL converts a config string to a CacheTTL.
// Accepts: "5m", "5min", "" (default) → 5m; "1h", "1hr", "1hour", "60m" → 1h.
func ParseCacheTTL(s string) CacheTTL {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "1h", "1hr", "1hour", "60m":
		return CacheTTL1h
	case "5m", "5min", "", "default":
		return CacheTTL5m
	default:
		slog.Warn("unrecognized cache TTL, defaulting to 5m",
			"input", s,
			"valid_ttls", "5m, 1h")
		return CacheTTL5m
	}
}

// buildCacheControl returns the cache_control map for injection into
// Anthropic request bodies. When ttl is CacheTTL1h, includes "ttl": "1h".
// When ttl is CacheTTL5m (default), omits the ttl field for backward compat.
func buildCacheControl(ttl CacheTTL) map[string]string {
	cc := map[string]string{"type": "ephemeral"}
	if ttl == CacheTTL1h {
		cc["ttl"] = "1h"
	}
	return cc
}

// AnthropicFormatTranslator translates between OpenAI Chat Completions format
// and Anthropic Messages format.
type AnthropicFormatTranslator struct {
	cachingMode atomic.Value // stores CachingMode; default is CachingAuto
	cacheTTL    atomic.Value // stores CacheTTL; default is CacheTTL5m
}

// SetCachingMode sets the caching strategy. Safe to call concurrently.
func (t *AnthropicFormatTranslator) SetCachingMode(mode CachingMode) {
	t.cachingMode.Store(mode)
}

// GetCachingMode returns the current caching strategy.
func (t *AnthropicFormatTranslator) GetCachingMode() CachingMode {
	v := t.cachingMode.Load()
	if v == nil {
		return CachingAuto // default: caching enabled
	}
	return v.(CachingMode)
}

// SetCacheTTL sets the cache TTL duration. Safe to call concurrently.
func (t *AnthropicFormatTranslator) SetCacheTTL(ttl CacheTTL) {
	t.cacheTTL.Store(ttl)
}

// GetCacheTTL returns the current cache TTL duration.
func (t *AnthropicFormatTranslator) GetCacheTTL() CacheTTL {
	v := t.cacheTTL.Load()
	if v == nil {
		return CacheTTL5m // default: 5-minute TTL
	}
	return v.(CacheTTL)
}

// --- Request Translation: OpenAI → Anthropic ---

// TranslateRequest converts an OpenAI request using the translator's configured
// caching mode and TTL. Both are snapshotted once at the start of the call,
// ensuring a consistent view even if another goroutine changes them concurrently.
func (t *AnthropicFormatTranslator) TranslateRequest(body []byte) ([]byte, string, error) {
	return t.translateRequestFull(body, t.GetCachingMode(), t.GetCacheTTL())
}

// TranslateRequestWithMode converts an OpenAI request using an explicit caching
// mode. This is used for per-request header overrides (X-Candela-Caching) to
// avoid mutating shared translator state, which would race with concurrent requests.
func (t *AnthropicFormatTranslator) TranslateRequestWithMode(body []byte, mode CachingMode) ([]byte, string, error) {
	return t.translateRequestFull(body, mode, t.GetCacheTTL())
}

// TranslateRequestWithModeAndTTL converts an OpenAI request using explicit
// caching mode and TTL. Used for per-request header overrides.
func (t *AnthropicFormatTranslator) TranslateRequestWithModeAndTTL(body []byte, mode CachingMode, ttl CacheTTL) ([]byte, string, error) {
	return t.translateRequestFull(body, mode, ttl)
}

func (t *AnthropicFormatTranslator) translateRequestFull(body []byte, mode CachingMode, ttl CacheTTL) ([]byte, string, error) {
	var oaiReq openAIRequest
	if err := json.Unmarshal(body, &oaiReq); err != nil {
		return nil, "", fmt.Errorf("invalid OpenAI request: %w", err)
	}

	anthReq := anthropicRequest{
		// Note: Model is NOT included in the body for Vertex AI.
		// Vertex AI identifies the model from the URL path.
		MaxTokens:        oaiReq.MaxTokens,
		Stream:           oaiReq.Stream,
		AnthropicVersion: "vertex-2023-10-16",
	}

	// Default max_tokens if unset (required for Anthropic).
	if anthReq.MaxTokens == 0 {
		anthReq.MaxTokens = 4096
	}

	if oaiReq.Temperature != nil {
		anthReq.Temperature = oaiReq.Temperature
	}
	if oaiReq.TopP != nil {
		anthReq.TopP = oaiReq.TopP
	}

	// Translate tools if present (OpenAI → Anthropic format).
	if len(oaiReq.Tools) > 0 {
		for _, tool := range oaiReq.Tools {
			if tool.Type == "function" && tool.Function != nil {
				anthTool := anthropicTool{
					Name:        tool.Function.Name,
					Description: tool.Function.Description,
					InputSchema: tool.Function.Parameters,
				}
				anthReq.Tools = append(anthReq.Tools, anthTool)
			}
		}
	}

	// Translate messages: system, user, assistant (with tool_calls), tool results.
	for _, msg := range oaiReq.Messages {
		switch msg.Role {
		case "system":
			// OpenAI allows system content as a string or an array of content blocks.
			switch c := msg.Content.(type) {
			case string:
				if c == "" {
					break
				}
				if mode == CachingAuto || mode == CachingSystemOnly {
					anthReq.System = []interface{}{
						map[string]interface{}{
							"type":          "text",
							"text":          c,
							"cache_control": buildCacheControl(ttl),
						},
					}
				} else {
					anthReq.System = c
				}
			case []interface{}:
				// Array of content blocks — pass through as-is.
				if (mode == CachingAuto || mode == CachingSystemOnly) && len(c) > 0 {
					// Add cache_control to the last block.
					if block, ok := c[len(c)-1].(map[string]interface{}); ok {
						block["cache_control"] = buildCacheControl(ttl)
					}
				}
				anthReq.System = c
			}

		case "assistant":
			if len(msg.ToolCalls) > 0 {
				// Assistant message with tool_calls → content blocks with tool_use.
				var contentBlocks []interface{}
				// Include any text content first.
				if textContent, ok := msg.Content.(string); ok && textContent != "" {
					contentBlocks = append(contentBlocks, map[string]interface{}{
						"type": "text",
						"text": textContent,
					})
				}
				for _, tc := range msg.ToolCalls {
					// Parse the arguments string into a raw object.
					var inputObj interface{}
					if err := json.Unmarshal([]byte(tc.Function.Arguments), &inputObj); err != nil {
						inputObj = map[string]interface{}{}
					}
					contentBlocks = append(contentBlocks, map[string]interface{}{
						"type":  "tool_use",
						"id":    tc.ID,
						"name":  tc.Function.Name,
						"input": inputObj,
					})
				}
				anthReq.Messages = append(anthReq.Messages, anthropicMessage{
					Role:    "assistant",
					Content: contentBlocks,
				})
			} else {
				anthReq.Messages = append(anthReq.Messages, anthropicMessage(msg.toAnthropicMessage()))
			}

		case "tool":
			// OpenAI tool result → Anthropic user message with tool_result content.
			toolContent, _ := msg.Content.(string)
			contentBlock := map[string]interface{}{
				"type":        "tool_result",
				"tool_use_id": msg.ToolCallID,
				"content":     toolContent,
			}
			// Merge consecutive tool results into one user message if the last
			// message is already a user message with tool_result content.
			if len(anthReq.Messages) > 0 {
				last := &anthReq.Messages[len(anthReq.Messages)-1]
				if last.Role == "user" {
					if blocks, ok := last.Content.([]interface{}); ok {
						last.Content = append(blocks, contentBlock)
						continue
					}
				}
			}
			anthReq.Messages = append(anthReq.Messages, anthropicMessage{
				Role:    "user",
				Content: []interface{}{contentBlock},
			})

		default:
			anthReq.Messages = append(anthReq.Messages, anthropicMessage(msg.toAnthropicMessage()))
		}
	}

	// Add cache_control to the last user/tool message's content so the
	// entire conversation prefix is cached. This is Anthropic's recommended
	// two-breakpoint pattern: system prompt + end of conversation history.
	if mode == CachingAuto {
		injectLastMessageCacheControl(anthReq.Messages, ttl)
	}

	translated, err := json.Marshal(anthReq)
	if err != nil {
		return nil, "", fmt.Errorf("failed to marshal Anthropic request: %w", err)
	}

	return translated, oaiReq.Model, nil
}

// --- Response Translation: Anthropic → OpenAI ---

func (t *AnthropicFormatTranslator) TranslateResponse(body []byte, model string) ([]byte, error) {
	var anthResp anthropicResponse
	if err := json.Unmarshal(body, &anthResp); err != nil {
		return nil, fmt.Errorf("invalid Anthropic response: %w", err)
	}

	info := ParseModelName(model)

	oaiResp := openAIResponse{
		ID:      anthResp.ID,
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Model:   info.Display,
		Choices: []openAIChoice{
			{
				Index: 0,
				Message: openAIMessage{
					Role:    "assistant",
					Content: extractAnthropicText(anthResp.Content),
				},
				FinishReason: mapStopReason(anthResp.StopReason),
			},
		},
		Usage: openAIUsage{
			PromptTokens:     anthResp.Usage.InputTokens,
			CompletionTokens: anthResp.Usage.OutputTokens,
			TotalTokens:      anthResp.Usage.InputTokens + anthResp.Usage.OutputTokens,
		},
	}

	return json.Marshal(oaiResp)
}

// --- Streaming Translation: Anthropic SSE → OpenAI SSE ---

func (t *AnthropicFormatTranslator) TranslateStreamChunk(data []byte, model string) ([]byte, error) {
	info := ParseModelName(model)
	var result strings.Builder

	// We need a consistent ID across all chunks in the stream.
	// Extract it from message_start if present, otherwise generate one.
	streamID := "chatcmpl-" + generateSpanID()

	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)

		// Drop "event:" lines and empty lines — OpenAI SSE doesn't use them.
		if strings.HasPrefix(line, "event:") || line == "" {
			continue
		}

		// Only process "data: " lines.
		if !strings.HasPrefix(line, "data: ") {
			continue
		}

		payload := strings.TrimPrefix(line, "data: ")
		if payload == "[DONE]" {
			result.WriteString("data: [DONE]\n\n")
			continue
		}

		var chunk map[string]interface{}
		if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
			continue // Skip unparseable chunks.
		}

		chunkType, _ := chunk["type"].(string)
		switch chunkType {
		case "message_start":
			// Extract message ID for use in all subsequent chunks.
			if msgID := getStringField(chunk, "message", "id"); msgID != "" {
				streamID = msgID
			}
			oaiChunk := openAIStreamChunk{
				ID:      streamID,
				Object:  "chat.completion.chunk",
				Created: time.Now().Unix(),
				Model:   info.Display,
				Choices: []openAIStreamChoice{
					{Index: 0, Delta: openAIDelta{Role: "assistant"}},
				},
			}
			b, _ := json.Marshal(oaiChunk)
			result.WriteString("data: " + string(b) + "\n\n")

		case "content_block_delta":
			delta, _ := chunk["delta"].(map[string]interface{})
			deltaType, _ := delta["type"].(string)
			switch deltaType {
			case "text_delta":
				text, _ := delta["text"].(string)
				if text != "" {
					oaiChunk := openAIStreamChunk{
						ID:      streamID,
						Object:  "chat.completion.chunk",
						Created: time.Now().Unix(),
						Model:   info.Display,
						Choices: []openAIStreamChoice{
							{Index: 0, Delta: openAIDelta{Content: text}},
						},
					}
					b, _ := json.Marshal(oaiChunk)
					result.WriteString("data: " + string(b) + "\n\n")
				}
			case "input_json_delta":
				// Stream tool call arguments as they arrive.
				partialJSON, _ := delta["partial_json"].(string)
				if partialJSON != "" {
					oaiChunk := map[string]interface{}{
						"id":      streamID,
						"object":  "chat.completion.chunk",
						"created": time.Now().Unix(),
						"model":   info.Display,
						"choices": []map[string]interface{}{
							{
								"index": 0,
								"delta": map[string]interface{}{
									"tool_calls": []map[string]interface{}{
										{
											"index": 0,
											"function": map[string]interface{}{
												"arguments": partialJSON,
											},
										},
									},
								},
							},
						},
					}
					b, _ := json.Marshal(oaiChunk)
					result.WriteString("data: " + string(b) + "\n\n")
				}
			}

		case "content_block_start":
			cb, _ := chunk["content_block"].(map[string]interface{})
			cbType, _ := cb["type"].(string)
			if cbType == "tool_use" {
				toolID, _ := cb["id"].(string)
				toolName, _ := cb["name"].(string)
				oaiChunk := map[string]interface{}{
					"id":      streamID,
					"object":  "chat.completion.chunk",
					"created": time.Now().Unix(),
					"model":   info.Display,
					"choices": []map[string]interface{}{
						{
							"index": 0,
							"delta": map[string]interface{}{
								"tool_calls": []map[string]interface{}{
									{
										"index": 0,
										"id":    toolID,
										"type":  "function",
										"function": map[string]interface{}{
											"name":      toolName,
											"arguments": "",
										},
									},
								},
							},
						},
					},
				}
				b, _ := json.Marshal(oaiChunk)
				result.WriteString("data: " + string(b) + "\n\n")
			}

		case "message_delta":
			usage, _ := chunk["usage"].(map[string]interface{})
			stopDelta, _ := chunk["delta"].(map[string]interface{})
			sr, _ := stopDelta["stop_reason"].(string)

			oaiChunk := openAIStreamChunk{
				ID:      streamID,
				Object:  "chat.completion.chunk",
				Created: time.Now().Unix(),
				Model:   info.Display,
				Choices: []openAIStreamChoice{
					{Index: 0, Delta: openAIDelta{}, FinishReason: mapStopReason(sr)},
				},
			}
			if usage != nil {
				oaiChunk.Usage = &openAIUsage{
					PromptTokens:     toInt64(usage["input_tokens"]),
					CompletionTokens: toInt64(usage["output_tokens"]),
					TotalTokens:      toInt64(usage["input_tokens"]) + toInt64(usage["output_tokens"]),
				}
			}
			b, _ := json.Marshal(oaiChunk)
			result.WriteString("data: " + string(b) + "\n\n")

		case "message_stop":
			result.WriteString("data: [DONE]\n\n")

		case "ping", "content_block_stop":
			// No OpenAI equivalent — skip silently.

		default:
			// Unknown event — skip to avoid breaking clients.
		}
	}

	return []byte(result.String()), nil
}

// ====================================================================
// VertexAIPathRewriter — implements PathRewriter
// ====================================================================

// VertexAIPathRewriter rewrites URL paths for Vertex AI's publisher model endpoints.
type VertexAIPathRewriter struct {
	ProjectID string // GCP project ID
	Region    string // GCP region (e.g. "us-central1")
}

func (r *VertexAIPathRewriter) RewritePath(model string, streaming bool) string {
	info := ParseModelName(model)

	method := "rawPredict"
	if streaming {
		method = "streamRawPredict"
	}
	return fmt.Sprintf("/v1/projects/%s/locations/%s/publishers/anthropic/models/%s:%s",
		r.ProjectID, r.Region, info.VertexAI, method)
}

// ====================================================================
// Shared types for OpenAI ↔ Anthropic translation
// ====================================================================

// --- OpenAI types ---

type openAIRequest struct {
	Model       string         `json:"model"`
	Messages    []openAIReqMsg `json:"messages"`
	MaxTokens   int            `json:"max_tokens,omitempty"`
	Temperature *float64       `json:"temperature,omitempty"`
	TopP        *float64       `json:"top_p,omitempty"`
	Stream      bool           `json:"stream,omitempty"`
	Tools       []openAITool   `json:"tools,omitempty"`
}

type openAITool struct {
	Type     string          `json:"type"` // "function"
	Function *openAIToolFunc `json:"function,omitempty"`
}

type openAIToolFunc struct {
	Name        string      `json:"name"`
	Description string      `json:"description,omitempty"`
	Parameters  interface{} `json:"parameters,omitempty"` // JSON Schema object
}

type openAIToolCall struct {
	ID       string           `json:"id"`
	Type     string           `json:"type"` // "function"
	Function openAIToolCallFn `json:"function"`
}

type openAIToolCallFn struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"` // JSON string
}

type openAIReqMsg struct {
	Role       string           `json:"role"`
	Content    interface{}      `json:"content"`                // string or []content_part
	ToolCalls  []openAIToolCall `json:"tool_calls,omitempty"`   // assistant tool invocations
	ToolCallID string           `json:"tool_call_id,omitempty"` // for role=tool responses
}

// toAnthropicMessage converts a simple (non-tool) OpenAI message to Anthropic format.
func (m openAIReqMsg) toAnthropicMessage() anthropicMessage {
	return anthropicMessage{Role: m.Role, Content: m.Content}
}

type openAIResponse struct {
	ID      string         `json:"id"`
	Object  string         `json:"object"`
	Created int64          `json:"created"`
	Model   string         `json:"model"`
	Choices []openAIChoice `json:"choices"`
	Usage   openAIUsage    `json:"usage"`
}

type openAIChoice struct {
	Index        int           `json:"index"`
	Message      openAIMessage `json:"message"`
	FinishReason string        `json:"finish_reason"`
}

type openAIMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type openAIUsage struct {
	PromptTokens     int64 `json:"prompt_tokens"`
	CompletionTokens int64 `json:"completion_tokens"`
	TotalTokens      int64 `json:"total_tokens"`
}

type openAIStreamChunk struct {
	ID      string               `json:"id,omitempty"`
	Object  string               `json:"object"`
	Created int64                `json:"created"`
	Model   string               `json:"model"`
	Choices []openAIStreamChoice `json:"choices"`
	Usage   *openAIUsage         `json:"usage,omitempty"`
}

type openAIStreamChoice struct {
	Index        int         `json:"index"`
	Delta        openAIDelta `json:"delta"`
	FinishReason string      `json:"finish_reason,omitempty"`
}

type openAIDelta struct {
	Role    string `json:"role,omitempty"`
	Content string `json:"content,omitempty"`
}

// --- Anthropic types ---

type anthropicRequest struct {
	Model            string             `json:"model,omitempty"`
	Messages         []anthropicMessage `json:"messages"`
	System           interface{}        `json:"system,omitempty"` // string or []content_block with cache_control
	MaxTokens        int                `json:"max_tokens"`
	Temperature      *float64           `json:"temperature,omitempty"`
	TopP             *float64           `json:"top_p,omitempty"`
	Stream           bool               `json:"stream,omitempty"`
	AnthropicVersion string             `json:"anthropic_version,omitempty"`
	Tools            []anthropicTool    `json:"tools,omitempty"`
}

type anthropicTool struct {
	Name        string      `json:"name"`
	Description string      `json:"description,omitempty"`
	InputSchema interface{} `json:"input_schema"` // JSON Schema
}

type anthropicMessage struct {
	Role    string      `json:"role"`
	Content interface{} `json:"content"`
}

type anthropicResponse struct {
	ID         string             `json:"id"`
	Type       string             `json:"type"`
	Role       string             `json:"role"`
	Content    []anthropicContent `json:"content"`
	StopReason string             `json:"stop_reason"`
	Usage      anthropicUsage     `json:"usage"`
}

type anthropicContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type anthropicUsage struct {
	InputTokens  int64 `json:"input_tokens"`
	OutputTokens int64 `json:"output_tokens"`
}

// --- Helpers ---

func extractAnthropicText(content []anthropicContent) string {
	var parts []string
	for _, c := range content {
		if c.Type == "text" {
			parts = append(parts, c.Text)
		}
	}
	return strings.Join(parts, "")
}

func mapStopReason(reason string) string {
	switch reason {
	case "end_turn":
		return "stop"
	case "max_tokens":
		return "length"
	case "stop_sequence":
		return "stop"
	case "tool_use":
		return "tool_calls"
	default:
		if reason == "" {
			return "stop"
		}
		return reason
	}
}

func getStringField(m map[string]interface{}, keys ...string) string {
	current := m
	for i, key := range keys {
		if i == len(keys)-1 {
			v, _ := current[key].(string)
			return v
		}
		next, ok := current[key].(map[string]interface{})
		if !ok {
			return ""
		}
		current = next
	}
	return ""
}

// injectLastMessageCacheControl adds a cache_control breakpoint to the last
// user-role message in the conversation. This implements Anthropic's
// recommended two-breakpoint pattern:
//
//  1. System prompt (always cached — set above)
//  2. End of conversation history (this function)
//
// Together they cache the entire stable prefix so each new turn only pays
// full price for the latest user message.
func injectLastMessageCacheControl(messages []anthropicMessage, ttl CacheTTL) {
	// Walk backwards to find the last user or tool-result message.
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role != "user" {
			continue
		}

		switch content := messages[i].Content.(type) {
		case string:
			// Simple string content → convert to cached content block.
			messages[i].Content = []interface{}{
				map[string]interface{}{
					"type":          "text",
					"text":          content,
					"cache_control": buildCacheControl(ttl),
				},
			}
		case []interface{}:
			// Array of content blocks → add cache_control to the last block.
			if len(content) > 0 {
				if block, ok := content[len(content)-1].(map[string]interface{}); ok {
					block["cache_control"] = buildCacheControl(ttl)
				}
			}
		}
		return
	}
}
