package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
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

// ── adcPath tests ──────────────────────────────────────────────────────────

func TestAdcPath_CLOUDSDK_CONFIG(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("CLOUDSDK_CONFIG", tmpDir)
	path, err := adcPath()
	if err != nil {
		t.Fatalf("adcPath: %v", err)
	}
	want := filepath.Join(tmpDir, "application_default_credentials.json")
	if path != want {
		t.Errorf("adcPath = %q, want %q", path, want)
	}
}

func TestAdcPath_CLOUDSDK_CONFIG_TakesPriority(t *testing.T) {
	// Even with HOME set, CLOUDSDK_CONFIG should win.
	tmpDir := t.TempDir()
	t.Setenv("CLOUDSDK_CONFIG", tmpDir)
	path, err := adcPath()
	if err != nil {
		t.Fatalf("adcPath: %v", err)
	}
	if !strings.HasPrefix(path, tmpDir) {
		t.Errorf("expected path to start with CLOUDSDK_CONFIG dir %s, got %s", tmpDir, path)
	}
}

// ── writeADC tests ──────────────────────────────────────────────────────────

func TestWriteADC_CreatesDirectoryAndFile(t *testing.T) {
	// Point CLOUDSDK_CONFIG to a nested non-existent directory.
	tmpDir := t.TempDir()
	nestedDir := filepath.Join(tmpDir, "deep", "nested", "gcloud-config")
	t.Setenv("CLOUDSDK_CONFIG", nestedDir)

	token := &oauth2.Token{
		RefreshToken: "test-refresh-token-write",
	}

	if err := writeADC(token); err != nil {
		t.Fatalf("writeADC: %v", err)
	}

	// Verify the file was created.
	path := filepath.Join(nestedDir, "application_default_credentials.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read written file: %v", err)
	}

	var creds adcCredentials
	if err := json.Unmarshal(data, &creds); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if creds.RefreshToken != "test-refresh-token-write" {
		t.Errorf("refresh_token = %q, want %q", creds.RefreshToken, "test-refresh-token-write")
	}
	if creds.Type != "authorized_user" {
		t.Errorf("type = %q, want %q", creds.Type, "authorized_user")
	}
	if creds.ClientID != oauthConfig.ClientID {
		t.Errorf("client_id = %q, want %q", creds.ClientID, oauthConfig.ClientID)
	}
}

func TestWriteADC_AtomicOverwrite(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("CLOUDSDK_CONFIG", tmpDir)

	// Write first version.
	token1 := &oauth2.Token{RefreshToken: "token-v1"}
	if err := writeADC(token1); err != nil {
		t.Fatalf("writeADC v1: %v", err)
	}

	// Overwrite with second version.
	token2 := &oauth2.Token{RefreshToken: "token-v2"}
	if err := writeADC(token2); err != nil {
		t.Fatalf("writeADC v2: %v", err)
	}

	// Read back and verify it's v2.
	path := filepath.Join(tmpDir, "application_default_credentials.json")
	creds, err := readADC(path)
	if err != nil {
		t.Fatalf("readADC: %v", err)
	}
	if creds.RefreshToken != "token-v2" {
		t.Errorf("refresh_token = %q, want %q", creds.RefreshToken, "token-v2")
	}

	// Verify no temp file left behind.
	tmpPath := path + ".tmp"
	if _, err := os.Stat(tmpPath); !os.IsNotExist(err) {
		t.Errorf("temp file %s should not exist after successful write", tmpPath)
	}
}

func TestWriteADC_FilePermissions(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("CLOUDSDK_CONFIG", tmpDir)

	if err := writeADC(&oauth2.Token{RefreshToken: "secret"}); err != nil {
		t.Fatalf("writeADC: %v", err)
	}

	path := filepath.Join(tmpDir, "application_default_credentials.json")
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	// File should be owner-only readable/writable (0600).
	// Windows does not support POSIX permission bits in the same way.
	if runtime.GOOS != "windows" {
		perm := info.Mode().Perm()
		if perm&0o077 != 0 {
			t.Errorf("file permissions = %o, want no group/other access (0600)", perm)
		}
	}
}

// ── OAuth callback handler tests (uses production newCallbackHandler) ─────

// callbackTestServer creates an httptest.Server using the real newCallbackHandler
// and returns the server along with the result channel.
func callbackTestServer(state string) (*httptest.Server, chan authResult) {
	resultCh := make(chan authResult, 1)
	mux := http.NewServeMux()
	mux.HandleFunc("/callback", newCallbackHandler(state, resultCh))
	return httptest.NewServer(mux), resultCh
}

