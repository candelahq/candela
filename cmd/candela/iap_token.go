package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"golang.org/x/oauth2"
)

// iapImpersonatingTokenSource generates IAP-compatible OIDC ID tokens by
// impersonating a service account via the IAM Credentials API.
//
// When candela-local runs with user ADC credentials (from `candela auth login`
// or `gcloud auth application-default login`), those credentials produce OAuth2
// access tokens — which IAP rejects. IAP requires an OIDC ID token with the
// IAP client ID as the audience.
//
// This token source solves the problem by using the user's access token to call
// IAM's generateIdToken endpoint, which returns an ID token scoped to the IAP
// audience. The user needs `roles/iam.serviceAccountTokenCreator` on the target
// service account (project owners/editors have this by default).
//
// Flow:
//
//	User ADC → access_token → IAM generateIdToken(SA, audience) → id_token → IAP ✅
type iapImpersonatingTokenSource struct {
	base           oauth2.TokenSource
	serviceAccount string
	audience       string
}

func (s *iapImpersonatingTokenSource) Token() (*oauth2.Token, error) {
	// Get the user's access token from ADC.
	baseToken, err := s.base.Token()
	if err != nil {
		return nil, fmt.Errorf("IAP: failed to get base credentials: %w", err)
	}

	// Call IAM Credentials API to generate an ID token with the IAP audience.
	url := fmt.Sprintf(
		"https://iamcredentials.googleapis.com/v1/projects/-/serviceAccounts/%s:generateIdToken",
		s.serviceAccount,
	)
	body := fmt.Sprintf(`{"audience":%q,"includeEmail":true}`, s.audience)

	req, err := http.NewRequest("POST", url, strings.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("IAP: failed to build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+baseToken.AccessToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("IAP: generateIdToken request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("IAP: generateIdToken returned %d: %s\n"+
			"Ensure you have roles/iam.serviceAccountTokenCreator on %s",
			resp.StatusCode, respBody, s.serviceAccount)
	}

	var result struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("IAP: failed to decode generateIdToken response: %w", err)
	}

	// Return the ID token as AccessToken so the Director sends it as Bearer.
	return &oauth2.Token{
		AccessToken: result.Token,
		TokenType:   "Bearer",
		Expiry:      time.Now().Add(3500 * time.Second), // ID tokens valid ~1 hour
	}, nil
}
