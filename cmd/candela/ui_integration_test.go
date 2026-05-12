package main

import (
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestEmbeddedUI_ServesHTML verifies that the embedded management UI is served
// correctly at /_local/ and that the HTML contains the expected structure.
func TestEmbeddedUI_ServesHTML(t *testing.T) {
	mux := http.NewServeMux()
	uiContent, err := fs.Sub(uiFS, "ui")
	if err != nil {
		t.Fatal(err)
	}
	mux.Handle("/_local/", http.StripPrefix("/_local/", http.FileServer(http.FS(uiContent))))

	req := httptest.NewRequest(http.MethodGet, "/_local/", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	body, _ := io.ReadAll(w.Body)
	html := string(body)

	// Core structure.
	checks := []string{
		"Candela",
		"app.js",
		"style.css",
		"models-list",
		"popular-grid",
		"pull-form",
		"pull-input",
		"btn-start",
		"btn-stop",
		"btn-reset",
	}
	for _, want := range checks {
		if !strings.Contains(html, want) {
			t.Errorf("HTML missing expected content: %q", want)
		}
	}
}

// TestEmbeddedUI_ServesCSS verifies that style.css is served.
func TestEmbeddedUI_ServesCSS(t *testing.T) {
	mux := http.NewServeMux()
	uiContent, err := fs.Sub(uiFS, "ui")
	if err != nil {
		t.Fatal(err)
	}
	mux.Handle("/_local/", http.StripPrefix("/_local/", http.FileServer(http.FS(uiContent))))

	req := httptest.NewRequest(http.MethodGet, "/_local/style.css", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	body, _ := io.ReadAll(w.Body)
	css := string(body)

	// Verify new UI classes exist.
	cssChecks := []string{
		".model-actions",
		".btn-danger",
		".pull-cancel",
		".model-row",
		".popular-item",
	}
	for _, want := range cssChecks {
		if !strings.Contains(css, want) {
			t.Errorf("CSS missing expected class: %q", want)
		}
	}
}

// TestEmbeddedUI_ServesJS verifies that app.js is served and contains the
// new feature APIs (DeleteModel, CancelPull, ListCatalog).
func TestEmbeddedUI_ServesJS(t *testing.T) {
	mux := http.NewServeMux()
	uiContent, err := fs.Sub(uiFS, "ui")
	if err != nil {
		t.Fatal(err)
	}
	mux.Handle("/_local/", http.StripPrefix("/_local/", http.FileServer(http.FS(uiContent))))

	req := httptest.NewRequest(http.MethodGet, "/_local/app.js", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	body, _ := io.ReadAll(w.Body)
	js := string(body)

	// Verify new feature RPC calls are present.
	jsChecks := []struct {
		name string
		want string
	}{
		{"DeleteModel RPC call", "'DeleteModel'"},
		{"CancelPull RPC call", "'CancelPull'"},
		{"ListCatalog RPC call", "'ListCatalog'"},
		{"deleteModel function", "async function deleteModel"},
		{"cancelPull function", "async function cancelPull"},
		{"renderPopularModels is async", "async function renderPopularModels"},
		{"delete confirmation", "Delete"},
		{"cancel button", "cancel-pull"},
		{"trash icon button", "btn-danger"},
		{"auto-refresh on complete", "completedPulls"},
		{"refreshModels on pull complete", "refreshModels()"},
	}
	for _, tc := range jsChecks {
		if !strings.Contains(js, tc.want) {
			t.Errorf("JS missing %s: expected to find %q", tc.name, tc.want)
		}
	}

	// Verify hardcoded POPULAR_MODELS is gone.
	if strings.Contains(js, "const POPULAR_MODELS") {
		t.Error("JS still contains hardcoded POPULAR_MODELS; should use ListCatalog RPC")
	}
}
