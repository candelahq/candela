package proxy

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"strings"
	"sync"
	"time"
)

// thoughtSignatureEntry holds a cached signature with expiry.
type thoughtSignatureEntry struct {
	signature string
	expiresAt time.Time
}

// thoughtSignatureStore caches thought_signature values keyed by tool_call ID.
// Signatures are required for Gemini 2.5+ / 3.x thinking models with function calling.
// When clients use the OpenAI-compatible endpoint (gemini-oai), the client SDK
// (e.g. @ai-sdk/openai-compatible) may strip unknown fields, so the proxy
// must stash signatures from responses and re-inject them on the next request.
type ThoughtSignatureStore struct {
	mu   sync.RWMutex
	data map[string]thoughtSignatureEntry
	ttl  time.Duration
}

func NewThoughtSignatureStore(ttl time.Duration) *ThoughtSignatureStore {
	if ttl <= 0 {
		ttl = 15 * time.Minute
	}
	s := &ThoughtSignatureStore{
		data: make(map[string]thoughtSignatureEntry),
		ttl:  ttl,
	}
	// Lazy cleanup via background goroutine (every ttl/2)
	go s.janitor()
	return s
}

func (s *ThoughtSignatureStore) janitor() {
	tick := time.NewTicker(s.ttl / 2)
	if s.ttl/2 < time.Minute {
		tick = time.NewTicker(time.Minute)
	}
	for range tick.C {
		s.cleanup()
	}
}

func (s *ThoughtSignatureStore) cleanup() {
	now := time.Now()
	s.mu.Lock()
	defer s.mu.Unlock()
	for k, v := range s.data {
		if now.After(v.expiresAt) {
			delete(s.data, k)
		}
	}
}

func (s *ThoughtSignatureStore) Store(id, sig string) {
	if id == "" || sig == "" {
		return
	}
	s.mu.Lock()
	s.data[id] = thoughtSignatureEntry{signature: sig, expiresAt: time.Now().Add(s.ttl)}
	s.mu.Unlock()
	slog.Info("thought_signature: stored", "tool_call_id", id, "sig_len", len(sig))
}

func (s *ThoughtSignatureStore) Load(id string) (string, bool) {
	if id == "" {
		return "", false
	}
	s.mu.RLock()
	e, ok := s.data[id]
	s.mu.RUnlock()
	if !ok {
		return "", false
	}
	if time.Now().After(e.expiresAt) {
		s.mu.Lock()
		delete(s.data, id)
		s.mu.Unlock()
		return "", false
	}
	return e.signature, true
}

// extractAndStoreThoughtSignatures parses a non-streaming response body (OpenAI or Google)
// and stores any thought_signature/thoughtSignature found.
func (s *ThoughtSignatureStore) ExtractAndStoreFromResponse(body []byte, provider string) {
	if len(body) == 0 || s == nil {
		return
	}
	var raw map[string]interface{}
	if err := json.Unmarshal(body, &raw); err != nil {
		return
	}
	// Provider-specific paths
	switch provider {
	case "gemini-oai", "openai", "google", "gemini-vertex":
		// Try OpenAI path first (choices[].message.tool_calls)
		s.extractFromOpenAIChoices(raw)
		// Try Google native path (candidates[].content.parts[].functionCall)
		s.extractFromGoogleCandidates(raw)
	default:
		// For gemini-oai we care, but also try both for safety
		if provider == "gemini-oai" || strings.HasPrefix(provider, "gemini") || provider == "google" {
			s.extractFromOpenAIChoices(raw)
			s.extractFromGoogleCandidates(raw)
		}
	}
}

func (s *ThoughtSignatureStore) extractFromOpenAIChoices(raw map[string]interface{}) {
	choices, _ := raw["choices"].([]interface{})
	for _, c := range choices {
		cm, ok := c.(map[string]interface{})
		if !ok {
			continue
		}
		// non-streaming: message.tool_calls
		if msg, ok := cm["message"].(map[string]interface{}); ok {
			if tcs, ok := msg["tool_calls"].([]interface{}); ok {
				for _, tc := range tcs {
					if tcm, ok := tc.(map[string]interface{}); ok {
						s.storeFromToolCallMap(tcm)
					}
				}
			}
		}
		// streaming chunk may be in delta (but non-stream response shouldn't have delta)
		if delta, ok := cm["delta"].(map[string]interface{}); ok {
			if tcs, ok := delta["tool_calls"].([]interface{}); ok {
				for _, tc := range tcs {
					if tcm, ok := tc.(map[string]interface{}); ok {
						s.storeFromToolCallMap(tcm)
					}
				}
			}
		}
	}
}

