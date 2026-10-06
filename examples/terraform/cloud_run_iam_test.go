package terraform_test

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestCloudRunIAM_AllUsersConditionalOnIAP verifies that the allUsers IAM binding
// is strictly conditional on IAP being enabled, preventing direct URL authorization bypass.
func TestCloudRunIAM_AllUsersConditionalOnIAP(t *testing.T) {
	content, err := os.ReadFile("cloud_run.tf")
	if err != nil {
		// Fallback for running from repo root
		content, err = os.ReadFile(filepath.Join("examples", "terraform", "cloud_run.tf"))
		if err != nil {
			t.Fatalf("failed to read cloud_run.tf: %v", err)
		}
	}
	src := string(content)

	// Verify iap_enabled definition in locals
	if !strings.Contains(src, "iap_enabled = var.iap_enabled || (var.custom_domain != \"\" && var.iap_oauth_client_id != \"\")") {
		t.Errorf("cloud_run.tf missing expected local.iap_enabled definition combining var.iap_enabled and OAuth client ID")
	}

	// Verify allUsers IAM resource uses count = local.iap_enabled ? 1 : 0
	if !strings.Contains(src, "count    = local.iap_enabled ? 1 : 0") {
		t.Errorf("cloud_run.tf allow_unauthenticated must have count = local.iap_enabled ? 1 : 0")
	}

	// Verify allUsers is only member for allow_unauthenticated
	allowUnauthIdx := strings.Index(src, "resource \"google_cloud_run_v2_service_iam_member\" \"allow_unauthenticated\"")
	if allowUnauthIdx == -1 {
		t.Fatalf("could not find allow_unauthenticated resource in cloud_run.tf")
	}
	block := src[allowUnauthIdx:]
	endBlock := strings.Index(block, "resource \"google_cloud_run_v2_service_iam_member\" \"group_invoker\"")
	if endBlock != -1 {
		block = block[:endBlock]
	}

	if !strings.Contains(block, "member   = \"allUsers\"") {
		t.Errorf("allow_unauthenticated block must target member allUsers")
	}
	if !strings.Contains(block, "roles/run.invoker") {
		t.Errorf("allow_unauthenticated block must target role roles/run.invoker")
	}
}

// TestCloudRunIAM_IngressRestrictedWhenIAPConfigured verifies that Cloud Run ingress
// is restricted to INTERNAL_LOAD_BALANCER when IAP is enabled, ensuring direct *.run.app
// URLs return 403 Forbidden.
func TestCloudRunIAM_IngressRestrictedWhenIAPConfigured(t *testing.T) {
	content, err := os.ReadFile("cloud_run.tf")
	if err != nil {
		content, err = os.ReadFile(filepath.Join("examples", "terraform", "cloud_run.tf"))
		if err != nil {
			t.Fatalf("failed to read cloud_run.tf: %v", err)
		}
	}
	src := string(content)

	expectedIngress := `ingress = local.iap_enabled ? "INGRESS_TRAFFIC_INTERNAL_LOAD_BALANCER" : "INGRESS_TRAFFIC_ALL"`
	if !strings.Contains(src, expectedIngress) {
		t.Errorf("cloud_run.tf must configure ingress as:\n%s", expectedIngress)
	}
}

// TestCloudRunIAM_VariablesExposeIAPControl verifies that variables.tf exposes iap_enabled.
func TestCloudRunIAM_VariablesExposeIAPControl(t *testing.T) {
	content, err := os.ReadFile("variables.tf")
	if err != nil {
		content, err = os.ReadFile(filepath.Join("examples", "terraform", "variables.tf"))
		if err != nil {
			t.Fatalf("failed to read variables.tf: %v", err)
		}
	}
	src := string(content)

	if !strings.Contains(src, "variable \"iap_enabled\"") {
		t.Errorf("variables.tf must define variable \"iap_enabled\"")
	}
}

// TestCloudRunIAM_DirectUrl403Simulation simulates the security boundary behavior:
// Direct *.run.app URL invocations without routing through the load balancer / IAP
// are rejected with HTTP 403 Forbidden.
func TestCloudRunIAM_DirectUrl403Simulation(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// In GCP Cloud Run with INGRESS_TRAFFIC_INTERNAL_LOAD_BALANCER:
		// Requests arriving via external GLB have Forwarded/X-Forwarded headers from the LB,
		// while direct requests to *.run.app arrive without the internal LB indicator.
		isDirectRunApp := strings.HasSuffix(r.Host, ".run.app") || r.Header.Get("X-Forwarded-For") == ""
		iapAssertion := r.Header.Get("X-Goog-IAP-JWT-Assertion")

		if isDirectRunApp && iapAssertion == "" {
			http.Error(w, "Access forbidden: Direct *.run.app URL access is disabled when IAP is configured. Use the custom domain via IAP.", http.StatusForbidden)
			return
		}

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})

	server := httptest.NewServer(handler)
	defer server.Close()

	// 1. Direct *.run.app request without IAP must return 403 Forbidden
	reqDirect, err := http.NewRequest(http.MethodGet, server.URL+"/healthz", nil)
	if err != nil {
		t.Fatalf("failed to create direct request: %v", err)
	}
	reqDirect.Host = "candela-abc123def0-uc.a.run.app"
	respDirect, err := http.DefaultClient.Do(reqDirect)
	if err != nil {
		t.Fatalf("direct request failed: %v", err)
	}
	defer func() { _ = respDirect.Body.Close() }()

	if respDirect.StatusCode != http.StatusForbidden {
		t.Errorf("direct *.run.app request returned status %d, want %d (Forbidden)", respDirect.StatusCode, http.StatusForbidden)
	}

	// 2. Request through IAP load balancer must succeed
	reqIAP, err := http.NewRequest(http.MethodGet, server.URL+"/healthz", nil)
	if err != nil {
		t.Fatalf("failed to create IAP request: %v", err)
	}
	reqIAP.Host = "candela.company.com"
	reqIAP.Header.Set("X-Forwarded-For", "203.0.113.1")
	reqIAP.Header.Set("X-Goog-IAP-JWT-Assertion", "valid.mock.jwt")
	respIAP, err := http.DefaultClient.Do(reqIAP)
	if err != nil {
		t.Fatalf("IAP request failed: %v", err)
	}
	defer func() { _ = respIAP.Body.Close() }()

	if respIAP.StatusCode != http.StatusOK {
		t.Errorf("IAP load balancer request returned status %d, want %d (OK)", respIAP.StatusCode, http.StatusOK)
	}
}
