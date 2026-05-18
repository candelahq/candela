package main

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

// adcCredentials represents the Application Default Credentials file format.
// This is the same structure that `gcloud auth application-default login` writes.
type adcCredentials struct {
	Account      string `json:"account"`
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
	QuotaProject string `json:"quota_project_id,omitempty"`
	RefreshToken string `json:"refresh_token"`
	Type         string `json:"type"`
}

// Google's well-known OAuth client for CLI tools. This is the same client_id
// used by `gcloud auth application-default login` — it's public and intended
// for native/desktop applications.
var oauthConfig = &oauth2.Config{
	ClientID:     "764086051850-6qr4p6gpi6hn506pt8ejuq83di341hur.apps.googleusercontent.com",
	ClientSecret: "d-FL95Q19q7MQmFpd7hHD0Ty",
	Scopes: []string{
		"openid",
		"https://www.googleapis.com/auth/userinfo.email",
		"https://www.googleapis.com/auth/cloud-platform",
	},
	Endpoint: google.Endpoint,
}

// adcPath returns the standard Application Default Credentials file path.
// Supports CLOUDSDK_CONFIG override, Windows (APPDATA), and Unix (~/.config).
func adcPath() (string, error) {
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

// handleAuth dispatches auth subcommands.
func handleAuth(args []string) {
	sub := ""
	if len(args) > 0 {
		sub = args[0]
	}

	switch sub {
	case "login":
		cmdAuthLogin()
	case "status":
		cmdAuthStatus()
	case "token":
		cmdAuthToken()
	default:
		_, _ = fmt.Fprintf(os.Stderr, `candela auth — manage Google Cloud credentials

Usage:
  candela auth login      Login via browser (writes ADC credentials)
  candela auth status     Show current credential status
  candela auth token      Print a fresh access token
`)
		if sub != "" {
			os.Exit(1)
		}
	}
}

// cmdAuthLogin performs the OAuth2 authorization code flow via browser.
func cmdAuthLogin() {
	// Start a temporary HTTP server on a random port.
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "error: failed to start local server: %v\n", err)
		os.Exit(1)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	redirectURL := fmt.Sprintf("http://127.0.0.1:%d/callback", port)
	oauthConfig.RedirectURL = redirectURL

	// Generate a random state parameter for CSRF protection.
	stateBytes := make([]byte, 16)
	if _, err := rand.Read(stateBytes); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "error: failed to generate random state: %v\n", err)
		os.Exit(1)
	}
	state := base64.URLEncoding.EncodeToString(stateBytes)

	// Channel to receive the auth result.
	type authResult struct {
		code string
		err  error
	}
	resultCh := make(chan authResult, 1)

	// Set up the callback handler.
	mux := http.NewServeMux()
	mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("state") != state {
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

		w.Header().Set("Content-Type", "text/html")
		_, _ = fmt.Fprintf(w, `<!DOCTYPE html>
<html><body style="font-family:system-ui;text-align:center;padding:60px">
<h2>✅ Authenticated</h2>
<p>You can close this tab and return to your terminal.</p>
</body></html>`)
		resultCh <- authResult{code: code}
	})

	server := &http.Server{Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	go func() { _ = server.Serve(listener) }()
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = server.Shutdown(ctx)
	}()

	// Build authorization URL and open browser.
	authURL := oauthConfig.AuthCodeURL(state, oauth2.AccessTypeOffline)

	fmt.Println("Opening browser to authenticate...")
	fmt.Printf("If the browser doesn't open, visit:\n  %s\n\n", authURL)

	if err := openBrowser(authURL); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "warning: could not open browser: %v\n", err)
	}

	// Wait for the callback (with timeout).
	fmt.Println("Waiting for authentication...")
	select {
	case result := <-resultCh:
		if result.err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "error: %v\n", result.err)
			os.Exit(1)
		}

		// Exchange the authorization code for tokens.
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		token, err := oauthConfig.Exchange(ctx, result.code)
		if err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "error: failed to exchange authorization code: %v\n", err)
			os.Exit(1)
		}

		// Write credentials to the standard ADC path.
		if err := writeADC(token); err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "error: failed to write credentials: %v\n", err)
			os.Exit(1)
		}

		// Display the authenticated identity.
		email := extractEmail(token)
		path, _ := adcPath()
		fmt.Printf("\n🕯️  Authenticated as: %s\n", email)
		fmt.Printf("   Credentials saved to: %s\n", path)

	case <-time.After(5 * time.Minute):
		_, _ = fmt.Fprintf(os.Stderr, "error: authentication timed out (5 minutes)\n")
		os.Exit(1)
	}
}

// cmdAuthStatus reads the ADC file and displays credential status.
func cmdAuthStatus() {
	path, err := adcPath()
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	creds, err := readADC(path)
	if err != nil {
		fmt.Println("● credentials: not configured")
		fmt.Printf("  Run: candela auth login\n")
		return
	}

	fmt.Printf("● credentials: %s\n", path)
	fmt.Printf("  type:     %s\n", creds.Type)
	if creds.QuotaProject != "" {
		fmt.Printf("  project:  %s\n", creds.QuotaProject)
	}

	// Try to refresh and get identity info.
	token, err := refreshFromADC(creds)
	if err != nil {
		fmt.Printf("  status:   ❌ refresh failed — %v\n", err)
		fmt.Printf("  Run: candela auth login\n")
		return
	}

	email := extractEmail(token)
	remaining := time.Until(token.Expiry)

	fmt.Printf("  account:  %s\n", email)
	fmt.Printf("  status:   ✅ valid (expires in %s)\n", formatDuration(remaining))
}

// cmdAuthToken refreshes and prints a fresh access token (for piping).
func cmdAuthToken() {
	path, err := adcPath()
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	creds, err := readADC(path)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "error: no credentials found — run: candela auth login\n")
		os.Exit(1)
	}

	token, err := refreshFromADC(creds)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "error: failed to refresh token: %v\n", err)
		os.Exit(1)
	}

	// Print just the token (suitable for piping to curl, etc.).
	fmt.Print(token.AccessToken)
}

// writeADC writes OAuth2 credentials to the standard ADC file path.
func writeADC(token *oauth2.Token) error {
	path, err := adcPath()
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
		ClientID:     oauthConfig.ClientID,
		ClientSecret: oauthConfig.ClientSecret,
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

// readADC reads and parses the ADC credentials file.
func readADC(path string) (*adcCredentials, error) {
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

// refreshFromADC uses the refresh token to get a fresh access token.
func refreshFromADC(creds *adcCredentials) (*oauth2.Token, error) {
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

// extractEmail attempts to extract the email from the ID token in the
// OAuth2 token response. Falls back to "unknown" if unavailable.
func extractEmail(token *oauth2.Token) string {
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

// formatDuration returns a human-friendly duration string.
func formatDuration(d time.Duration) string {
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
