package main

import (
	"os"
	"strings"
	"testing"
)

// Integration tests verify the sidecar binary's structural correctness.

func TestSidecar_Dockerfile_Exists(t *testing.T) {
	_, err := os.Stat("../../Dockerfile.sidecar")
	if os.IsNotExist(err) {
		t.Fatal("Dockerfile.sidecar missing — sidecar cannot be containerized")
	}
}

func TestSidecar_CIWorkflow_References_SidecarTag(t *testing.T) {
	data, err := os.ReadFile("../../.github/workflows/sidecar.yml")
	if err != nil {
		t.Fatalf("sidecar CI workflow missing: %v", err)
	}
	content := string(data)

	if !strings.Contains(content, "sidecar-v*") {
		t.Error("sidecar.yml does not trigger on sidecar-v* tags")
	}
	if !strings.Contains(content, "Dockerfile.sidecar") {
		t.Error("sidecar.yml does not reference Dockerfile.sidecar")
	}
	// After deduplication, sidecar.yml delegates to the reusable workflow
	if !strings.Contains(content, "docker-build-push.yml") {
		t.Error("sidecar.yml does not use the reusable docker-build-push workflow")
	}

	// Verify the reusable workflow publishes to GHCR
	reusable, err := os.ReadFile("../../.github/workflows/docker-build-push.yml")
	if err != nil {
		t.Fatalf("reusable workflow missing: %v", err)
	}
	if !strings.Contains(string(reusable), "ghcr.io") {
		t.Error("docker-build-push.yml does not publish to GHCR")
	}
}

func TestSidecar_GitignoreIncludesBinary(t *testing.T) {
	data, err := os.ReadFile("../../.gitignore")
	if err != nil {
		t.Fatalf("failed to read .gitignore: %v", err)
	}
	if !strings.Contains(string(data), "/candela-sidecar") {
		t.Error(".gitignore missing /candela-sidecar entry — binary will be committed")
	}
}