func (s *ThoughtSignatureStore) extractFromGoogleCandidates(raw map[string]interface{}) {
	cands, _ := raw["candidates"].([]interface{})
	for _, c := range cands {
		cm, ok := c.(map[string]interface{})
		if !ok {
			continue
		}
		content, _ := cm["content"].(map[string]interface{})
		if content == nil {
			continue
		}
		parts, _ := content["parts"].([]interface{})
		for _, p := range parts {
			pm, ok := p.(map[string]interface{})
			if !ok {
				continue
			}
			fc, _ := pm["functionCall"].(map[string]interface{})
			if fc == nil {
				fc, _ = pm["function_call"].(map[string]interface{})
			}
			if fc == nil {
				continue
			}
			sig := getThoughtSignatureFromMap(fc)
			if sig == "" {
				sig = getThoughtSignatureFromMap(pm)
			}
			if sig == "" {
				sig = getThoughtSignatureFromExtraContent(fc)
			}
			if sig == "" {
				sig = getThoughtSignatureFromExtraContent(pm)
			}
			id, _ := fc["id"].(string)
			if id == "" {
				id, _ = pm["id"].(string)
			}
			if sig != "" && id != "" {
				s.Store(id, sig)
			}
		}
	}
}

func (s *ThoughtSignatureStore) storeFromToolCallMap(tcm map[string]interface{}) {
	id, _ := tcm["id"].(string)
	sig := getThoughtSignatureFromMap(tcm)
	// also check nested function object
	if sig == "" {
		if fn, ok := tcm["function"].(map[string]interface{}); ok {
			sig = getThoughtSignatureFromMap(fn)
		}
	}
	// also check extra_content.google.thought_signature (real Gemini 3.x via OpenAI compat)
	if sig == "" {
		sig = getThoughtSignatureFromExtraContent(tcm)
	}
	if sig != "" && id != "" {
		s.Store(id, sig)
	}
}

func getThoughtSignatureFromExtraContent(m map[string]interface{}) string {
	if ec, ok := m["extra_content"].(map[string]interface{}); ok {
		if g, ok := ec["google"].(map[string]interface{}); ok {
			if v, ok := g["thought_signature"].(string); ok && v != "" {
				return v
			}
			if v, ok := g["thoughtSignature"].(string); ok && v != "" {
				return v
			}
		}
	}
	return ""
}

func getThoughtSignatureFromMap(m map[string]interface{}) string {
	if v, ok := m["thought_signature"].(string); ok && v != "" {
		return v
	}
	if v, ok := m["thoughtSignature"].(string); ok && v != "" {
		return v
	}
	return ""
}

// extractAndStoreFromStream parses SSE/streaming data (OpenAI SSE or Google JSON array)
// and stores signatures found in any chunk.
func (s *ThoughtSignatureStore) ExtractAndStoreFromStream(data []byte, provider string) {
	if len(data) == 0 || s == nil {
		return
	}
	// Try as OpenAI SSE (data: lines)
	if bytes.Contains(data, []byte("data: ")) {
		for _, line := range bytes.Split(data, []byte("\n")) {
			line = bytes.TrimSpace(line)
			if !bytes.HasPrefix(line, []byte("data: ")) {
				continue
			}
			payload := bytes.TrimPrefix(line, []byte("data: "))
			if string(payload) == "[DONE]" {
				continue
			}
			var chunk map[string]interface{}
			if err := json.Unmarshal(payload, &chunk); err != nil {
				continue
			}
			// Reuse OpenAI extraction on a fake wrapper with choices
			s.extractFromOpenAIChoices(chunk)
		}
		return
	}
	// Try as Google JSON array (streamGenerateContent)
	var arr []map[string]interface{}
	if err := json.Unmarshal(data, &arr); err == nil && len(arr) > 0 {
		for _, chunk := range arr {
			s.extractFromGoogleCandidates(chunk)
			s.extractFromOpenAIChoices(chunk)
		}
		return
	}
	// Try as single JSON object (Google)
	var single map[string]interface{}
	if err := json.Unmarshal(data, &single); err == nil {
		s.extractFromGoogleCandidates(single)
		s.extractFromOpenAIChoices(single)
	}
	// Fallback: try line-delimited JSON (Google NDJSON)
	if !bytes.Contains(data, []byte("data: ")) {
		dec := json.NewDecoder(bytes.NewReader(data))
		for dec.More() {
			var chunk map[string]interface{}
			if err := dec.Decode(&chunk); err != nil {
				break
			}
			s.extractFromGoogleCandidates(chunk)
			s.extractFromOpenAIChoices(chunk)
		}
	}
}

