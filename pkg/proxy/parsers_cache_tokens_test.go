package proxy

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// cacheTokenCase is one provider entry in test/fixtures/cache_tokens.json.
// The same file drives the Rust proxy's tests, so a provider that is measured
// differently by the two binaries fails one of them.
type cacheTokenCase struct {
	Provider            string          `json:"provider"`
	StandardBody        json.RawMessage `json:"standard_body"`
	StreamingBody       string          `json:"streaming_body"`
	CacheReadTokens     int64           `json:"cache_read_tokens"`
	CacheCreationTokens int64           `json:"cache_creation_tokens"`
}

func loadCacheTokenFixture(t *testing.T) []cacheTokenCase {
	t.Helper()

	path := filepath.Join("..", "..", "test", "fixtures", "cache_tokens.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}

	var fixture struct {
		Cases []cacheTokenCase `json:"cases"`
	}
	if err := json.Unmarshal(data, &fixture); err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	if len(fixture.Cases) == 0 {
		t.Fatalf("%s has no cases", path)
	}
	return fixture.Cases
}

func TestCacheTokenFixture_StandardBodies(t *testing.T) {
	for _, tc := range loadCacheTokenFixture(t) {
		ct := extractCacheTokens(tc.Provider, tc.StandardBody)
		if ct.CacheReadTokens != tc.CacheReadTokens || ct.CacheCreationTokens != tc.CacheCreationTokens {
			t.Errorf("%s standard body: got read=%d creation=%d, want read=%d creation=%d",
				tc.Provider, ct.CacheReadTokens, ct.CacheCreationTokens,
				tc.CacheReadTokens, tc.CacheCreationTokens)
		}
	}
}

func TestCacheTokenFixture_StreamingBodies(t *testing.T) {
	for _, tc := range loadCacheTokenFixture(t) {
		ct := extractStreamingCacheTokens(tc.Provider, []byte(tc.StreamingBody))
		if ct.CacheReadTokens != tc.CacheReadTokens || ct.CacheCreationTokens != tc.CacheCreationTokens {
			t.Errorf("%s streaming body: got read=%d creation=%d, want read=%d creation=%d",
				tc.Provider, ct.CacheReadTokens, ct.CacheCreationTokens,
				tc.CacheReadTokens, tc.CacheCreationTokens)
		}
	}
}

func TestCacheTokenFixture_CoversRegistryExactly(t *testing.T) {
	seen := make(map[string]bool)
	for _, tc := range loadCacheTokenFixture(t) {
		if seen[tc.Provider] {
			t.Errorf("provider %q appears twice in the fixture", tc.Provider)
		}
		seen[tc.Provider] = true
		if _, ok := parserRegistry[tc.Provider]; !ok {
			t.Errorf("provider %q has a fixture case but no parserRegistry entry", tc.Provider)
		}
	}
	for provider := range parserRegistry {
		if !seen[provider] {
			t.Errorf("provider %q is in parserRegistry but has no fixture case", provider)
		}
	}
}
