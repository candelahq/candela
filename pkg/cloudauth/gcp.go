package cloudauth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

// GCPProvider implements Provider for Google Cloud Platform.
// It performs browser-based OAuth2 login and writes/reads Application Default
// Credentials (ADC) files compatible with gcloud CLI.
type GCPProvider struct {
	// oauthConfig is the OAuth2 configuration used for the login flow.
	// If nil, defaults to Google's well-known CLI client.
	oauthConfig *oauth2.Config
}

// NewGCPProvider creates a new GCP credential provider using Google's
// well-known OAuth client for CLI tools (the same client_id used by
// gcloud auth application-default login).
func NewGCPProvider() *GCPProvider {
	return &GCPProvider{
		oauthConfig: &oauth2.Config{
			ClientID:     "764086051850-6qr4p6gpi6hn506pt8ejuq83di341hur.apps.googleusercontent.com",
			ClientSecret: "d-FL95Q19q7MQmFpd7hHD0Ty",
			Scopes: []string{
				"openid",
				"https://www.googleapis.com/auth/userinfo.email",
				"https://www.googleapis.com/auth/cloud-platform",
			},
			Endpoint: google.Endpoint,
		},
	}
}

// Name returns "gcp".
func (g *GCPProvider) Name() string { return "gcp" }

// adcCredentials represents the Application Default Credentials file format.
// Compatible with the structure that gcloud auth application-default login writes.
type adcCredentials struct {
	Account      string `json:"account"`
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
	QuotaProject string `json:"quota_project_id,omitempty"`
	RefreshToken string `json:"refresh_token"`
	Type         string `json:"type"`
}

// authResult carries the OAuth2 callback result from the local HTTP server.
type authResult struct {
	code string
	err  error
}

