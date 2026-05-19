package proxy

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"net/http"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	v4 "github.com/aws/aws-sdk-go-v2/aws/signer/v4"
)

// SigV4Signer implements cloudauth.RequestSigner for AWS services.
// It signs HTTP requests using AWS Signature Version 4, which is required
// for services like Bedrock that don't accept Bearer tokens.
type SigV4Signer struct {
	// Region is the AWS region (e.g., "us-east-1").
	Region string

	// Service is the AWS service name for signing (e.g., "bedrock").
	Service string

	// Credentials provides AWS access keys for signing.
	Credentials aws.CredentialsProvider
}

// SignRequest adds AWS SigV4 authentication headers to the request.
// This computes the SHA-256 hash of the request body and signs the
// request using the configured credentials, region, and service.
func (s *SigV4Signer) SignRequest(ctx context.Context, req *http.Request, body []byte) error {
	creds, err := s.Credentials.Retrieve(ctx)
	if err != nil {
		return fmt.Errorf("failed to retrieve AWS credentials: %w", err)
	}

	// Compute SHA-256 hash of the request body.
	hash := sha256.Sum256(body)
	payloadHash := fmt.Sprintf("%x", hash)

	// Ensure the body is readable for the signer.
	req.Body = http.NoBody
	if len(body) > 0 {
		req.Body = nopCloser(bytes.NewReader(body))
		req.ContentLength = int64(len(body))
	}

	signer := v4.NewSigner()
	return signer.SignHTTP(ctx, creds, req, payloadHash, s.Service, s.Region, time.Now())
}

// nopCloser wraps an io.Reader with a no-op Close method.
type nopReadCloser struct {
	*bytes.Reader
}

func (nopReadCloser) Close() error { return nil }

func nopCloser(r *bytes.Reader) *nopReadCloser {
	return &nopReadCloser{r}
}
