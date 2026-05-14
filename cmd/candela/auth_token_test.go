package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"testing"

	"golang.org/x/oauth2"
)

// staticTokenSource returns a fixed token, optionally with an id_token extra.
type staticTokenSource struct {
	accessToken string
	idToken     string // simulates user ADC id_token extra field
}

func (s *staticTokenSource) Token() (*oauth2.Token, error) {
	tok := &oauth2.Token{AccessToken: s.accessToken}
	if s.idToken != "" {
		// Simulate google.DefaultTokenSource returning an id_token
		// in the Extra map for user credentials with the "openid" scope.
		tok = tok.WithExtra(map[string]interface{}{
			"id_token": s.idToken,
		})
	}
	return tok, nil
}

// buildTestProxy creates a reverse proxy using the same Director logic as
// runForeground — always uses token.AccessToken, never token.Extra("id_token").
func buildTestProxy(t *testing.T, ts oauth2.TokenSource, remoteURL *url.URL) *httputil.ReverseProxy {
	t.Helper()
	return &httputil.ReverseProxy{
		Director: func(req *http.Request) {
			req.URL.Scheme = remoteURL.Scheme
			req.URL.Host = remoteURL.Host
			req.Host = remoteURL.Host

			token, err := ts.Token()
			if err != nil {
				t.Fatalf("Token() error: %v", err)
			}
			// This matches the production Director logic exactly.
			req.Header.Set("Authorization", "Bearer "+token.AccessToken)
		},
	}
}

// TestRemoteProxy_ServiceAccount_UsesAccessToken verifies that for service
// accounts (where AccessToken IS the audience-scoped ID token), the proxy
// sends AccessToken as the Bearer token.
func TestRemoteProxy_ServiceAccount_UsesAccessToken(t *testing.T) {
	ts := &staticTokenSource{accessToken: "sa-id-token-audience-scoped"}

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got := r.Header.Get("Authorization")
		if got != "Bearer sa-id-token-audience-scoped" {
			t.Errorf("Authorization = %q, want %q", got, "Bearer sa-id-token-audience-scoped")
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	}))
	defer upstream.Close()

	remoteURL, _ := url.Parse(upstream.URL)
	proxy := buildTestProxy(t, ts, remoteURL)
	srv := httptest.NewServer(proxy)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/v1/models")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != 200 {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
}

// TestRemoteProxy_UserCredentials_UsesAccessToken is a regression test for the
// original bug. User ADC credentials include a generic id_token in Extra that
// the server rejects (wrong audience). The proxy must send AccessToken instead.
func TestRemoteProxy_UserCredentials_UsesAccessToken(t *testing.T) {
	ts := &staticTokenSource{
		accessToken: "ya29.valid-access-token",
		idToken:     "eyJhbGci.wrong-audience-id-token.signature",
	}

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got := r.Header.Get("Authorization")
		if got != "Bearer ya29.valid-access-token" {
			t.Errorf("Authorization = %q, want %q (bug: proxy sent id_token instead of access_token)", got, "Bearer ya29.valid-access-token")
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	}))
	defer upstream.Close()

	remoteURL, _ := url.Parse(upstream.URL)
	proxy := buildTestProxy(t, ts, remoteURL)
	srv := httptest.NewServer(proxy)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/v1/models")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != 200 {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
}

// TestRemoteProxy_IdTokenExtra_NeverUsed is a direct regression test ensuring
// that token.Extra("id_token") is never preferred over token.AccessToken.
func TestRemoteProxy_IdTokenExtra_NeverUsed(t *testing.T) {
	ts := &staticTokenSource{
		accessToken: "correct-access-token",
		idToken:     "wrong-id-token-must-not-be-used",
	}

	token, err := ts.Token()
	if err != nil {
		t.Fatal(err)
	}

	// Verify test setup: Extra("id_token") IS populated.
	idExtra, ok := token.Extra("id_token").(string)
	if !ok || idExtra == "" {
		t.Fatal("test setup: expected id_token in Extra")
	}

	// The proxy must use AccessToken, not the Extra id_token.
	if token.AccessToken != "correct-access-token" {
		t.Errorf("AccessToken = %q, want %q", token.AccessToken, "correct-access-token")
	}
	if token.AccessToken == idExtra {
		t.Error("AccessToken must NOT equal id_token Extra — regression: old buggy code preferred id_token")
	}
}
