package proxy

import (
	"testing"
)

func boolPtr(b bool) *bool { return &b }

func TestEffectiveHost(t *testing.T) {
	tests := []struct {
		name     string
		provider Provider
		want     string
	}{
		{
			name:     "derives from UpstreamURL",
			provider: Provider{Name: "openai", UpstreamURL: "https://api.openai.com"},
			want:     "api.openai.com",
		},
		{
			name:     "derives from UpstreamURL with path",
			provider: Provider{Name: "gemini-oai", UpstreamURL: "https://generativelanguage.googleapis.com/v1beta/openai"},
			want:     "generativelanguage.googleapis.com",
		},
		{
			name:     "explicit Host takes precedence",
			provider: Provider{Name: "custom", UpstreamURL: "https://api.openai.com", Host: "custom.example.com"},
			want:     "custom.example.com",
		},
		{
			name:     "empty UpstreamURL returns empty",
			provider: Provider{Name: "empty"},
			want:     "",
		},
		{
			name:     "invalid URL returns empty",
			provider: Provider{Name: "bad", UpstreamURL: "://not-a-url"},
			want:     "",
		},
		{
			name:     "URL with port strips port",
			provider: Provider{Name: "local", UpstreamURL: "http://localhost:8080"},
			want:     "localhost",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.provider.EffectiveHost()
			if got != tt.want {
				t.Errorf("EffectiveHost() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestShouldIntercept(t *testing.T) {
	tests := []struct {
		name     string
		provider Provider
		want     bool
	}{
		{
			name:     "nil defaults to true",
			provider: Provider{Name: "openai"},
			want:     true,
		},
		{
			name:     "explicit true",
			provider: Provider{Name: "openai", Intercept: boolPtr(true)},
			want:     true,
		},
		{
			name:     "explicit false",
			provider: Provider{Name: "gemini-oai", Intercept: boolPtr(false)},
			want:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.provider.ShouldIntercept()
			if got != tt.want {
				t.Errorf("ShouldIntercept() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestBuildSNIMap(t *testing.T) {
	providers := []Provider{
		{Name: "openai", UpstreamURL: "https://api.openai.com"},
		{Name: "google", UpstreamURL: "https://generativelanguage.googleapis.com"},
		// gemini-oai shares host with google, should not intercept.
		{Name: "gemini-oai", UpstreamURL: "https://generativelanguage.googleapis.com/v1beta/openai", Intercept: boolPtr(false)},
		// anthropic uses suffix pattern for regional Vertex AI endpoints.
		{Name: "anthropic", UpstreamURL: "https://us-central1-aiplatform.googleapis.com", HostPattern: "*-aiplatform.googleapis.com"},
		// anthropic-vertex shares host with anthropic, should not intercept.
		{Name: "anthropic-vertex", UpstreamURL: "https://us-central1-aiplatform.googleapis.com", Intercept: boolPtr(false)},
		{Name: "anthropic-direct", UpstreamURL: "https://api.anthropic.com"},
	}

	m := BuildSNIMap(providers)

	t.Run("exact match", func(t *testing.T) {
		tests := []struct {
			hostname string
			wantName string
			wantOK   bool
		}{
			{"api.openai.com", "openai", true},
			{"generativelanguage.googleapis.com", "google", true},
			{"api.anthropic.com", "anthropic-direct", true},
			{"unknown.example.com", "", false},
		}
		for _, tt := range tests {
			name, ok := m.Lookup(tt.hostname)
			if ok != tt.wantOK || name != tt.wantName {
				t.Errorf("Lookup(%q) = (%q, %v), want (%q, %v)",
					tt.hostname, name, ok, tt.wantName, tt.wantOK)
			}
		}
	})

	t.Run("wildcard match", func(t *testing.T) {
		tests := []struct {
			hostname string
			wantName string
			wantOK   bool
		}{
			{"us-central1-aiplatform.googleapis.com", "anthropic", true},
			{"europe-west4-aiplatform.googleapis.com", "anthropic", true},
			{"asia-northeast1-aiplatform.googleapis.com", "anthropic", true},
		}
		for _, tt := range tests {
			name, ok := m.Lookup(tt.hostname)
			if ok != tt.wantOK || name != tt.wantName {
				t.Errorf("Lookup(%q) = (%q, %v), want (%q, %v)",
					tt.hostname, name, ok, tt.wantName, tt.wantOK)
			}
		}
	})

	t.Run("non-intercepted providers excluded", func(t *testing.T) {
		// gemini-oai has Intercept=false, so the shared host should resolve to "google", not "gemini-oai".
		name, ok := m.Lookup("generativelanguage.googleapis.com")
		if !ok || name != "google" {
			t.Errorf("expected google for shared host, got (%q, %v)", name, ok)
		}
	})

	t.Run("Hosts returns all registered entries", func(t *testing.T) {
		hosts := m.Hosts()
		// Should have: api.openai.com, generativelanguage.googleapis.com,
		// api.anthropic.com, *-aiplatform.googleapis.com
		if len(hosts) != 4 {
			t.Errorf("Hosts() returned %d entries, want 4: %v", len(hosts), hosts)
		}
	})
}

func TestBuildSNIMapFromDefaultProviders(t *testing.T) {
	// BuildSNIMap should work with DefaultProviders() — all have intercept=true (nil).
	m := BuildSNIMap(DefaultProviders())

	// All default providers derive unique hosts (except anthropic/anthropic-vertex share one).
	name, ok := m.Lookup("api.openai.com")
	if !ok || name != "openai" {
		t.Errorf("expected openai for api.openai.com, got (%q, %v)", name, ok)
	}

	name, ok = m.Lookup("api.anthropic.com")
	if !ok || name != "anthropic-direct" {
		t.Errorf("expected anthropic-direct for api.anthropic.com, got (%q, %v)", name, ok)
	}

	// Two providers share the same host — first one wins.
	name, ok = m.Lookup("us-central1-aiplatform.googleapis.com")
	if !ok {
		t.Error("expected a match for us-central1-aiplatform.googleapis.com")
	}
	// Either "anthropic" or "anthropic-vertex" is acceptable since both share the host.
	if name != "anthropic" && name != "anthropic-vertex" {
		t.Errorf("expected anthropic or anthropic-vertex, got %q", name)
	}
}

func TestSNIMapNil(t *testing.T) {
	var m *SNIMap
	name, ok := m.Lookup("api.openai.com")
	if ok || name != "" {
		t.Errorf("nil SNIMap should return empty, got (%q, %v)", name, ok)
	}
	hosts := m.Hosts()
	if hosts != nil {
		t.Errorf("nil SNIMap.Hosts() should return nil, got %v", hosts)
	}
}

func TestSNIMapWildcardBoundary(t *testing.T) {
	// SECURITY: wildcard *-aiplatform.googleapis.com uses suffix pattern.
	// The "*" matches any prefix before "-aiplatform.googleapis.com".
	providers := []Provider{
		{Name: "anthropic", UpstreamURL: "https://us-central1-aiplatform.googleapis.com", HostPattern: "*-aiplatform.googleapis.com"},
	}
	m := BuildSNIMap(providers)

	tests := []struct {
		hostname string
		wantOK   bool
		desc     string
	}{
		// Should match: GCP regional endpoints.
		{"us-central1-aiplatform.googleapis.com", true, "valid GCP region"},
		{"europe-west4-aiplatform.googleapis.com", true, "valid GCP region"},
		{"a-aiplatform.googleapis.com", true, "single-char prefix"},

		// Must NOT match: no "-aiplatform" suffix.
		{"aiplatform.googleapis.com", false, "bare suffix (no prefix)"},
		{"api.openai.com", false, "unrelated hostname"},

		// Now test subdomain-style wildcards too.
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			_, ok := m.Lookup(tt.hostname)
			if ok != tt.wantOK {
				t.Errorf("Lookup(%q) ok = %v, want %v", tt.hostname, ok, tt.wantOK)
			}
		})
	}

	// Also verify subdomain-style wildcards (*.example.com) still work.
	subProviders := []Provider{
		{Name: "test", HostPattern: "*.example.com"},
	}
	sm := BuildSNIMap(subProviders)

	subTests := []struct {
		hostname string
		wantOK   bool
		desc     string
	}{
		{"sub.example.com", true, "subdomain match"},
		{"deep.sub.example.com", true, "deep subdomain"},
		{"example.com", false, "bare domain (no subdomain)"},
		{"notexample.com", false, "different domain entirely"},
	}

	for _, tt := range subTests {
		t.Run("subdomain/"+tt.desc, func(t *testing.T) {
			_, ok := sm.Lookup(tt.hostname)
			if ok != tt.wantOK {
				t.Errorf("Lookup(%q) ok = %v, want %v", tt.hostname, ok, tt.wantOK)
			}
		})
	}
}
