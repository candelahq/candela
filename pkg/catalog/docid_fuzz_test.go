package catalog

import (
	"net/url"
	"strings"
	"testing"
)

// FuzzDocID exercises the docID function with arbitrary provider and modelID
// inputs. It must never panic regardless of input.
func FuzzDocID(f *testing.F) {
	// Seed corpus: normal names, slashes, unicode, empty, special chars.
	f.Add("zai", "glm-5-maas")
	f.Add("anthropic", "claude-sonnet-4-20250514")
	f.Add("google", "gemini-2.5-pro")
	f.Add("meta-llama", "Llama-3/variant")
	f.Add("publisher/sub", "model/version")
	f.Add("模型提供者", "模型-test")
	f.Add("", "")
	f.Add("", "model")
	f.Add("provider", "")
	f.Add("a%00b", "c%00d")
	f.Add("provider\nwith\nnewlines", "model\twith\ttabs")
	f.Add("provider with spaces", "model with spaces")
	f.Add(strings.Repeat("a", 500), strings.Repeat("b", 500))
	f.Add("a_b", "c_d")
	f.Add("a/b/c", "d/e/f")

	f.Fuzz(func(t *testing.T, provider, modelID string) {
		// Must never panic — that's the primary invariant.
		result := docID(provider, modelID)

		// Verify result equals PathEscape(provider) + "_" + PathEscape(modelID).
		expected := url.PathEscape(provider) + "_" + url.PathEscape(modelID)
		if result != expected {
			t.Errorf("docID(%q, %q) = %q, want %q", provider, modelID, result, expected)
		}

		// Result must not contain "/" — Firestore document IDs cannot have slashes.
		if strings.Contains(result, "/") {
			t.Errorf("docID(%q, %q) contains '/': %q", provider, modelID, result)
		}
	})
}
