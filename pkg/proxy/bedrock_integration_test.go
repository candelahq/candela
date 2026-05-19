package proxy

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"

	"github.com/candelahq/candela/pkg/costcalc"
)

// TestBedrockProxyIntegration is an end-to-end integration test that verifies
// the full Bedrock proxy chain: request → path rewrite → SigV4 signing → upstream.
//
// It uses a mock upstream server to capture the request and assert that:
//   - The request path was rewritten to /model/{modelId}/invoke
//   - SigV4 signing headers (Authorization, X-Amz-Date) were injected
//   - The request body was forwarded intact
//   - The response was returned to the client
func TestBedrockProxyIntegration(t *testing.T) {
	// Track what the upstream received.
	var capturedPath string
	var capturedAuth string
	var capturedAmzDate string
	var capturedBody []byte

	// Start a mock upstream that captures headers and returns a Claude-like response.
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		capturedAuth = r.Header.Get("Authorization")
		capturedAmzDate = r.Header.Get("X-Amz-Date")

		var err error
		capturedBody, err = io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("upstream: failed to read body: %v", err)
		}
		_ = r.Body.Close()

		// Return a minimal Anthropic-style response.
		w.Header().Set("Content-Type", "application/json")
		resp := map[string]interface{}{
			"id":   "msg_test",
			"type": "message",
			"role": "assistant",
			"content": []map[string]interface{}{
				{"type": "text", "text": "Hello from Bedrock!"},
			},
			"model":         "anthropic.claude-3-5-sonnet-20241022-v2:0",
			"stop_reason":   "end_turn",
			"stop_sequence": nil,
			"usage":         map[string]int{"input_tokens": 10, "output_tokens": 5},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer upstream.Close()

	// Create a SigV4Signer with fake credentials.
	signer := &SigV4Signer{
		Region:  "us-east-1",
		Service: "bedrock",
		Credentials: aws.CredentialsProviderFunc(func(ctx context.Context) (aws.Credentials, error) {
			return aws.Credentials{
				AccessKeyID:     "AKIAIOSFODNN7EXAMPLE",
				SecretAccessKey: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
				Source:          "test",
			}, nil
		}),
	}

	// Build the proxy with an anthropic-bedrock provider pointing at our mock upstream.
	calc := costcalc.New()
	// Register pricing for Bedrock model IDs (different naming scheme than Vertex/direct).
	calc.SetPricing(costcalc.ModelPricing{
		Provider: "anthropic", Model: "anthropic.claude-3-5-sonnet-20241022-v2:0",
		InputPerMillion: 3.00, OutputPerMillion: 15.00,
	})
	p := New(Config{
		ProjectID: "test",
		Providers: []Provider{
			{
				Name:          "anthropic-bedrock",
				UpstreamURL:   upstream.URL,
				PathRewriter:  &BedrockPathRewriter{},
				RequestSigner: signer,
			},
		},
	}, nil, calc)

	// Register routes on a test mux.
	mux := http.NewServeMux()
	p.RegisterRoutes(mux)

	// Build a request that simulates what Claude Code would send.
	reqBody := `{
		"model": "anthropic.claude-3-5-sonnet-20241022-v2:0",
		"messages": [{"role": "user", "content": "Hello"}],
		"max_tokens": 100
	}`

	req := httptest.NewRequest("POST", "/proxy/anthropic-bedrock/v1/messages", strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")

	// Execute.
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	// ── Assertions ──

	// 1. Response should be 200 OK.
	if w.Code != http.StatusOK {
		t.Fatalf("response status = %d, want 200; body: %s", w.Code, w.Body.String())
	}

	// 2. Upstream path should be rewritten to Bedrock's invoke format.
	wantPath := "/model/anthropic.claude-3-5-sonnet-20241022-v2:0/invoke"
	if capturedPath != wantPath {
		t.Errorf("upstream path = %q, want %q", capturedPath, wantPath)
	}

	// 3. Authorization header should have AWS SigV4 signature.
	if capturedAuth == "" {
		t.Error("upstream missing Authorization header")
	} else if !strings.HasPrefix(capturedAuth, "AWS4-HMAC-SHA256") {
		t.Errorf("upstream Authorization = %q, want AWS4-HMAC-SHA256 prefix", capturedAuth[:min(50, len(capturedAuth))])
	}

	// 4. X-Amz-Date header should be present.
	if capturedAmzDate == "" {
		t.Error("upstream missing X-Amz-Date header")
	}

	// 5. Signature should reference the bedrock service.
	if !strings.Contains(capturedAuth, "bedrock") {
		t.Errorf("SigV4 signature doesn't reference bedrock service")
	}

	// 6. Request body should contain the original messages.
	if !strings.Contains(string(capturedBody), `"Hello"`) {
		t.Errorf("upstream body missing original message content: %s", string(capturedBody))
	}

	// 7. Response body should be the mock Claude response.
	var respBody map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &respBody); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	if respBody["id"] != "msg_test" {
		t.Errorf("response id = %v, want msg_test", respBody["id"])
	}
}