// injectIntoRequest parses an OpenAI chat completions request body and re-injects
// missing thought_signature values from the store. Returns modified body (or original if no changes).
// Only applies to gemini-oai (and google/gemini-vertex for safety).
func (s *ThoughtSignatureStore) InjectIntoRequest(body []byte, provider string) []byte {
	if s == nil || len(body) == 0 {
		return body
	}
	// Only for Gemini providers where thought_signature is required
	if provider != "gemini-oai" && provider != "google" && provider != "gemini-vertex" && !strings.HasPrefix(provider, "gemini") {
		return body
	}
	// Fast path: if body already contains thought_signature, don't parse unless needed?
	// But we need to inject missing ones, so we must parse if it contains tool_calls
	if !bytes.Contains(body, []byte("tool_calls")) && !bytes.Contains(body, []byte("toolCalls")) && !bytes.Contains(body, []byte("functionCall")) {
		return body
	}
	var raw map[string]interface{}
	if err := json.Unmarshal(body, &raw); err != nil {
		return body
	}
	modified := false
	if msgs, ok := raw["messages"].([]interface{}); ok {
		for _, m := range msgs {
			mm, ok := m.(map[string]interface{})
			if !ok {
				continue
			}
			// Check for tool_calls in assistant messages
			tcs, ok := mm["tool_calls"].([]interface{})
			if !ok {
				// also check camelCase?
				tcs, _ = mm["toolCalls"].([]interface{})
				if tcs == nil {
					continue
				}
			}
			for _, tcRaw := range tcs {
				tcm, ok := tcRaw.(map[string]interface{})
				if !ok {
					continue
				}
				id, _ := tcm["id"].(string)
				if id == "" {
					continue
				}
				// Skip if already has signature
				if hasThoughtSignature(tcm) {
					continue
				}
				// Check nested function object too
				hasNested := false
				if fn, ok := tcm["function"].(map[string]interface{}); ok {
					if hasThoughtSignature(fn) {
						hasNested = true
					}
				}
				if hasNested {
					continue
				}
				if sig, ok := s.Load(id); ok {
					// Inject at top level, inside function, and extra_content.google for compatibility
					tcm["thought_signature"] = sig
					tcm["thoughtSignature"] = sig
					if fn, ok := tcm["function"].(map[string]interface{}); ok {
						fn["thought_signature"] = sig
						fn["thoughtSignature"] = sig
					}
					// Gemini 3.x via OpenAI compat expects extra_content.google.thought_signature
					injectIntoExtraContent(tcm, sig)
					modified = true
					slog.Debug("thought_signature: injected", "tool_call_id", id, "provider", provider)
				}
			}
		}
	}
	// Also handle Google native "contents" format if present (for gemini-vertex passthrough)
	if contents, ok := raw["contents"].([]interface{}); ok {
		for _, c := range contents {
			cm, ok := c.(map[string]interface{})
			if !ok {
				continue
			}
			parts, _ := cm["parts"].([]interface{})
			for _, p := range parts {
				pm, ok := p.(map[string]interface{})
				if !ok {
					continue
				}
				fc, _ := pm["functionCall"].(map[string]interface{})
				if fc == nil {
					fc, _ = pm["function_call"].(map[string]interface{})
				}
				if fc == nil {
					continue
				}
				id, _ := fc["id"].(string)
				if id == "" {
					id, _ = pm["id"].(string)
				}
				if id == "" {
					continue
				}
				if hasThoughtSignature(fc) || hasThoughtSignature(pm) {
					continue
				}
				if sig, ok := s.Load(id); ok {
					fc["thought_signature"] = sig
					fc["thoughtSignature"] = sig
					pm["thought_signature"] = sig
					pm["thoughtSignature"] = sig
					modified = true
				}
			}
		}
	}
	if !modified {
		return body
	}
	out, err := json.Marshal(raw)
	if err != nil {
		return body
	}
	slog.Info("thought_signature: injected missing signatures into request", "provider", provider, "injected", modified)
	return out
}

func injectIntoExtraContent(m map[string]interface{}, sig string) {
	ec, _ := m["extra_content"].(map[string]interface{})
	if ec == nil {
		ec = make(map[string]interface{})
		m["extra_content"] = ec
	}
	g, _ := ec["google"].(map[string]interface{})
	if g == nil {
		g = make(map[string]interface{})
		ec["google"] = g
	}
	g["thought_signature"] = sig
	g["thoughtSignature"] = sig
}

func hasThoughtSignature(m map[string]interface{}) bool {
	if v, ok := m["thought_signature"].(string); ok && v != "" {
		return true
	}
	if v, ok := m["thoughtSignature"].(string); ok && v != "" {
		return true
	}
	if getThoughtSignatureFromExtraContent(m) != "" {
		return true
	}
	return false
}
