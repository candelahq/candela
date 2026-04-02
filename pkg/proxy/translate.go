package proxy

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
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

// AnthropicFormatTranslator translates between OpenAI Chat Completions format
// and Anthropic Messages format.
type AnthropicFormatTranslator struct{}

// --- Request Translation: OpenAI → Anthropic ---

func (t *AnthropicFormatTranslator) TranslateRequest(body []byte) ([]byte, string, error) {
	var oaiReq openAIRequest
	if err := json.Unmarshal(body, &oaiReq); err != nil {
		return nil, "", fmt.Errorf("invalid OpenAI request: %w", err)
	}

	anthReq := anthropicRequest{
		Model:            oaiReq.Model,
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

	// Separate system messages from user/assistant messages.
	for _, msg := range oaiReq.Messages {
		if msg.Role == "system" {
			content, _ := msg.Content.(string)
			anthReq.System = content
		} else {
			anthReq.Messages = append(anthReq.Messages, anthropicMessage(msg))
		}
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

	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)

		// Pass through non-data lines (event:, empty lines).
		if !strings.HasPrefix(line, "data: ") {
			result.WriteString(line + "\n")
			continue
		}

		payload := strings.TrimPrefix(line, "data: ")
		if payload == "[DONE]" {
			result.WriteString("data: [DONE]\n")
			continue
		}

		var chunk map[string]interface{}
		if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
			result.WriteString(line + "\n")
			continue
		}

		chunkType, _ := chunk["type"].(string)
		switch chunkType {
		case "message_start":
			oaiChunk := openAIStreamChunk{
				ID:      getStringField(chunk, "message", "id"),
				Object:  "chat.completion.chunk",
				Created: time.Now().Unix(),
				Model:   info.Display,
				Choices: []openAIStreamChoice{
					{Index: 0, Delta: openAIDelta{Role: "assistant"}},
				},
			}
			b, _ := json.Marshal(oaiChunk)
			result.WriteString("data: " + string(b) + "\n")

		case "content_block_delta":
			delta, _ := chunk["delta"].(map[string]interface{})
			text, _ := delta["text"].(string)
			if text != "" {
				oaiChunk := openAIStreamChunk{
					Object:  "chat.completion.chunk",
					Created: time.Now().Unix(),
					Model:   info.Display,
					Choices: []openAIStreamChoice{
						{Index: 0, Delta: openAIDelta{Content: text}},
					},
				}
				b, _ := json.Marshal(oaiChunk)
				result.WriteString("data: " + string(b) + "\n")
			}

		case "message_delta":
			usage, _ := chunk["usage"].(map[string]interface{})
			stopDelta, _ := chunk["delta"].(map[string]interface{})
			sr, _ := stopDelta["stop_reason"].(string)

			oaiChunk := openAIStreamChunk{
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
			result.WriteString("data: " + string(b) + "\n")

		case "message_stop":
			result.WriteString("data: [DONE]\n")

		default:
			result.WriteString(line + "\n")
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
}

type openAIReqMsg struct {
	Role    string      `json:"role"`
	Content interface{} `json:"content"` // string or []content_part
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
	Model            string             `json:"model"`
	Messages         []anthropicMessage `json:"messages"`
	System           string             `json:"system,omitempty"`
	MaxTokens        int                `json:"max_tokens"`
	Temperature      *float64           `json:"temperature,omitempty"`
	TopP             *float64           `json:"top_p,omitempty"`
	Stream           bool               `json:"stream,omitempty"`
	AnthropicVersion string             `json:"anthropic_version,omitempty"`
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
