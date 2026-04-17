package proxy

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/candelahq/candela/pkg/costcalc"
)

func TestBuildModelsResponse(t *testing.T) {
	models := []CompatModel{
		{ID: "gpt-4o", Provider: "openai"},
		{ID: "claude-sonnet-4-20250514", Provider: "anthropic"},
		{ID: "gemini-2.5-pro", Provider: "gemini-oai"},
	}

	data := buildModelsResponse(models)

	var resp struct {
		Object string `json:"object"`
		Data   []struct {
			ID      string `json:"id"`
			Object  string `json:"object"`
			Created int64  `json:"created"`
			OwnedBy string `json:"owned_by"`
		} `json:"data"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if resp.Object != "list" {
		t.Errorf("object = %q, want 'list'", resp.Object)
	}
	if len(resp.Data) != 3 {
		t.Fatalf("len(data) = %d, want 3", len(resp.Data))
	}

	// Verify each model entry.
	expected := []struct {
		id, ownedBy string
	}{
		{"gpt-4o", "openai"},
		{"claude-sonnet-4-20250514", "anthropic"},
		{"gemini-2.5-pro", "gemini-oai"},
	}
	for i, want := range expected {
		got := resp.Data[i]
		if got.ID != want.id {
			t.Errorf("data[%d].id = %q, want %q", i, got.ID, want.id)
		}
		if got.OwnedBy != want.ownedBy {
			t.Errorf("data[%d].owned_by = %q, want %q", i, got.OwnedBy, want.ownedBy)
		}
		if got.Object != "model" {
			t.Errorf("data[%d].object = %q, want 'model'", i, got.Object)
		}
	}
}

func TestBuildModelsResponse_Empty(t *testing.T) {
	data := buildModelsResponse(nil)

	var resp struct {
		Object string        `json:"object"`
		Data   []interface{} `json:"data"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if resp.Object != "list" {
		t.Errorf("object = %q, want 'list'", resp.Object)
	}
	// Data should be null or empty — either is valid for the OpenAI API.
}

func TestCompatRoutes_GetModels(t *testing.T) {
	submitter := &mockSubmitter{}
	calc := costcalc.New()

	p := New(Config{
		Providers: []Provider{{Name: "openai", UpstreamURL: "http://unused"}},
		ProjectID: "test",
	}, submitter, calc)

	models := []CompatModel{
		{ID: "gpt-4o", Provider: "openai"},
		{ID: "gpt-4o-mini", Provider: "openai"},
	}

	mux := http.NewServeMux()
	p.RegisterRoutes(mux)
	p.RegisterCompatRoutes(mux, models)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/v1/models")
	if err != nil {
		t.Fatalf("GET /v1/models failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != 200 {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}

	if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
		t.Errorf("content-type = %q, want application/json", ct)
	}

	var result struct {
		Object string `json:"object"`
		Data   []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode error: %v", err)
	}

	if len(result.Data) != 2 {
		t.Fatalf("len(data) = %d, want 2", len(result.Data))
	}
	if result.Data[0].ID != "gpt-4o" {
		t.Errorf("data[0].id = %q, want gpt-4o", result.Data[0].ID)
	}
}

func TestCompatRoutes_ChatCompletions_RoutesToProvider(t *testing.T) {
	// Fake upstream that returns an OpenAI-format response.
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{
			"id": "chatcmpl-test",
			"object": "chat.completion",
			"choices": [{"message": {"role": "assistant", "content": "Hello from proxy!"}}],
			"usage": {"prompt_tokens": 5, "completion_tokens": 7, "total_tokens": 12}
		}`)
	}))
	defer upstream.Close()

	submitter := &mockSubmitter{}
	calc := costcalc.New()

	p := New(Config{
		Providers: []Provider{{Name: "openai", UpstreamURL: upstream.URL}},
		ProjectID: "test",
	}, submitter, calc)

	models := []CompatModel{
		{ID: "gpt-4o", Provider: "openai"},
	}

	mux := http.NewServeMux()
	p.RegisterRoutes(mux)
	p.RegisterCompatRoutes(mux, models)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	// POST to /v1/chat/completions (LM Studio path).
	body := `{"model": "gpt-4o", "messages": [{"role": "user", "content": "hello"}]}`
	req, _ := http.NewRequest("POST", srv.URL+"/v1/chat/completions", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer test-key")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != 200 {
		respBody, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, body = %s", resp.StatusCode, respBody)
	}

	var result map[string]interface{}
	_ = json.NewDecoder(resp.Body).Decode(&result)

	if result["id"] != "chatcmpl-test" {
		t.Errorf("response id = %v, want chatcmpl-test", result["id"])
	}

	// Wait for async span creation.
	for i := 0; i < 50; i++ {
		if len(submitter.getSpans()) > 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	spans := submitter.getSpans()
	if len(spans) == 0 {
		t.Fatal("expected span to be submitted via compat route")
	}

	span := spans[0]
	if span.GenAI == nil {
		t.Fatal("expected GenAI attributes")
	}
	if span.GenAI.Model != "gpt-4o" {
		t.Errorf("span model = %q, want gpt-4o", span.GenAI.Model)
	}
	if span.GenAI.Provider != "openai" {
		t.Errorf("span provider = %q, want openai", span.GenAI.Provider)
	}
}

func TestCompatRoutes_ChatCompletions_UnknownModel(t *testing.T) {
	submitter := &mockSubmitter{}
	calc := costcalc.New()

	p := New(Config{
		Providers: []Provider{{Name: "openai", UpstreamURL: "http://unused"}},
		ProjectID: "test",
	}, submitter, calc)

	models := []CompatModel{
		{ID: "gpt-4o", Provider: "openai"},
	}

	mux := http.NewServeMux()
	p.RegisterRoutes(mux)
	p.RegisterCompatRoutes(mux, models)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	// Request with a model not in the configured list.
	body := `{"model": "unknown-model-xyz", "messages": [{"role": "user", "content": "hi"}]}`
	req, _ := http.NewRequest("POST", srv.URL+"/v1/chat/completions", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != 400 {
		t.Errorf("status = %d, want 400", resp.StatusCode)
	}

	respBody, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(respBody), "unknown-model-xyz") {
		t.Errorf("error body should mention the unknown model, got: %s", respBody)
	}
}

func TestCompatRoutes_ChatCompletions_MissingModel(t *testing.T) {
	submitter := &mockSubmitter{}
	calc := costcalc.New()

	p := New(Config{
		Providers: []Provider{{Name: "openai", UpstreamURL: "http://unused"}},
		ProjectID: "test",
	}, submitter, calc)

	mux := http.NewServeMux()
	p.RegisterRoutes(mux)
	p.RegisterCompatRoutes(mux, []CompatModel{{ID: "gpt-4o", Provider: "openai"}})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	// Request with no model field.
	body := `{"messages": [{"role": "user", "content": "hi"}]}`
	req, _ := http.NewRequest("POST", srv.URL+"/v1/chat/completions", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != 400 {
		t.Errorf("status = %d, want 400", resp.StatusCode)
	}

	respBody, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(respBody), "missing or invalid model") {
		t.Errorf("error body = %s", respBody)
	}
}

func TestCompatRoutes_ChatCompletions_InvalidJSON(t *testing.T) {
	submitter := &mockSubmitter{}
	calc := costcalc.New()

	p := New(Config{
		Providers: []Provider{{Name: "openai", UpstreamURL: "http://unused"}},
		ProjectID: "test",
	}, submitter, calc)

	mux := http.NewServeMux()
	p.RegisterRoutes(mux)
	p.RegisterCompatRoutes(mux, []CompatModel{{ID: "gpt-4o", Provider: "openai"}})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	// Send garbage body.
	req, _ := http.NewRequest("POST", srv.URL+"/v1/chat/completions", strings.NewReader("not json"))
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != 400 {
		t.Errorf("status = %d, want 400", resp.StatusCode)
	}
}
