package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"
	"testing"
	"time"
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

func TestStopProcess_NoPIDFile(t *testing.T) {
	dir := t.TempDir()
	pidPath := filepath.Join(dir, "candela.pid")

	// No PID file → stopProcess should return nil (nothing to stop).
	if err := stopProcess(pidPath); err != nil {
		t.Fatalf("stopProcess returned error for missing PID file: %v", err)
	}
}

func TestStopProcess_InvalidPIDFile(t *testing.T) {
	dir := t.TempDir()
	pidPath := filepath.Join(dir, "candela.pid")

	if err := os.WriteFile(pidPath, []byte("not-a-number"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := stopProcess(pidPath); err != nil {
		t.Fatalf("stopProcess returned error for invalid PID file: %v", err)
	}

	// PID file should be cleaned up.
	if _, err := os.Stat(pidPath); err == nil {
		t.Error("expected invalid PID file to be removed")
	}
}

func TestStopProcess_StalePID(t *testing.T) {
	dir := t.TempDir()
	pidPath := filepath.Join(dir, "candela.pid")

	// PID 99999999 is extremely unlikely to be a real process.
	if err := os.WriteFile(pidPath, []byte("99999999"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Verify the PID is indeed not running before proceeding.
	proc, err := os.FindProcess(99999999)
	if err == nil && proc.Signal(syscall.Signal(0)) == nil {
		t.Skip("PID 99999999 unexpectedly exists — skipping")
	}

	if err := stopProcess(pidPath); err != nil {
		t.Fatalf("stopProcess returned error for stale PID: %v", err)
	}

	// PID file should be cleaned up.
	if _, err := os.Stat(pidPath); err == nil {
		t.Error("expected stale PID file to be removed")
	}
}

func TestStopProcess_LiveProcess(t *testing.T) {
	dir := t.TempDir()
	pidPath := filepath.Join(dir, "candela.pid")

	cmd := helperProcessCommand("wait")
	if err := cmd.Start(); err != nil {
		t.Fatalf("failed to start process: %v", err)
	}

	pid := cmd.Process.Pid

	// Reap the child in background so the zombie is cleaned up and
	// Signal(0) correctly returns an error for a dead process.
	waitDone := make(chan struct{})
	go func() {
		_ = cmd.Wait()
		close(waitDone)
	}()
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		<-waitDone
	})

	if err := os.WriteFile(pidPath, []byte(strconv.Itoa(pid)), 0o644); err != nil {
		t.Fatal(err)
	}

	// stopProcess should SIGTERM it and wait for exit.
	if err := stopProcess(pidPath); err != nil {
		t.Fatalf("stopProcess returned error for live process: %v", err)
	}

	// Wait for the child to be reaped.
	<-waitDone

	// Process should be dead now.
	if processRunning(pid) {
		t.Error("expected process to be dead after stopProcess")
	}

	// PID file should be cleaned up.
	if _, err := os.Stat(pidPath); err == nil {
		t.Error("expected PID file to be removed after stopping")
	}
}

func TestStopProcess_VerifiesExitBeforeReturn(t *testing.T) {
	dir := t.TempDir()
	pidPath := filepath.Join(dir, "candela.pid")

	// Start a short-lived process that will exit on its own before
	// the SIGTERM wait loop completes, testing the "dies mid-wait" path.
	cmd := helperProcessCommand("short")
	if err := cmd.Start(); err != nil {
		t.Fatalf("failed to start process: %v", err)
	}
	pid := cmd.Process.Pid

	// Reap in background.
	go func() { _ = cmd.Wait() }()

	if err := os.WriteFile(pidPath, []byte(strconv.Itoa(pid)), 0o644); err != nil {
		t.Fatal(err)
	}

	// stopProcess should handle the process dying mid-wait gracefully.
	if err := stopProcess(pidPath); err != nil {
		t.Fatalf("stopProcess returned error: %v", err)
	}

	// PID file must be gone — this is the key invariant that prevents
	// the race condition (cmdStart checks for existing PID files).
	if _, err := os.Stat(pidPath); err == nil {
		t.Error("PID file must be removed before stopProcess returns")
	}
}

func helperProcessCommand(mode string) *exec.Cmd {
	cmd := exec.Command(os.Args[0], "-test.run=TestHelperProcess", "--", mode)
	cmd.Env = append(os.Environ(), "CANDELA_HELPER_PROCESS=1")
	configureBackgroundCommand(cmd) // CREATE_NEW_PROCESS_GROUP on Windows — prevents CTRL_BREAK_EVENT from leaking to the parent shell
	return cmd
}

func TestHelperProcess(t *testing.T) {
	if os.Getenv("CANDELA_HELPER_PROCESS") != "1" {
		return
	}
	if len(os.Args) == 0 {
		os.Exit(2)
	}
	mode := os.Args[len(os.Args)-1]
	switch mode {
	case "wait":
		select {}
	case "short":
		time.Sleep(100 * time.Millisecond)
		os.Exit(0)
	default:
		os.Exit(2)
	}
}
