package candela

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestTransport_InjectsBaggage(t *testing.T) {
	var captured http.Header
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured = r.Header.Clone()
	}))
	defer srv.Close()

	client := &http.Client{
		Transport: NewTransport(nil,
			WithTenantID("acme"),
			WithJobID("run-42"),
		),
	}

	req, _ := http.NewRequest("GET", srv.URL, nil)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()

	baggage := captured.Get("Baggage")
	if !strings.Contains(baggage, "candela.tenant_id=acme") {
		t.Errorf("baggage missing tenant_id: %q", baggage)
	}
	if !strings.Contains(baggage, "candela.job_id=run-42") {
		t.Errorf("baggage missing job_id: %q", baggage)
	}
	if captured.Get("X-Candela-Tenant-Id") != "acme" {
		t.Errorf("X-Candela-Tenant-Id = %q, want acme", captured.Get("X-Candela-Tenant-Id"))
	}
	if captured.Get("X-Candela-Job-Id") != "run-42" {
		t.Errorf("X-Candela-Job-Id = %q, want run-42", captured.Get("X-Candela-Job-Id"))
	}
}

func TestTransport_PreservesExistingBaggage(t *testing.T) {
	var captured http.Header
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured = r.Header.Clone()
	}))
	defer srv.Close()

	client := &http.Client{
		Transport: NewTransport(nil, WithTenantID("acme")),
	}

	req, _ := http.NewRequest("GET", srv.URL, nil)
	req.Header.Set("Baggage", "existing=value")
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()

	baggage := captured.Get("Baggage")
	if !strings.Contains(baggage, "existing=value") {
		t.Errorf("existing baggage lost: %q", baggage)
	}
	if !strings.Contains(baggage, "candela.tenant_id=acme") {
		t.Errorf("candela baggage not appended: %q", baggage)
	}
}

func TestTransport_RejectsInvalidID(t *testing.T) {
	client := &http.Client{
		Transport: NewTransport(nil, WithTenantID("invalid spaces!")),
	}
	req, _ := http.NewRequest("GET", "http://localhost", nil)
	_, err := client.Do(req)
	if err == nil {
		t.Fatal("expected error for invalid tenant_id")
	}
	if !strings.Contains(err.Error(), "invalid tenant_id") {
		t.Errorf("error = %v, want invalid tenant_id", err)
	}
}

func TestInjectHeaders(t *testing.T) {
	req, _ := http.NewRequest("GET", "http://localhost", nil)
	err := InjectHeaders(req, WithTenantID("t1"), WithJobID("j1"))
	if err != nil {
		t.Fatal(err)
	}

	if req.Header.Get("X-Candela-Tenant-Id") != "t1" {
		t.Errorf("tenant header = %q, want t1", req.Header.Get("X-Candela-Tenant-Id"))
	}
	if req.Header.Get("X-Candela-Job-Id") != "j1" {
		t.Errorf("job header = %q, want j1", req.Header.Get("X-Candela-Job-Id"))
	}

	baggage := req.Header.Get("Baggage")
	if !strings.Contains(baggage, "candela.tenant_id=t1") {
		t.Errorf("baggage missing tenant: %q", baggage)
	}
	if !strings.Contains(baggage, "candela.job_id=j1") {
		t.Errorf("baggage missing job: %q", baggage)
	}
}

func TestTransport_NoOptions(t *testing.T) {
	var captured http.Header
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured = r.Header.Clone()
	}))
	defer srv.Close()

	client := &http.Client{Transport: NewTransport(nil)}
	req, _ := http.NewRequest("GET", srv.URL, nil)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()

	if captured.Get("Baggage") != "" {
		t.Errorf("unexpected baggage: %q", captured.Get("Baggage"))
	}
}
