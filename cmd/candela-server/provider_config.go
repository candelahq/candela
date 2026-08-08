package main

// providerConfig defines the server-side configuration for providers that
// route through Vertex AI. This is the single source of truth consumed by
// main.go for PathRewriter setup, project-ID fallback filtering, and
// override validation. Tests validate against this to prevent providers
// from being added to proxy.DefaultProviders() without server wiring.

// maaSProviderRegion maps MaaS provider names to their default Vertex AI
// region. These providers use the OpenAI-compatible endpoint and share the
// same PathRewriter (VertexAIGeminiOAIPathRewriter).
var maaSProviderRegion = map[string]string{
	"deepseek":    "us-central1",
	"deepseek-v3": "global",
	"qwen":        "us-south1",
	"zai":         "global",
	"meta":        "us-east5",
	"xai":         "global",
}

// vertexAIProviders is the set of all provider names that require a Vertex AI
// project ID to function. Providers not in this set (e.g. "openai",
// "anthropic-direct") can operate without Vertex AI configuration.
//
// This includes:
//   - Gemini native/OAI-compat providers
//   - Anthropic via Vertex AI
//   - Mistral via Vertex AI rawPredict
//   - All MaaS providers (from maaSProviderRegion)
var vertexAIProviders = func() map[string]bool {
	m := map[string]bool{
		"gemini-oai":       true,
		"google":           true,
		"gemini-vertex":    true,
		"anthropic":        true,
		"anthropic-vertex": true,
		"mistral":          true,
	}
	for name := range maaSProviderRegion {
		m[name] = true
	}
	return m
}()

// nonVertexProviders is the set of providers that do NOT require Vertex AI
// project configuration. They route directly to external APIs.
var nonVertexProviders = map[string]bool{
	"openai":           true,
	"anthropic-direct": true,
}

// allKnownProviders returns the complete set of provider names that the
// server knows how to configure. Used by tests to verify parity with
// proxy.DefaultProviders().
func allKnownProviders() map[string]bool {
	m := make(map[string]bool, len(vertexAIProviders)+len(nonVertexProviders))
	for name := range vertexAIProviders {
		m[name] = true
	}
	for name := range nonVertexProviders {
		m[name] = true
	}
	return m
}