// TestBedrockProxyIntegration_SignerError verifies the proxy returns 500
// when AWS credentials cannot be retrieved.
func TestBedrockProxyIntegration_SignerError(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("upstream should not be called when signing fails")
	}))
	defer upstream.Close()

	signer := &SigV4Signer{
		Region:  "us-east-1",
		Service: "bedrock",
		Credentials: aws.CredentialsProviderFunc(func(ctx context.Context) (aws.Credentials, error) {
			return aws.Credentials{}, context.DeadlineExceeded
		}),
	}

	calc := costcalc.New()
	calc.SetPricing(costcalc.ModelPricing{
		Provider: "anthropic", Model: "anthropic.claude-3-5-sonnet-20241022-v2:0",
		InputPerMillion: 3.00, OutputPerMillion: 15.00,
	})
	p := New(Config{
		ProjectID: "test",
		Providers: []Provider{
			{
				Name:          "anthropic-bedrock",
				UpstreamURL:   upstream.URL,
				PathRewriter:  &BedrockPathRewriter{},
				RequestSigner: signer,
			},
		},
	}, nil, calc)

	mux := http.NewServeMux()
	p.RegisterRoutes(mux)

	reqBody := `{"model":"anthropic.claude-3-5-sonnet-20241022-v2:0","messages":[{"role":"user","content":"Hi"}],"max_tokens":10}`
	req := httptest.NewRequest("POST", "/proxy/anthropic-bedrock/v1/messages", strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	// Should return 500 because signing failed.
	if w.Code != http.StatusInternalServerError {
		t.Errorf("response status = %d, want 500; body: %s", w.Code, w.Body.String())
	}
}

// TestBedrockProxyIntegration_StreamingPath verifies that streaming requests
// get routed to the invoke-with-response-stream endpoint.
func TestBedrockProxyIntegration_StreamingPath(t *testing.T) {
	var capturedPath string

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		w.Header().Set("Content-Type", "text/event-stream")
		// Return a minimal SSE response.
		_, _ = w.Write([]byte("event: message_start\ndata: {\"type\":\"message_start\"}\n\n"))
	}))
	defer upstream.Close()

	signer := &SigV4Signer{
		Region:  "us-east-1",
		Service: "bedrock",
		Credentials: aws.CredentialsProviderFunc(func(ctx context.Context) (aws.Credentials, error) {
			return aws.Credentials{
				AccessKeyID:     "AKIAIOSFODNN7EXAMPLE",
				SecretAccessKey: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
				Source:          "test",
			}, nil
		}),
	}

	calc := costcalc.New()
	calc.SetPricing(costcalc.ModelPricing{
		Provider: "anthropic", Model: "anthropic.claude-3-5-sonnet-20241022-v2:0",
		InputPerMillion: 3.00, OutputPerMillion: 15.00,
	})
	p := New(Config{
		ProjectID: "test",
		Providers: []Provider{
			{
				Name:          "anthropic-bedrock",
				UpstreamURL:   upstream.URL,
				PathRewriter:  &BedrockPathRewriter{},
				RequestSigner: signer,
			},
		},
	}, nil, calc)

	mux := http.NewServeMux()
	p.RegisterRoutes(mux)

	// Anthropic streaming uses "stream": true in the body.
	reqBody := `{"model":"anthropic.claude-3-5-sonnet-20241022-v2:0","messages":[{"role":"user","content":"Hi"}],"max_tokens":10,"stream":true}`
	req := httptest.NewRequest("POST", "/proxy/anthropic-bedrock/v1/messages", strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	// Upstream path should use invoke-with-response-stream for streaming.
	wantPath := "/model/anthropic.claude-3-5-sonnet-20241022-v2:0/invoke-with-response-stream"
	if capturedPath != wantPath {
		t.Errorf("streaming path = %q, want %q", capturedPath, wantPath)
	}
}
