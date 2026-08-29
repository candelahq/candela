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
			// Set K_SERVICE so validateAuthConfig can detect Cloud Run (#648).
			t.Setenv("K_SERVICE", tt.kService)
			err := validateAuthConfig(tt.devMode, tt.kService, tt.cloudRunURL)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateAuthConfig(devMode=%v, kService=%q, cloudRunURL=%q) error = %v, wantErr %v",
					tt.devMode, tt.kService, tt.cloudRunURL, err, tt.wantErr)
			}
		})
	}
}
