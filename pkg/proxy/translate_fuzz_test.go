package proxy

import (
	"strings"
	"testing"
)

// FuzzVertexAIPathRewriter exercises VertexAIPathRewriter.RewritePath with
// arbitrary model names. It must never panic regardless of input.
func FuzzVertexAIPathRewriter(f *testing.F) {
	// Seed corpus: normal models, path traversal, unicode, edge cases.
	f.Add("claude-sonnet-4-20250514", true)
	f.Add("claude-sonnet-4-20250514", false)
	f.Add("claude-3-5-sonnet-20241022", true)
	f.Add("claude-opus-4-20250514", false)
	f.Add("../../etc/passwd", true)
	f.Add("model/../../../secret", false)
	f.Add("模型-test", true)
	f.Add("publisher/model", false)
	f.Add("", true)
	f.Add(strings.Repeat("a", 500), false)
	f.Add("model%00name", true)
	f.Add("model\nname", false)
	f.Add("model\x00name", true)
	f.Add("../../../etc/shadow", false)
	f.Add("a]b[c{d}e", true)

	rewriter := &VertexAIPathRewriter{
		ProjectID: "test-project",
		Region:    "us-central1",
	}

	f.Fuzz(func(t *testing.T, model string, streaming bool) {
		// Must never panic — that's the primary invariant.
		result := rewriter.RewritePath(model, streaming)

		// Result must always start with the expected Vertex AI prefix.
		if !strings.HasPrefix(result, "/v1/projects/") {
			t.Errorf("result does not start with /v1/projects/: %q", result)
		}

		// Path traversal check: if the input itself contains "..",
		// the output will too (no sanitization). Only flag when the
		// rewriter *introduces* ".." that wasn't in the input.
		if strings.Contains(result, "..") && !strings.Contains(model, "..") {
			t.Errorf("result contains '..' not present in input: %q", result)
		}
	})
}

// FuzzVertexAIMaaSPathRewriter exercises VertexAIMaaSPathRewriter.RewritePath
// with arbitrary model names.
func FuzzVertexAIMaaSPathRewriter(f *testing.F) {
	f.Add("mistral-large-2411", true)
	f.Add("mistral-large-2411", false)
	f.Add("codestral-2501", true)
	f.Add("../../etc/passwd", true)
	f.Add("model/../../../secret", false)
	f.Add("模型-test", true)
	f.Add("publisher/model", false)
	f.Add("", true)
	f.Add("", false)
	f.Add(strings.Repeat("a", 500), false)
	f.Add("model%00name", true)
	f.Add("model\nname", false)
	f.Add("model\x00name", true)

	rewriter := &VertexAIMaaSPathRewriter{
		ProjectID: "test-project",
		Region:    "us-east1",
		Publisher: "mistralai",
	}

	f.Fuzz(func(t *testing.T, model string, streaming bool) {
		// Must never panic — that's the primary invariant.
		result := rewriter.RewritePath(model, streaming)

		// Empty model returns empty string per implementation.
		if model == "" {
			return
		}

		// Path traversal check: only flag when the rewriter introduces ".."
		// that wasn't in the input model name.
		if strings.Contains(result, "..") && !strings.Contains(model, "..") {
			t.Errorf("result contains '..' not present in input: %q", result)
		}
	})
}

// FuzzVertexAIGeminiOAIPathRewriter exercises VertexAIGeminiOAIPathRewriter.RewritePath
// with arbitrary model names.
func FuzzVertexAIGeminiOAIPathRewriter(f *testing.F) {
	f.Add("gemini-2.5-pro", true)
	f.Add("gemini-2.5-pro", false)
	f.Add("gemini-2.5-flash", true)
	f.Add("../../etc/passwd", false)
	f.Add("模型-test", true)
	f.Add("", true)
	f.Add("", false)
	f.Add(strings.Repeat("a", 500), false)
	f.Add("model%00name", true)
	f.Add("model\nname", false)

	rewriter := &VertexAIGeminiOAIPathRewriter{
		ProjectID: "test-project",
		Region:    "us-central1",
	}

	f.Fuzz(func(t *testing.T, model string, streaming bool) {
		// Must never panic — that's the primary invariant.
		result := rewriter.RewritePath(model, streaming)

		// Result must always contain the OpenAI-compat endpoint path.
		if !strings.Contains(result, "/openapi/chat/completions") {
			t.Errorf("result does not contain /openapi/chat/completions: %q", result)
		}
	})
}

// FuzzVertexAIGooglePathRewriter exercises VertexAIGooglePathRewriter.RewritePath
// with arbitrary model names.
func FuzzVertexAIGooglePathRewriter(f *testing.F) {
	f.Add("gemini-2.5-pro", true)
	f.Add("gemini-2.5-pro", false)
	f.Add("gemini-2.5-flash", true)
	f.Add("../../etc/passwd", false)
	f.Add("model/../../../secret", true)
	f.Add("模型-test", true)
	f.Add("publisher/model", false)
	f.Add("", true)
	f.Add("", false)
	f.Add(strings.Repeat("a", 500), false)
	f.Add("model%00name", true)
	f.Add("model\nname", false)
	f.Add("model\x00name", true)

	rewriter := &VertexAIGooglePathRewriter{
		ProjectID: "test-project",
		Region:    "us-central1",
	}

	f.Fuzz(func(t *testing.T, model string, streaming bool) {
		// Must never panic — that's the primary invariant.
		result := rewriter.RewritePath(model, streaming)

		// Path traversal check: only flag when the rewriter introduces ".."
		// that wasn't in the input model name.
		if strings.Contains(result, "..") && !strings.Contains(model, "..") {
			t.Errorf("result contains '..' not present in input: %q", result)
		}
	})
}

// FuzzParseModelName exercises ParseModelName with arbitrary inputs.
func FuzzParseModelName(f *testing.F) {
	f.Add("claude-sonnet-4-20250514")
	f.Add("claude-3-5-sonnet-20241022")
	f.Add("claude-opus-4-20250514")
	f.Add("gemini-2.5-pro")
	f.Add("../../etc/passwd")
	f.Add("模型-test")
	f.Add("")
	f.Add(strings.Repeat("a", 500))
	f.Add("model-00000000")
	f.Add("model-99999999")
	f.Add("model%00name")
	f.Add("model\nname")
	f.Add("-20250514")
	f.Add("model-2025051")   // 7 digits — not a date
	f.Add("model-202505140") // 9 digits — not a date

	f.Fuzz(func(t *testing.T, input string) {
		// Must never panic — that's the primary invariant.
		result := ParseModelName(input)

		// Raw must always equal the input.
		if result.Raw != input {
			t.Errorf("Raw = %q, want %q", result.Raw, input)
		}
	})
}
