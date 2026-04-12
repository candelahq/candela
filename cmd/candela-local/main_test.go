package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfig_ValidFile(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "candela.yaml")
	err := os.WriteFile(cfgPath, []byte(`
remote: https://candela-xxx.run.app
audience: "123456.apps.googleusercontent.com"
port: 9090
`), 0o644)
	if err != nil {
		t.Fatal(err)
	}

	cfg := loadConfig(cfgPath)

	if cfg.Remote != "https://candela-xxx.run.app" {
		t.Errorf("Remote = %q, want %q", cfg.Remote, "https://candela-xxx.run.app")
	}
	if cfg.Audience != "123456.apps.googleusercontent.com" {
		t.Errorf("Audience = %q, want %q", cfg.Audience, "123456.apps.googleusercontent.com")
	}
	if cfg.Port != 9090 {
		t.Errorf("Port = %d, want 9090", cfg.Port)
	}
}

func TestLoadConfig_IndentedYAML(t *testing.T) {
	// Terraform output produces indented YAML — verify we handle that.
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "candela.yaml")
	err := os.WriteFile(cfgPath, []byte(`
    # ~/.candela.yaml
    remote: https://candela-abc.run.app
    audience: 7890.apps.googleusercontent.com
    port: 8181
`), 0o644)
	if err != nil {
		t.Fatal(err)
	}

	cfg := loadConfig(cfgPath)

	if cfg.Remote != "https://candela-abc.run.app" {
		t.Errorf("Remote = %q, want %q", cfg.Remote, "https://candela-abc.run.app")
	}
	if cfg.Audience != "7890.apps.googleusercontent.com" {
		t.Errorf("Audience = %q, want %q", cfg.Audience, "7890.apps.googleusercontent.com")
	}
	if cfg.Port != 8181 {
		t.Errorf("Port = %d, want 8181", cfg.Port)
	}
}

func TestLoadConfig_MissingFile(t *testing.T) {
	cfg := loadConfig("/nonexistent/path/candela.yaml")

	// Should return empty config, not panic.
	if cfg.Remote != "" {
		t.Errorf("Remote = %q, want empty for missing file", cfg.Remote)
	}
	if cfg.Audience != "" {
		t.Errorf("Audience = %q, want empty for missing file", cfg.Audience)
	}
	if cfg.Port != 0 {
		t.Errorf("Port = %d, want 0 for missing file", cfg.Port)
	}
}

func TestLoadConfig_EmptyPath(t *testing.T) {
	// Unset env var to test default path fallback.
	t.Setenv("CANDELA_CONFIG", "")
	cfg := loadConfig("")

	// Should not panic; returns empty or default config.
	if cfg == nil {
		t.Fatal("expected non-nil config")
	}
}

func TestLoadConfig_InvalidYAML(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "candela.yaml")
	err := os.WriteFile(cfgPath, []byte(`{{{not valid yaml`), 0o644)
	if err != nil {
		t.Fatal(err)
	}

	cfg := loadConfig(cfgPath)

	// Should return empty config, not error.
	if cfg.Remote != "" {
		t.Errorf("Remote = %q, want empty for invalid YAML", cfg.Remote)
	}
}

func TestLoadConfig_EnvVar(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "custom.yaml")
	err := os.WriteFile(cfgPath, []byte(`
remote: https://env-test.run.app
audience: env-audience
port: 7777
`), 0o644)
	if err != nil {
		t.Fatal(err)
	}

	t.Setenv("CANDELA_CONFIG", cfgPath)
	cfg := loadConfig("") // Empty path should fall back to env var.

	if cfg.Remote != "https://env-test.run.app" {
		t.Errorf("Remote = %q, want %q", cfg.Remote, "https://env-test.run.app")
	}
}

func TestLoadConfig_PartialConfig(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "candela.yaml")
	err := os.WriteFile(cfgPath, []byte(`
remote: https://partial.run.app
`), 0o644)
	if err != nil {
		t.Fatal(err)
	}

	cfg := loadConfig(cfgPath)

	if cfg.Remote != "https://partial.run.app" {
		t.Errorf("Remote = %q, want %q", cfg.Remote, "https://partial.run.app")
	}
	if cfg.Audience != "" {
		t.Errorf("Audience = %q, want empty", cfg.Audience)
	}
	if cfg.Port != 0 {
		t.Errorf("Port = %d, want 0", cfg.Port)
	}
}
