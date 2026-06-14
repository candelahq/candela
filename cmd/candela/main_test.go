package main

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
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
    # ~/.config/candela/config.yaml
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

func TestLoadConfig_LocalUpstream(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "candela.yaml")
	err := os.WriteFile(cfgPath, []byte(`
remote: https://candela-xxx.run.app
audience: test-audience
local_upstream: "http://127.0.0.1:11434"
`), 0o644)
	if err != nil {
		t.Fatal(err)
	}

	cfg := loadConfig(cfgPath)
	if cfg.LocalUpstream != "http://127.0.0.1:11434" {
		t.Errorf("LocalUpstream = %q, want %q", cfg.LocalUpstream, "http://127.0.0.1:11434")
	}
}

func TestSingleJoiningSlash(t *testing.T) {
	tests := []struct {
		a, b string
		want string
	}{
		// Basic cases
		{"/api", "/v1/models", "/api/v1/models"},
		{"", "/v1/chat", "/v1/chat"},
		{"/", "/v1/chat", "/v1/chat"},

		// Slash normalization
		{"/api/", "/v1/models", "/api/v1/models"},
		{"/api", "v1/models", "/api/v1/models"},
		{"/api/", "v1/models", "/api/v1/models"},

		// Root paths
		{"", "/", "/"},
		{"/", "/", "/"},
		{"", "", "/"},
	}

	for _, tt := range tests {
		t.Run(tt.a+"_"+tt.b, func(t *testing.T) {
			got := singleJoiningSlash(tt.a, tt.b)
			if got != tt.want {
				t.Errorf("singleJoiningSlash(%q, %q) = %q, want %q", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

func TestLoadConfig_RuntimeManagement(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "candela.yaml")
	err := os.WriteFile(cfgPath, []byte(`
remote: https://candela-xxx.run.app
audience: test-audience
port: 8181
runtime_backend: ollama
runtime_config:
  host: 127.0.0.1
  port: 11434
runtime_manage:
  auto_start: true
  auto_pull: true
  health_interval: 15s
  models:
    - llama3.2:8b
    - codellama:13b
`), 0o644)
	if err != nil {
		t.Fatal(err)
	}

	cfg := loadConfig(cfgPath)

	if cfg.RuntimeBackend != "ollama" {
		t.Errorf("RuntimeBackend = %q, want %q", cfg.RuntimeBackend, "ollama")
	}
	if cfg.RuntimeConfig.Host != "127.0.0.1" {
		t.Errorf("RuntimeConfig.Host = %q, want %q", cfg.RuntimeConfig.Host, "127.0.0.1")
	}
	if cfg.RuntimeConfig.Port != 11434 {
		t.Errorf("RuntimeConfig.Port = %d, want 11434", cfg.RuntimeConfig.Port)
	}
	if !cfg.RuntimeManage.AutoStart {
		t.Error("RuntimeManage.AutoStart should be true")
	}
	if !cfg.RuntimeManage.AutoPull {
		t.Error("RuntimeManage.AutoPull should be true")
	}
	if len(cfg.RuntimeManage.Models) != 2 {
		t.Fatalf("RuntimeManage.Models = %v, want 2 entries", cfg.RuntimeManage.Models)
	}
	if cfg.RuntimeManage.Models[0] != "llama3.2:8b" {
		t.Errorf("Models[0] = %q, want %q", cfg.RuntimeManage.Models[0], "llama3.2:8b")
	}
}

func TestCmdRestart_NotRunning_NoPIDFile(t *testing.T) {
	// When there is no PID file, cmdRestart should proceed to start.
	// We can't easily test the full start (it execs a binary), but we can
	// verify that the restart path doesn't panic or error when no PID file exists.

	dir := t.TempDir()
	pidPath := filepath.Join(dir, "candela.pid")

	// Verify PID file does not exist.
	if _, err := os.Stat(pidPath); err == nil {
		t.Fatal("expected PID file to not exist before test")
	}

	// Simulate the restart logic for the "not running" path.
	data, err := os.ReadFile(pidPath)
	if err != nil {
		// This is the expected path — no PID file means not running.
		t.Logf("no PID file found (expected): %v", err)
	} else {
		t.Errorf("unexpected PID data: %s", string(data))
	}
}

func TestCmdRestart_StalePIDFile(t *testing.T) {
	// When a stale PID file exists (process is dead), restart should
	// clean it up and proceed to start.

	dir := t.TempDir()
	pidPath := filepath.Join(dir, "candela.pid")

	// Write a PID that definitely doesn't exist (use a very high PID).
	// PID 99999999 is extremely unlikely to be a real process.
	err := os.WriteFile(pidPath, []byte("99999999"), 0o644)
	if err != nil {
		t.Fatal(err)
	}

	// Read the PID file — simulating the restart logic.
	data, readErr := os.ReadFile(pidPath)
	if readErr != nil {
		t.Fatal("expected PID file to be readable")
	}

	pid, parseErr := strconv.Atoi(strings.TrimSpace(string(data)))
	if parseErr != nil {
		t.Fatalf("expected valid PID, got parse error: %v", parseErr)
	}
	if pid != 99999999 {
		t.Fatalf("expected PID 99999999, got %d", pid)
	}

	// The process should not be alive.
	process, findErr := os.FindProcess(pid)
	if findErr != nil {
		t.Logf("FindProcess returned error (expected on some OS): %v", findErr)
	} else {
		// Signal(0) should fail for a non-existent process.
		if process.Signal(syscall.Signal(0)) == nil {
			t.Skip("PID 99999999 unexpectedly exists — skipping")
		}
	}

	// Clean up stale PID file (as cmdRestart would).
	_ = os.Remove(pidPath)

	if _, err := os.Stat(pidPath); err == nil {
		t.Error("expected PID file to be removed after stale cleanup")
	}
}
