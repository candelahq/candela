package cloudauth

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAWSProvider_Name(t *testing.T) {
	p := NewAWSProvider("us-east-1", "")
	if got := p.Name(); got != "aws" {
		t.Errorf("Name() = %q, want %q", got, "aws")
	}
}

func TestAWSProvider_Region(t *testing.T) {
	p := NewAWSProvider("eu-west-1", "")
	if got := p.Region(); got != "eu-west-1" {
		t.Errorf("Region() = %q, want %q", got, "eu-west-1")
	}
}

func TestAWSProvider_DefaultRegion(t *testing.T) {
	p := NewAWSProvider("", "")
	if got := p.Region(); got != "us-east-1" {
		t.Errorf("Region() = %q, want default %q", got, "us-east-1")
	}
}

func TestAWSProvider_IsConfigured_NoCredentials(t *testing.T) {
	// Point to a nonexistent AWS config directory.
	tmpDir := t.TempDir()
	t.Setenv("AWS_CONFIG_FILE", filepath.Join(tmpDir, "config"))
	t.Setenv("AWS_SHARED_CREDENTIALS_FILE", filepath.Join(tmpDir, "credentials"))
	// Clear any existing env var credentials.
	t.Setenv("AWS_ACCESS_KEY_ID", "")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "")

	p := NewAWSProvider("us-east-1", "nonexistent-profile")
	// This should not panic even when no credentials exist.
	_ = p.IsConfigured()
}

func TestAWSProvider_CredentialsPath(t *testing.T) {
	t.Setenv("AWS_SHARED_CREDENTIALS_FILE", "/custom/path/credentials")
	p := NewAWSProvider("us-east-1", "")
	if got := p.credentialsPath(); got != "/custom/path/credentials" {
		t.Errorf("credentialsPath() = %q, want %q", got, "/custom/path/credentials")
	}
}

func TestAWSProvider_CredentialsPath_Default(t *testing.T) {
	t.Setenv("AWS_SHARED_CREDENTIALS_FILE", "")
	t.Setenv("AWS_CONFIG_FILE", "")
	p := NewAWSProvider("us-east-1", "")
	got := p.credentialsPath()
	if !filepath.IsAbs(got) {
		t.Errorf("credentialsPath() returned relative path: %s", got)
	}
	if filepath.Base(got) != "credentials" {
		t.Errorf("credentialsPath() missing expected filename: %s", got)
	}
}

func TestAWSProvider_TokenSource_ReturnsError(t *testing.T) {
	p := NewAWSProvider("us-east-1", "")
	_, err := p.TokenSource(t.Context())
	if err == nil {
		t.Error("expected error from TokenSource() — AWS uses SigV4, not Bearer tokens")
	}
}

func TestAWSProvider_HasSSOProfile_NoConfig(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("AWS_CONFIG_FILE", filepath.Join(tmpDir, "config"))
	p := NewAWSProvider("us-east-1", "")
	if p.hasSSOProfile() {
		t.Error("hasSSOProfile() = true for nonexistent config")
	}
}

func TestAWSProvider_HasSSOProfile_WithSSO_DefaultProfile(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config")
	content := `[default]
sso_start_url = https://my-sso.awsapps.com/start
sso_region = us-east-1
sso_account_id = 123456789
sso_role_name = DevAccess
region = us-east-1
`
	if err := os.WriteFile(configPath, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	t.Setenv("AWS_CONFIG_FILE", configPath)
	p := NewAWSProvider("us-east-1", "") // empty profile = default
	if !p.hasSSOProfile() {
		t.Error("hasSSOProfile() = false for SSO-configured default profile")
	}
}

func TestAWSProvider_HasSSOProfile_WithSSO_NamedProfile(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config")
	content := `[default]
region = us-east-1

[profile dev-sso]
sso_start_url = https://my-sso.awsapps.com/start
sso_region = us-east-1
sso_account_id = 123456789
sso_role_name = DevAccess
region = us-east-1
`
	if err := os.WriteFile(configPath, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	t.Setenv("AWS_CONFIG_FILE", configPath)
	p := NewAWSProvider("us-east-1", "dev-sso")
	if !p.hasSSOProfile() {
		t.Error("hasSSOProfile() = false for SSO-configured named profile")
	}
}

func TestAWSProvider_HasSSOProfile_NonSSOProfile(t *testing.T) {
	// Regression: SSO detection must NOT match a different profile's sso_start_url.
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config")
	content := `[default]
region = us-east-1
aws_access_key_id = AKIAIOSFODNN7EXAMPLE
aws_secret_access_key = wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY

[profile other-sso]
sso_start_url = https://my-sso.awsapps.com/start
sso_region = us-east-1
`
	if err := os.WriteFile(configPath, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	t.Setenv("AWS_CONFIG_FILE", configPath)
	p := NewAWSProvider("us-east-1", "") // default profile
	if p.hasSSOProfile() {
		t.Error("hasSSOProfile() = true for non-SSO default profile (SSO is on a different profile)")
	}
}

func TestAWSProvider_HasSSOProfile_NamedNonSSO(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config")
	content := `[profile keys-profile]
region = us-west-2

[profile sso-profile]
sso_start_url = https://my-sso.awsapps.com/start
`
	if err := os.WriteFile(configPath, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	t.Setenv("AWS_CONFIG_FILE", configPath)
	p := NewAWSProvider("us-east-1", "keys-profile")
	if p.hasSSOProfile() {
		t.Error("hasSSOProfile() = true for non-SSO named profile")
	}
}

func TestAWSProvider_Status_NoCreds(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("AWS_CONFIG_FILE", filepath.Join(tmpDir, "config"))
	t.Setenv("AWS_SHARED_CREDENTIALS_FILE", filepath.Join(tmpDir, "credentials"))
	t.Setenv("AWS_ACCESS_KEY_ID", "")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "")

	p := NewAWSProvider("us-east-1", "nonexistent")
	status, err := p.Status(t.Context())
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if status.Valid {
		t.Error("Status.Valid = true for missing credentials")
	}
	if status.Provider != "aws" {
		t.Errorf("Status.Provider = %q, want %q", status.Provider, "aws")
	}
}

func TestRegistry_Get_AWS(t *testing.T) {
	p, err := Get("aws")
	if err != nil {
		t.Fatalf("Get(aws): %v", err)
	}
	if p.Name() != "aws" {
		t.Errorf("Name() = %q, want %q", p.Name(), "aws")
	}
}
