package proxy

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// mockFailingTranslator is a FormatTranslator that fails on TranslateResponse.
type mockFailingTranslator struct {
	TranslateResponseErr error
}

func (m *mockFailingTranslator) TranslateRequest(body []byte) ([]byte, string, error) {
	return body, "test-model", nil
}

func (m *mockFailingTranslator) TranslateResponse(body []byte, model string) ([]byte, error) {
	if m.TranslateResponseErr != nil {
		return nil, m.TranslateResponseErr
	}
	return []byte(`{"translated":true}`), nil
}

func (m *mockFailingTranslator) TranslateStreamChunk(chunk []byte, model string, streamID *string) ([]byte, error) {
	return chunk, nil
}

func TestResponseTranslationFailureReturns502(t *testing.T) {
	// Upstream returns 200 OK with some arbitrary body
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"unexpected":"format"}`))
	}))
	defer upstream.Close()

	translator := &mockFailingTranslator{
		TranslateResponseErr: fmt.Errorf("malformed upstream response schema"),
	}

	submitter := &mockSubmitter{}
	calc := newCalcWithTestModels()

	p, err := New(Config{
		Providers: []Provider{{
			Name:             "custom-translator-provider",
			UpstreamURL:      upstream.URL,
			FormatTranslator: translator,
		}},
		ProjectID: "test-project",
	}, submitter, calc)
	if err != nil {
		t.Fatalf("failed to create proxy: %v", err)
	}

	mux := http.NewServeMux()
	p.RegisterRoutes(mux)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	req, err := http.NewRequest(
		"POST",
		srv.URL+"/proxy/custom-translator-provider/v1/chat/completions",
		strings.NewReader(`{"model":"test-model","messages":[{"role":"user","content":"hello"}]}`),
	)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("failed to read response body: %v", err)
	}

	// Acceptance criteria: Must return HTTP 502 Bad Gateway
	if resp.StatusCode != http.StatusBadGateway {
		t.Fatalf("status = %d, want %d (HTTP 502 Bad Gateway); body: %s", resp.StatusCode, http.StatusBadGateway, string(bodyBytes))
	}

	// Content-Type must be application/json
	if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}

	// Body must be OpenAI-compatible error
	var errResp openAIErrorResponse
	if err := json.Unmarshal(bodyBytes, &errResp); err != nil {
		t.Fatalf("failed to parse error response JSON: %v, body: %s", err, string(bodyBytes))
	}

	if errResp.Error.Type != "bad_gateway" {
		t.Errorf("error.type = %q, want 'bad_gateway'", errResp.Error.Type)
	}
	if errResp.Error.Code != "502" {
		t.Errorf("error.code = %q, want '502'", errResp.Error.Code)
	}
	if !strings.Contains(errResp.Error.Message, "malformed upstream response schema") {
		t.Errorf("error.message = %q, expected substring 'malformed upstream response schema'", errResp.Error.Message)
	}
}

func TestResponseTranslationSuccessReturns200(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"upstream":"payload"}`))
	}))
	defer upstream.Close()

	translator := &mockFailingTranslator{
		TranslateResponseErr: nil, // success
	}

	submitter := &mockSubmitter{}
	calc := newCalcWithTestModels()

	p, err := New(Config{
		Providers: []Provider{{
			Name:             "custom-translator-provider",
			UpstreamURL:      upstream.URL,
			FormatTranslator: translator,
		}},
		ProjectID: "test-project",
	}, submitter, calc)
	if err != nil {
		t.Fatalf("failed to create proxy: %v", err)
	}

	mux := http.NewServeMux()
	p.RegisterRoutes(mux)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	req, err := http.NewRequest(
		"POST",
		srv.URL+"/proxy/custom-translator-provider/v1/chat/completions",
		strings.NewReader(`{"model":"test-model","messages":[{"role":"user","content":"hello"}]}`),
	)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("failed to read response body: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	if string(bodyBytes) != `{"translated":true}` {
		t.Errorf("body = %q, want %q", string(bodyBytes), `{"translated":true}`)
	}
}
