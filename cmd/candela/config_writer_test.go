package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestUpsertConfigProvider_NewFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	// Override config path via env var.
	t.Setenv("CANDELA_CONFIG", path)

	changed, err := upsertConfigProvider("gcp")
	if err != nil {
		t.Fatalf("upsertConfigProvider: %v", err)
	}
	if !changed {
		t.Error("expected changed=true for new file")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "providers:") {
		t.Error("expected 'providers:' in config file")
	}
	if !strings.Contains(content, "name: google") {
		t.Error("expected 'name: google' in config file (gcp maps to google)")
	}
}

func TestUpsertConfigProvider_ExistingConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	// Write an existing config.
	existing := "port: 8181\n\nproviders:\n  - name: anthropic\n\nvertex_ai:\n  project: my-project\n"
	if err := os.WriteFile(path, []byte(existing), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	t.Setenv("CANDELA_CONFIG", path)

	// Add GCP provider.
	changed, err := upsertConfigProvider("gcp")
	if err != nil {
		t.Fatalf("upsertConfigProvider(gcp): %v", err)
	}
	if !changed {
		t.Error("expected changed=true when adding new provider")
	}

	data, _ := os.ReadFile(path)
	content := string(data)
	if !strings.Contains(content, "name: google") {
		t.Error("expected 'name: google' to be added")
	}
	// Original content should be preserved.
	if !strings.Contains(content, "port: 8181") {
		t.Error("expected original 'port: 8181' to be preserved")
	}
	if !strings.Contains(content, "name: anthropic") {
		t.Error("expected original 'name: anthropic' to be preserved")
	}
}

func TestUpsertConfigProvider_AlreadyPresent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	existing := "providers:\n  - name: google\n"
	if err := os.WriteFile(path, []byte(existing), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	t.Setenv("CANDELA_CONFIG", path)

	changed, err := upsertConfigProvider("gcp")
	if err != nil {
		t.Fatalf("upsertConfigProvider: %v", err)
	}
	if changed {
		t.Error("expected changed=false when provider already exists")
	}
}

func TestUpsertConfigProvider_AWSMapping(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	t.Setenv("CANDELA_CONFIG", path)

	changed, err := upsertConfigProvider("aws")
	if err != nil {
		t.Fatalf("upsertConfigProvider(aws): %v", err)
	}
	if !changed {
		t.Error("expected changed=true")
	}

	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), "name: anthropic-bedrock") {
		t.Error("expected 'name: anthropic-bedrock' (aws maps to anthropic-bedrock)")
	}
}

func TestIsProxyRunning_NoPidFile(t *testing.T) {
	// With a non-existent home dir, isProxyRunning should return false.
	t.Setenv("HOME", t.TempDir())
	if isProxyRunning() {
		t.Error("expected isProxyRunning()=false when no PID file exists")
	}
}

func TestIsProxyRunning_StalePid(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	// Create .candela dir and write a stale PID.
	candelaDir := filepath.Join(dir, ".candela")
	if err := os.MkdirAll(candelaDir, 0o700); err != nil {
		t.Fatal(err)
	}
	// PID 99999999 almost certainly doesn't exist.
	if err := os.WriteFile(filepath.Join(candelaDir, "candela.pid"), []byte("99999999"), 0o644); err != nil {
		t.Fatal(err)
	}

	if isProxyRunning() {
		t.Error("expected isProxyRunning()=false for stale PID")
	}
}
