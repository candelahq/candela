package cloudauth

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/oauth2"
)

func TestGCPProvider_Name(t *testing.T) {
	p := NewGCPProvider()
	if got := p.Name(); got != "gcp" {
		t.Errorf("Name() = %q, want %q", got, "gcp")
	}
}

func TestGCPProvider_IsConfigured_NoFile(t *testing.T) {
	// Point to a nonexistent path via CLOUDSDK_CONFIG.
	t.Setenv("CLOUDSDK_CONFIG", filepath.Join(t.TempDir(), "nonexistent"))
	p := NewGCPProvider()
	if p.IsConfigured() {
		t.Error("IsConfigured() = true for nonexistent ADC path")
	}
}

func TestGCPProvider_IsConfigured_WithFile(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("CLOUDSDK_CONFIG", tmpDir)

	// Write a dummy ADC file.
	path := filepath.Join(tmpDir, "application_default_credentials.json")
	if err := os.WriteFile(path, []byte(`{}`), 0o600); err != nil {
		t.Fatal(err)
	}

	p := NewGCPProvider()
	if !p.IsConfigured() {
		t.Error("IsConfigured() = false when ADC file exists")
	}
}

func TestADCPath_CloudSDKConfig(t *testing.T) {
	t.Setenv("CLOUDSDK_CONFIG", "/custom/gcloud")
	got, err := ADCPath()
	if err != nil {
		t.Fatalf("ADCPath: %v", err)
	}
	want := filepath.Join("/custom/gcloud", "application_default_credentials.json")
	if got != want {
		t.Errorf("ADCPath() = %q, want %q", got, want)
	}
}

func TestADCPath_Default(t *testing.T) {
	t.Setenv("CLOUDSDK_CONFIG", "")
	path, err := ADCPath()
	if err != nil {
		t.Fatalf("ADCPath: %v", err)
	}
	if !filepath.IsAbs(path) {
		t.Errorf("ADCPath returned relative path: %s", path)
	}
	if filepath.Base(path) != "application_default_credentials.json" {
		t.Errorf("ADCPath missing expected filename: %s", path)
	}
}

func TestReadADC_ValidFile(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "adc.json")

	creds := adcCredentials{
		ClientID:     "test-client-id",
		ClientSecret: "test-secret",
		RefreshToken: "test-refresh-token",
		Type:         "authorized_user",
	}
	data, _ := json.Marshal(creds)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := ReadADC(path)
	if err != nil {
		t.Fatalf("ReadADC: %v", err)
	}
	if got.RefreshToken != "test-refresh-token" {
		t.Errorf("RefreshToken = %q, want %q", got.RefreshToken, "test-refresh-token")
	}
	if got.Type != "authorized_user" {
		t.Errorf("Type = %q, want %q", got.Type, "authorized_user")
	}
}

func TestReadADC_MissingFile(t *testing.T) {
	_, err := ReadADC("/nonexistent/path/to/adc.json")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestReadADC_InvalidJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bad.json")
	if err := os.WriteFile(path, []byte("not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := ReadADC(path)
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestReadADC_NoRefreshToken(t *testing.T) {
	path := filepath.Join(t.TempDir(), "no_refresh.json")
	data := `{"type":"authorized_user","client_id":"id","client_secret":"secret"}`
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := ReadADC(path)
	if err == nil {
		t.Fatal("expected error for missing refresh token")
	}
}

func TestExtractEmail_NoIDToken(t *testing.T) {
	token := &oauth2.Token{AccessToken: "test"}
	if got := ExtractEmail(token); got != "unknown" {
		t.Errorf("ExtractEmail(no id_token) = %q, want %q", got, "unknown")
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
		if got := FormatDuration(tt.d); got != tt.want {
			t.Errorf("FormatDuration(%v) = %q, want %q", tt.d, got, tt.want)
		}
	}
}

func TestRefreshFromADC_InvalidCreds(t *testing.T) {
	creds := &adcCredentials{
		ClientID:     "fake-client-id",
		ClientSecret: "fake-secret",
		RefreshToken: "fake-refresh-token",
	}
	_, err := RefreshFromADC(creds)
	if err == nil {
		t.Fatal("expected error for invalid credentials")
	}
}

func TestGCPProvider_Status_NoCreds(t *testing.T) {
	t.Setenv("CLOUDSDK_CONFIG", filepath.Join(t.TempDir(), "nonexistent"))
	p := NewGCPProvider()

	status, err := p.Status(t.Context())
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if status.Valid {
		t.Error("Status.Valid = true for missing credentials")
	}
	if status.Provider != "gcp" {
		t.Errorf("Status.Provider = %q, want %q", status.Provider, "gcp")
	}
}
