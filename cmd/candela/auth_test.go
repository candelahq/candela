package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/oauth2"
)

func TestWriteAndReadADC(t *testing.T) {
	// Use a temp dir to avoid touching real credentials.
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "application_default_credentials.json")

	token := &oauth2.Token{
		RefreshToken: "test-refresh-token-12345",
	}

	creds := adcCredentials{
		Account:      "",
		ClientID:     oauthConfig.ClientID,
		ClientSecret: oauthConfig.ClientSecret,
		RefreshToken: token.RefreshToken,
		Type:         "authorized_user",
	}

	data, err := json.MarshalIndent(creds, "", "  ")
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	// Read it back.
	got, err := readADC(path)
	if err != nil {
		t.Fatalf("readADC: %v", err)
	}

	if got.Type != "authorized_user" {
		t.Errorf("type = %q, want %q", got.Type, "authorized_user")
	}
	if got.RefreshToken != "test-refresh-token-12345" {
		t.Errorf("refresh_token = %q, want %q", got.RefreshToken, "test-refresh-token-12345")
	}
	if got.ClientID != oauthConfig.ClientID {
		t.Errorf("client_id = %q, want %q", got.ClientID, oauthConfig.ClientID)
	}
}

func TestReadADC_MissingFile(t *testing.T) {
	_, err := readADC("/nonexistent/path/to/adc.json")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestReadADC_InvalidJSON(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "bad.json")
	if err := os.WriteFile(path, []byte("not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := readADC(path)
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestReadADC_NoRefreshToken(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "no_refresh.json")
	data := `{"type":"authorized_user","client_id":"id","client_secret":"secret"}`
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := readADC(path)
	if err == nil {
		t.Fatal("expected error for missing refresh token")
	}
}

func TestExtractEmail(t *testing.T) {
	// Token with no id_token extra → "unknown".
	token := &oauth2.Token{AccessToken: "test"}
	if got := extractEmail(token); got != "unknown" {
		t.Errorf("extractEmail(no id_token) = %q, want %q", got, "unknown")
	}
}

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		d    time.Duration
		want string
	}{
		{-time.Second, "expired"},
		{30 * time.Second, "< 1 min"},
		{5 * time.Minute, "5 min"},
		{45 * time.Minute, "45 min"},
		{90 * time.Minute, "1h 30m"},
		{2*time.Hour + 15*time.Minute, "2h 15m"},
	}
	for _, tt := range tests {
		if got := formatDuration(tt.d); got != tt.want {
			t.Errorf("formatDuration(%v) = %q, want %q", tt.d, got, tt.want)
		}
	}
}

func TestAdcPath(t *testing.T) {
	path, err := adcPath()
	if err != nil {
		t.Fatalf("adcPath: %v", err)
	}
	if !filepath.IsAbs(path) {
		t.Errorf("adcPath returned relative path: %s", path)
	}
	if !strings.Contains(path, "application_default_credentials.json") {
		t.Errorf("adcPath missing expected filename: %s", path)
	}
}

func TestRefreshFromADC_InvalidCreds(t *testing.T) {
	// Mock a token endpoint that returns an error.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"invalid_grant","error_description":"Token has been revoked"}`))
	}))
	defer server.Close()

	// refreshFromADC uses google.Endpoint which points to real Google servers,
	// so this test validates that an invalid refresh token returns an error
	// (the actual HTTP call to Google will fail with invalid credentials).
	creds := &adcCredentials{
		ClientID:     "fake-client-id",
		ClientSecret: "fake-secret",
		RefreshToken: "fake-refresh-token",
	}
	_, err := refreshFromADC(creds)
	if err == nil {
		t.Fatal("expected error for invalid credentials")
	}
}

func TestHandleAuth_Help(t *testing.T) {
	// Just verify handleAuth doesn't panic with empty args.
	// It writes to stderr but shouldn't crash.
	handleAuth(nil)
}