// ADCPath returns the standard Application Default Credentials file path.
// Supports CLOUDSDK_CONFIG override, Windows (APPDATA), and Unix (~/.config).
func ADCPath() (string, error) {
	// Respect CLOUDSDK_CONFIG if set (same as gcloud).
	if p := os.Getenv("CLOUDSDK_CONFIG"); p != "" {
		return filepath.Join(p, "application_default_credentials.json"), nil
	}
	if runtime.GOOS == "windows" {
		appData := os.Getenv("APPDATA")
		if appData == "" {
			return "", fmt.Errorf("APPDATA environment variable not set")
		}
		return filepath.Join(appData, "gcloud", "application_default_credentials.json"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot determine home directory: %w", err)
	}
	return filepath.Join(home, ".config", "gcloud", "application_default_credentials.json"), nil
}

// IsConfigured returns true if ADC credentials exist on disk.
func (g *GCPProvider) IsConfigured() bool {
	path, err := ADCPath()
	if err != nil {
		return false
	}
	_, err = os.Stat(path)
	return err == nil
}

// Login performs the OAuth2 authorization code flow via browser.
// It starts a temporary local HTTP server, opens the browser for Google OAuth
// consent, exchanges the auth code for tokens, and writes ADC credentials.
func (g *GCPProvider) Login(ctx context.Context) error {
	// Start a temporary HTTP server on a random port.
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return fmt.Errorf("failed to start local server: %w", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	redirectURL := fmt.Sprintf("http://127.0.0.1:%d/callback", port)
	g.oauthConfig.RedirectURL = redirectURL

	// Generate a random state parameter for CSRF protection.
	stateBytes := make([]byte, 16)
	if _, err := rand.Read(stateBytes); err != nil {
		_ = listener.Close()
		return fmt.Errorf("failed to generate random state: %w", err)
	}
	state := base64.URLEncoding.EncodeToString(stateBytes)

	// Channel to receive the auth result.
	resultCh := make(chan authResult, 1)

	// Set up the callback handler.
	mux := http.NewServeMux()
	mux.HandleFunc("/callback", handleOAuthCallback(state, resultCh))

	server := &http.Server{Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	go func() { _ = server.Serve(listener) }()
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()

	// Build authorization URL and open browser.
	authURL := g.oauthConfig.AuthCodeURL(state, oauth2.AccessTypeOffline)

	fmt.Println("Opening browser to authenticate...")
	fmt.Printf("If the browser doesn't open, visit:\n  %s\n\n", authURL)

	if err := openBrowser(authURL); err != nil {
		fmt.Fprintf(os.Stderr, "warning: could not open browser: %v\n", err)
	}

	// Wait for the callback (with timeout).
	fmt.Println("Waiting for authentication...")

	timeoutCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	select {
	case result := <-resultCh:
		if result.err != nil {
			return result.err
		}

		// Exchange the authorization code for tokens.
		exchangeCtx, exchangeCancel := context.WithTimeout(ctx, 30*time.Second)
		defer exchangeCancel()

		token, err := g.oauthConfig.Exchange(exchangeCtx, result.code)
		if err != nil {
			return fmt.Errorf("failed to exchange authorization code: %w", err)
		}

		// Write credentials to the standard ADC path.
		if err := g.writeADC(token); err != nil {
			return fmt.Errorf("failed to write credentials: %w", err)
		}

		// Display the authenticated identity.
		email := ExtractEmail(token)
		path, _ := ADCPath()
		fmt.Printf("\n🕯️  Authenticated as: %s\n", email)
		fmt.Printf("   Credentials saved to: %s\n", path)
		return nil

	case <-timeoutCtx.Done():
		return fmt.Errorf("authentication timed out (5 minutes)")
	}
}

// Status returns the current credential state.
func (g *GCPProvider) Status(ctx context.Context) (*CredentialStatus, error) {
	path, err := ADCPath()
	if err != nil {
		return nil, err
	}

	creds, err := ReadADC(path)
	if err != nil {
		return &CredentialStatus{
			Provider: "gcp",
			Valid:    false,
			FilePath: path,
		}, nil
	}

	// Try to refresh and get identity info.
	token, err := RefreshFromADC(creds)
	if err != nil {
		return &CredentialStatus{
			Provider: "gcp",
			Valid:    false,
			FilePath: path,
			Account:  "refresh failed: " + err.Error(),
		}, nil
	}

	email := ExtractEmail(token)
	remaining := time.Until(token.Expiry)

	return &CredentialStatus{
		Provider:  "gcp",
		Account:   email,
		ExpiresIn: remaining,
		Valid:     true,
		FilePath:  path,
	}, nil
}

// TokenSource returns a reusable token source for GCP API calls.
// It reads the ADC file and creates a token source that auto-refreshes.
// Falls back to google.DefaultTokenSource for service account environments.
func (g *GCPProvider) TokenSource(ctx context.Context, scopes ...string) (oauth2.TokenSource, error) {
	if len(scopes) == 0 {
		scopes = g.oauthConfig.Scopes
	}
	return google.DefaultTokenSource(ctx, scopes...)
}

// writeADC writes OAuth2 credentials to the standard ADC file path.
func (g *GCPProvider) writeADC(token *oauth2.Token) error {
	path, err := ADCPath()
	if err != nil {
		return err
	}

	// Ensure the directory exists.
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("failed to create directory %s: %w", dir, err)
	}

	creds := adcCredentials{
		Account:      "",
		ClientID:     g.oauthConfig.ClientID,
		ClientSecret: g.oauthConfig.ClientSecret,
		RefreshToken: token.RefreshToken,
		Type:         "authorized_user",
	}

	data, err := json.MarshalIndent(creds, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal credentials: %w", err)
	}

	// Write atomically: write to temp file then rename.
	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0o600); err != nil {
		return fmt.Errorf("failed to write credentials: %w", err)
	}
	// On Windows, os.Rename fails if the destination exists.
	if runtime.GOOS == "windows" {
		_ = os.Remove(path)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("failed to save credentials: %w", err)
	}

	return nil
}

// ReadADC reads and parses the ADC credentials file.
func ReadADC(path string) (*adcCredentials, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var creds adcCredentials
	if err := json.Unmarshal(data, &creds); err != nil {
		return nil, fmt.Errorf("invalid credentials file: %w", err)
	}

	if creds.RefreshToken == "" {
		return nil, fmt.Errorf("credentials file has no refresh token")
	}

	return &creds, nil
}

// RefreshFromADC uses the refresh token to get a fresh access token.
func RefreshFromADC(creds *adcCredentials) (*oauth2.Token, error) {
	cfg := &oauth2.Config{
		ClientID:     creds.ClientID,
		ClientSecret: creds.ClientSecret,
		Endpoint:     google.Endpoint,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	tokenSource := cfg.TokenSource(ctx, &oauth2.Token{
		RefreshToken: creds.RefreshToken,
	})

	return tokenSource.Token()
}

// ExtractEmail attempts to extract the email from the ID token in the
// OAuth2 token response. Falls back to "unknown" if unavailable.
func ExtractEmail(token *oauth2.Token) string {
	// The ID token is in token.Extra("id_token").
	rawIDToken, ok := token.Extra("id_token").(string)
	if !ok || rawIDToken == "" {
		return "unknown"
	}

	// Decode the JWT payload (second segment).
	parts := strings.Split(rawIDToken, ".")
	if len(parts) < 2 {
		return "unknown"
	}

	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return "unknown"
	}

	var claims struct {
		Email string `json:"email"`
	}
	if err := json.Unmarshal(payload, &claims); err != nil {
		return "unknown"
	}

	if claims.Email != "" {
		return claims.Email
	}
	return "unknown"
}

// FormatDuration returns a human-friendly duration string.
func FormatDuration(d time.Duration) string {
	if d < 0 {
		return "expired"
	}
	if d < time.Minute {
		return "< 1 min"
	}
	if d < time.Hour {
		return fmt.Sprintf("%d min", int(d.Minutes()))
	}
	return fmt.Sprintf("%dh %dm", int(d.Hours()), int(d.Minutes())%60)
}

// handleOAuthCallback returns an http.HandlerFunc for the OAuth2 callback.
func handleOAuthCallback(expectedState string, resultCh chan<- authResult) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("state") != expectedState {
			http.Error(w, "Invalid state parameter", http.StatusBadRequest)
			resultCh <- authResult{err: fmt.Errorf("state mismatch")}
			return
		}

		if errParam := r.URL.Query().Get("error"); errParam != "" {
			desc := r.URL.Query().Get("error_description")
			http.Error(w, "Authorization failed: "+desc, http.StatusBadRequest)
			resultCh <- authResult{err: fmt.Errorf("authorization error: %s — %s", errParam, desc)}
			return
		}

		code := r.URL.Query().Get("code")
		if code == "" {
			http.Error(w, "No authorization code received", http.StatusBadRequest)
			resultCh <- authResult{err: fmt.Errorf("no authorization code")}
			return
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprintf(w, `<!DOCTYPE html>
<html><body style="font-family:system-ui;text-align:center;padding:60px">
<h2>✅ Authenticated</h2>
<p>You can close this tab and return to your terminal.</p>
</body></html>`)
		resultCh <- authResult{code: code}
	}
}

// openBrowser opens a URL in the system's default browser.
func openBrowser(url string) error {
	switch runtime.GOOS {
	case "darwin":
		return exec.Command("open", url).Start()
	case "linux":
		return exec.Command("xdg-open", url).Start()
	case "windows":
		return exec.Command("cmd", "/c", "start", "", url).Start()
	default:
		return fmt.Errorf("unsupported platform %s", runtime.GOOS)
	}
}
