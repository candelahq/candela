// Package cloudauth provides a pluggable cloud credential management system.
//
// It abstracts the login, status, and token operations for different cloud
// providers (GCP, AWS, Azure) behind a common interface. The CLI commands
// (candela auth login/status/token) delegate to the appropriate provider
// implementation based on configuration or explicit --provider flag.
package cloudauth

import (
	"context"
	"net/http"
	"time"

	"golang.org/x/oauth2"
)

// Provider abstracts cloud authentication for a specific cloud platform.
type Provider interface {
	// Name returns the provider identifier (e.g., "gcp", "aws", "azure").
	Name() string

	// Login performs the interactive login flow (browser OAuth, SSO, etc.).
	// It should write credentials to the appropriate location on disk.
	Login(ctx context.Context) error

	// Status returns the current credential state, or an error if
	// credentials cannot be determined.
	Status(ctx context.Context) (*CredentialStatus, error)

	// TokenSource returns a reusable token source for API calls.
	// This is used for cloud providers that use Bearer token auth (e.g., GCP).
	// Returns nil for providers that use request signing (e.g., AWS SigV4).
	TokenSource(ctx context.Context, scopes ...string) (oauth2.TokenSource, error)

	// IsConfigured returns true if credentials exist on disk.
	IsConfigured() bool
}

// RequestSigner signs an outbound HTTP request for providers that use
// request-level signing rather than Bearer tokens (e.g., AWS SigV4).
type RequestSigner interface {
	// SignRequest adds authentication headers/signatures to the request.
	SignRequest(ctx context.Context, req *http.Request, body []byte) error
}

// CredentialStatus describes the current state of cloud credentials.
type CredentialStatus struct {
	Provider  string        // "gcp", "aws", "azure"
	Account   string        // email, ARN, principal, etc.
	ExpiresIn time.Duration // time until expiry (negative = expired)
	Valid     bool          // true if credentials can be refreshed
	FilePath  string        // where credentials are stored on disk
}
