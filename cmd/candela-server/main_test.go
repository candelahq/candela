package main

import (
	"testing"
)

func TestValidateAuthConfig(t *testing.T) {
	tests := []struct {
		name        string
		devMode     bool
		kService    string // set as K_SERVICE env var for the test
		cloudRunURL string
		wantErr     bool
	}{
		{"dev mode on Cloud Run is rejected", true, "candela-server", "https://candela.run.app", true},
		{"dev mode locally is allowed", true, "", "", false},
		{"prod mode on Cloud Run is allowed", false, "candela-server", "https://candela.run.app", false},
		{"prod mode locally is allowed", false, "", "", false},
		{"dev mode on Cloud Run without CLOUD_RUN_URL is rejected", true, "candela-server", "", true},
		{"prod mode on Cloud Run without CLOUD_RUN_URL warns but succeeds", false, "candela-server", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clear all production indicators so inherited env doesn't leak (#648).
			for _, env := range []string{
				"K_SERVICE", "K_REVISION", "KUBERNETES_SERVICE_HOST",
				"ECS_CONTAINER_METADATA_URI", "AWS_LAMBDA_FUNCTION_NAME",
				"CONTAINER_APP_NAME", "GAE_ENV",
			} {
				t.Setenv(env, "")
			}
			// Set K_SERVICE for this specific test case.
			if tt.kService != "" {
				t.Setenv("K_SERVICE", tt.kService)
			}
			err := validateAuthConfig(tt.devMode, tt.kService, tt.cloudRunURL)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateAuthConfig(devMode=%v, kService=%q, cloudRunURL=%q) error = %v, wantErr %v",
					tt.devMode, tt.kService, tt.cloudRunURL, err, tt.wantErr)
			}
		})
	}
}
