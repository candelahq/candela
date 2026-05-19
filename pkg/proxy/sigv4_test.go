package proxy

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
)

func TestSigV4Signer_SignRequest(t *testing.T) {
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

	body := []byte(`{"prompt":"Hello"}`)
	req, err := http.NewRequestWithContext(context.Background(), "POST",
		"https://bedrock-runtime.us-east-1.amazonaws.com/model/anthropic.claude-3-5-sonnet/invoke",
		strings.NewReader(string(body)))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")

	if err := signer.SignRequest(context.Background(), req, body); err != nil {
		t.Fatalf("SignRequest: %v", err)
	}

	// After signing, the request must have an Authorization header.
	auth := req.Header.Get("Authorization")
	if auth == "" {
		t.Error("missing Authorization header after signing")
	}
	if !strings.HasPrefix(auth, "AWS4-HMAC-SHA256") {
		t.Errorf("Authorization = %q, want prefix AWS4-HMAC-SHA256", auth)
	}

	// Must have X-Amz-Date header.
	if req.Header.Get("X-Amz-Date") == "" {
		t.Error("missing X-Amz-Date header after signing")
	}

	// Signature must reference the bedrock service.
	if !strings.Contains(auth, "bedrock") {
		t.Errorf("Authorization doesn't reference bedrock service: %s", auth)
	}
}

func TestSigV4Signer_SignRequest_EmptyBody(t *testing.T) {
	signer := &SigV4Signer{
		Region:  "us-west-2",
		Service: "bedrock",
		Credentials: aws.CredentialsProviderFunc(func(ctx context.Context) (aws.Credentials, error) {
			return aws.Credentials{
				AccessKeyID:     "AKIAIOSFODNN7EXAMPLE",
				SecretAccessKey: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
				Source:          "test",
			}, nil
		}),
	}

	req, err := http.NewRequestWithContext(context.Background(), "GET",
		"https://bedrock-runtime.us-west-2.amazonaws.com/model/test/invoke",
		nil)
	if err != nil {
		t.Fatal(err)
	}

	if err := signer.SignRequest(context.Background(), req, nil); err != nil {
		t.Fatalf("SignRequest with empty body: %v", err)
	}

	if req.Header.Get("Authorization") == "" {
		t.Error("missing Authorization header for empty-body request")
	}
}

func TestSigV4Signer_SignRequest_CredentialError(t *testing.T) {
	signer := &SigV4Signer{
		Region:  "us-east-1",
		Service: "bedrock",
		Credentials: aws.CredentialsProviderFunc(func(ctx context.Context) (aws.Credentials, error) {
			return aws.Credentials{}, context.DeadlineExceeded
		}),
	}

	req, _ := http.NewRequestWithContext(context.Background(), "POST",
		"https://bedrock-runtime.us-east-1.amazonaws.com/model/test/invoke",
		nil)

	err := signer.SignRequest(context.Background(), req, []byte("{}"))
	if err == nil {
		t.Error("expected error when credentials fail")
	}
}
