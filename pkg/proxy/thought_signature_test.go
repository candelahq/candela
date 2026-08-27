package proxy

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestThoughtSignature_StoreAndLoad(t *testing.T) {
	s := NewThoughtSignatureStore(0)
	s.Store("call_123", "SIG123")
	if v, ok := s.Load("call_123"); !ok || v != "SIG123" {
		t.Fatalf("Load failed, got %q %v", v, ok)
	}
	if _, ok := s.Load("missing"); ok {
		t.Fatalf("should not find missing")
	}
}

func TestThoughtSignature_ExtractAndStore_OpenAI(t *testing.T) {
	s := NewThoughtSignatureStore(0)
	body := []byte(`{"choices":[{"message":{"role":"assistant","tool_calls":[{"id":"call_abc","type":"function","function":{"name":"bash","arguments":"{}"},"thought_signature":"SIG_ABC"}]}}]}`)
	s.ExtractAndStoreFromResponse(body, "gemini-oai")
	if sig, ok := s.Load("call_abc"); !ok || sig != "SIG_ABC" {
		t.Fatalf("failed to extract thought_signature, got %q %v", sig, ok)
	}
	// Also test thoughtSignature camelCase
	body2 := []byte(`{"choices":[{"message":{"role":"assistant","tool_calls":[{"id":"call_def","type":"function","function":{"name":"bash","arguments":"{}"},"thoughtSignature":"SIG_DEF"}]}}]}`)
	s.ExtractAndStoreFromResponse(body2, "gemini-oai")
	if sig, ok := s.Load("call_def"); !ok || sig != "SIG_DEF" {
		t.Fatalf("failed to extract thoughtSignature camel, got %q %v", sig, ok)
	}
	// Also test nested function level
	body3 := []byte(`{"choices":[{"message":{"role":"assistant","tool_calls":[{"id":"call_ghi","type":"function","function":{"name":"bash","arguments":"{}","thought_signature":"SIG_GHI"}}]}}]}`)
	s.ExtractAndStoreFromResponse(body3, "gemini-oai")
	if sig, ok := s.Load("call_ghi"); !ok || sig != "SIG_GHI" {
		t.Fatalf("failed nested, got %q %v", sig, ok)
	}
}

func TestThoughtSignature_Extract_GoogleNative(t *testing.T) {
	s := NewThoughtSignatureStore(0)
	body := []byte(`{"candidates":[{"content":{"parts":[{"functionCall":{"name":"bash","args":{},"thoughtSignature":"GSIG","thought_signature":"GSIG2","id":"call_google"}}]}}]}`)
	s.ExtractAndStoreFromResponse(body, "gemini-vertex")
	if sig, ok := s.Load("call_google"); !ok || sig == "" {
		t.Fatalf("failed google native, got %q %v", sig, ok)
	}
}

func TestThoughtSignature_InjectIntoRequest(t *testing.T) {
	s := NewThoughtSignatureStore(0)
	s.Store("call_123", "SIG123")
	s.Store("call_456", "SIG456")
	req := []byte(`{"model":"gemini-3.7-flash","messages":[{"role":"user","content":"hi"},{"role":"assistant","tool_calls":[{"id":"call_123","type":"function","function":{"name":"bash","arguments":"{}"}}]},{"role":"assistant","tool_calls":[{"id":"call_456","type":"function","function":{"name":"bash","arguments":"{}"}}]}]}`)
	out := s.InjectIntoRequest(req, "gemini-oai")
	if strings.Contains(string(req), "thought_signature") {
		t.Fatalf("original should not contain sig")
	}
	if !strings.Contains(string(out), "SIG123") || !strings.Contains(string(out), "SIG456") {
		t.Fatalf("injected body missing sig: %s", string(out))
	}
	// Already has sig should not duplicate
	req2 := []byte(`{"model":"gemini-3.7-flash","messages":[{"role":"assistant","tool_calls":[{"id":"call_123","type":"function","function":{"name":"bash","arguments":"{}"},"thought_signature":"EXISTING"}]}]}`)
	out2 := s.InjectIntoRequest(req2, "gemini-oai")
	var m map[string]interface{}
	_ = json.Unmarshal(out2, &m)
	// Should keep EXISTING not overwrite
	if !strings.Contains(string(out2), "EXISTING") {
		t.Fatalf("should preserve existing")
	}
	if strings.Contains(string(out2), "SIG123") {
		t.Fatalf("should not overwrite existing")
	}
}

func TestThoughtSignature_InjectOnlyGemini(t *testing.T) {
	s := NewThoughtSignatureStore(0)
	s.Store("call_123", "SIG123")
	req := []byte(`{"model":"gpt-4o","messages":[{"role":"assistant","tool_calls":[{"id":"call_123","type":"function","function":{"name":"bash","arguments":"{}"}}]}]}`)
	out := s.InjectIntoRequest(req, "openai")
	if string(out) != string(req) {
		t.Fatalf("should not inject for openai")
	}
}

func TestThoughtSignature_StreamExtract(t *testing.T) {
	s := NewThoughtSignatureStore(0)
	stream := []byte("data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"id\":\"call_stream\",\"type\":\"function\",\"function\":{\"name\":\"bash\",\"arguments\":\"{}\"},\"thought_signature\":\"SIGSTREAM\"}]}}]}\n\ndata: [DONE]\n\n")
	s.ExtractAndStoreFromStream(stream, "gemini-oai")
	if sig, ok := s.Load("call_stream"); !ok || sig != "SIGSTREAM" {
		t.Fatalf("stream extract failed %q %v", sig, ok)
	}
}

func TestThoughtSignature_StoreFromGoogleStreamArray(t *testing.T) {
	s := NewThoughtSignatureStore(0)
	stream := []byte(`[{"candidates":[{"content":{"parts":[{"functionCall":{"name":"bash","args":{},"thoughtSignature":"GSIGSTREAM","id":"call_gs"}}]}}]}]`)
	s.ExtractAndStoreFromStream(stream, "google")
	if sig, ok := s.Load("call_gs"); !ok || sig != "GSIGSTREAM" {
		t.Fatalf("google stream failed %q %v", sig, ok)
	}
}
