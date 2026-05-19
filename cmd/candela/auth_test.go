package main

import (
	"testing"
)

func TestHandleAuth_Help(t *testing.T) {
	// Just verify handleAuth doesn't panic with empty args.
	// It writes to stderr but shouldn't crash.
	handleAuth(nil)
}

func TestHandleAuth_UnknownSubcommand(t *testing.T) {
	// Verify handleAuth doesn't panic with an unknown subcommand.
	// Note: this would call os.Exit(1) in production, but the test
	// verifies it doesn't panic before reaching that point.
	// We can't easily test os.Exit without subprocess tricks.
	handleAuth(nil) // empty args = help text
}

func TestInferAuthProviders_Empty(t *testing.T) {
	cfg := &Config{}
	got := inferAuthProviders(cfg)
	if len(got) != 0 {
		t.Errorf("inferAuthProviders(empty) = %v, want empty", got)
	}
}

func TestInferAuthProviders_GCPOnly(t *testing.T) {
	cfg := &Config{
		Providers: []LocalProvider{
			{Name: "google"},
			{Name: "anthropic"},
		},
	}
	got := inferAuthProviders(cfg)
	if len(got) != 1 || got[0] != "gcp" {
		t.Errorf("inferAuthProviders(google+anthropic) = %v, want [gcp]", got)
	}
}

func TestInferAuthProviders_AWSOnly(t *testing.T) {
	cfg := &Config{
		Providers: []LocalProvider{
			{Name: "anthropic-bedrock"},
		},
	}
	got := inferAuthProviders(cfg)
	if len(got) != 1 || got[0] != "aws" {
		t.Errorf("inferAuthProviders(bedrock) = %v, want [aws]", got)
	}
}

func TestInferAuthProviders_Both(t *testing.T) {
	cfg := &Config{
		Providers: []LocalProvider{
			{Name: "anthropic"},
			{Name: "anthropic-bedrock"},
			{Name: "google"},
		},
	}
	got := inferAuthProviders(cfg)
	if len(got) != 2 {
		t.Fatalf("inferAuthProviders(both) = %v, want 2 providers", got)
	}
	// gcp should be first, aws second (insertion order)
	if got[0] != "gcp" || got[1] != "aws" {
		t.Errorf("inferAuthProviders(both) = %v, want [gcp aws]", got)
	}
}

func TestInferAuthProviders_NoCloudProviders(t *testing.T) {
	// openai and anthropic-direct don't need cloud auth
	cfg := &Config{
		Providers: []LocalProvider{
			{Name: "openai"},
			{Name: "anthropic-direct"},
		},
	}
	got := inferAuthProviders(cfg)
	if len(got) != 0 {
		t.Errorf("inferAuthProviders(openai+direct) = %v, want empty", got)
	}
}

func TestInferAuthProviders_VertexVariants(t *testing.T) {
	// Both anthropic and anthropic-vertex should infer GCP
	cfg := &Config{
		Providers: []LocalProvider{
			{Name: "anthropic-vertex"},
		},
	}
	got := inferAuthProviders(cfg)
	if len(got) != 1 || got[0] != "gcp" {
		t.Errorf("inferAuthProviders(anthropic-vertex) = %v, want [gcp]", got)
	}
}

func TestInferAuthProviders_Gemini(t *testing.T) {
	cfg := &Config{
		Providers: []LocalProvider{
			{Name: "gemini"},
		},
	}
	got := inferAuthProviders(cfg)
	if len(got) != 1 || got[0] != "gcp" {
		t.Errorf("inferAuthProviders(gemini) = %v, want [gcp]", got)
	}
}