func TestCallbackHandler_StateMismatch(t *testing.T) {
	server, resultCh := callbackTestServer("correct-state-value")
	defer server.Close()

	resp, err := http.Get(server.URL + "/callback?state=wrong-state&code=test-code")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusBadRequest)
	}
	result := <-resultCh
	if result.err == nil {
		t.Fatal("expected error for state mismatch")
	}
	if !strings.Contains(result.err.Error(), "state mismatch") {
		t.Errorf("error = %v, want state mismatch", result.err)
	}
}

func TestCallbackHandler_ErrorFromProvider(t *testing.T) {
	server, resultCh := callbackTestServer("test-state")
	defer server.Close()

	resp, err := http.Get(server.URL + "/callback?state=test-state&error=access_denied&error_description=User+denied+access")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusBadRequest)
	}
	result := <-resultCh
	if result.err == nil {
		t.Fatal("expected error for provider error")
	}
	if !strings.Contains(result.err.Error(), "access_denied") {
		t.Errorf("error = %v, want access_denied", result.err)
	}
}

func TestCallbackHandler_MissingCode(t *testing.T) {
	server, resultCh := callbackTestServer("test-state")
	defer server.Close()

	resp, err := http.Get(server.URL + "/callback?state=test-state")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusBadRequest)
	}
	result := <-resultCh
	if result.err == nil {
		t.Fatal("expected error for missing code")
	}
	if !strings.Contains(result.err.Error(), "no authorization code") {
		t.Errorf("error = %v, want no authorization code", result.err)
	}
}

func TestCallbackHandler_SuccessfulCode(t *testing.T) {
	server, resultCh := callbackTestServer("test-state")
	defer server.Close()

	resp, err := http.Get(server.URL + "/callback?state=test-state&code=4/auth-code-xyz")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	result := <-resultCh
	if result.err != nil {
		t.Fatalf("unexpected error: %v", result.err)
	}
	if result.code != "4/auth-code-xyz" {
		t.Errorf("code = %q, want %q", result.code, "4/auth-code-xyz")
	}
}

// ── readADC edge cases ──────────────────────────────────────────────────────

func TestReadADC_ExtraFields(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "adc.json")
	// ADC file with extra fields (e.g., quota_project_id, account) should
	// still parse correctly.
	data := `{
		"type": "authorized_user",
		"client_id": "test-id",
		"client_secret": "test-secret",
		"refresh_token": "test-refresh",
		"quota_project_id": "my-project",
		"account": "user@example.com",
		"universe_domain": "googleapis.com"
	}`
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}

	creds, err := readADC(path)
	if err != nil {
		t.Fatalf("readADC: %v", err)
	}
	if creds.RefreshToken != "test-refresh" {
		t.Errorf("refresh_token = %q", creds.RefreshToken)
	}
	if creds.QuotaProject != "my-project" {
		t.Errorf("quota_project = %q", creds.QuotaProject)
	}
}

func TestReadADC_EmptyFile(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "empty.json")
	if err := os.WriteFile(path, []byte(""), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := readADC(path)
	if err == nil {
		t.Fatal("expected error for empty file")
	}
}

// ── handleAuth dispatch tests ───────────────────────────────────────────────

func TestHandleAuth_NoSubcommand(t *testing.T) {
	// Redirect stderr to capture output.
	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	// handleAuth with unknown subcommand prints help to stderr.
	// (It would normally call os.Exit(1), but we test the print path.)
	// We can't easily test os.Exit, so we just verify it doesn't panic.
	done := make(chan string)
	go func() {
		buf := make([]byte, 1024)
		n, _ := r.Read(buf)
		done <- string(buf[:n])
	}()

	// This will write to stderr and call os.Exit(1).
	// Since we can't intercept os.Exit in a unit test, just verify
	// that known subcommands are dispatched correctly by the switch.
	handleAuth([]string{}) // empty string → help path (no exit)
	_ = w.Close()
	os.Stderr = oldStderr

	output := <-done
	if !strings.Contains(output, "candela auth") {
		t.Errorf("expected help output, got: %s", output)
	}
}

// ── adcCredentials serialization ──────────────────────────────────────────

func TestAdcCredentials_JSONRoundTrip(t *testing.T) {
	creds := adcCredentials{
		Account:      "user@example.com",
		ClientID:     "test-client-id",
		ClientSecret: "test-client-secret",
		QuotaProject: "my-quota-project",
		RefreshToken: "my-refresh-token",
		Type:         "authorized_user",
	}

	data, err := json.Marshal(creds)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var got adcCredentials
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if got != creds {
		t.Errorf("round-trip mismatch: got %+v, want %+v", got, creds)
	}
}

func TestAdcCredentials_QuotaProjectOmitEmpty(t *testing.T) {
	creds := adcCredentials{
		ClientID:     "id",
		ClientSecret: "secret",
		RefreshToken: "token",
		Type:         "authorized_user",
	}

	data, err := json.Marshal(creds)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	if strings.Contains(string(data), "quota_project_id") {
		t.Error("expected quota_project_id to be omitted when empty")
	}
}
