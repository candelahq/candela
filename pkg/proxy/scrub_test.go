package proxy

import (
	"strings"
	"testing"
)

func TestScrubContent(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		contains string
		excludes []string
	}{
		{
			name:     "empty string",
			input:    "",
			contains: "",
		},
		{
			name:     "clean content unchanged",
			input:    "Hello world, what is the capital of France?",
			contains: "Hello world, what is the capital of France?",
		},
		{
			name:     "redacts OpenAI API key",
			input:    "Use key sk-1234567890abcdef1234567890abcdef to authenticate",
			contains: "[REDACTED_API_KEY]",
			excludes: []string{"sk-1234567890abcdef1234567890abcdef"},
		},
		{
			name:     "redacts Anthropic API key",
			input:    "Key is sk-ant-api03-abcdef1234567890abcdef12345-AA",
			contains: "[REDACTED_API_KEY]",
			excludes: []string{"sk-ant-api03-abcdef1234567890abcdef12345-AA"},
		},
		{
			name:     "redacts Google API key",
			input:    "Key is AIzaSyD-xxxxxxxxxxxxxxxxxxxxxxxxxxxxxx here",
			contains: "[REDACTED_API_KEY]",
			excludes: []string{"AIzaSyD-xxxxxxxxxxxxxxxxxxxxxxxxxxxxxx"},
		},
		{
			name:     "redacts AWS Access Key",
			input:    "AWS access key: AKIAIOSFODNN7EXAMPLE",
			contains: "[REDACTED_API_KEY]",
			excludes: []string{"AKIAIOSFODNN7EXAMPLE"},
		},
		{
			name:     "redacts Bearer token",
			input:    "Authorization: Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.xyz",
			contains: "Bearer [REDACTED_TOKEN]",
			excludes: []string{"eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.xyz"},
		},
		{
			name:     "redacts SSN",
			input:    "Customer SSN is 123-45-6789 in report",
			contains: "[REDACTED_SSN]",
			excludes: []string{"123-45-6789"},
		},
		{
			name:     "redacts email address",
			input:    "Contact alice.smith+work@example.com for support",
			contains: "[REDACTED_EMAIL]",
			excludes: []string{"alice.smith+work@example.com"},
		},
		{
			name: "redacts PEM private key",
			input: "Here is the private key:\n" +
				"-----" + "BEGIN RSA PRIVATE " + "KEY-----\n" +
				"MIIEowIBAAKCAQEA0Y1+abcdef\n" +
				"-----" + "END RSA PRIVATE " + "KEY-----\n" +
				"Done.",
			contains: "[REDACTED_PRIVATE_KEY]",
			excludes: []string{"MIIEowIBAAKCAQEA0Y1+abcdef"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ScrubContent(tt.input)
			if tt.contains != "" && !strings.Contains(got, tt.contains) {
				t.Errorf("ScrubContent() = %q, want to contain %q", got, tt.contains)
			}
			for _, excl := range tt.excludes {
				if strings.Contains(got, excl) {
					t.Errorf("ScrubContent() = %q still contains sensitive string %q", got, excl)
				}
			}
		})
	}
}

func TestTruncateContent(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		maxLen int
		want   string
	}{
		{
			name:   "short content below limit",
			input:  "hello",
			maxLen: 10,
			want:   "hello",
		},
		{
			name:   "exact limit",
			input:  "hello",
			maxLen: 5,
			want:   "hello",
		},
		{
			name:   "exceeds limit",
			input:  "hello world",
			maxLen: 5,
			want:   "hello... [truncated]",
		},
		{
			name:   "zero maxLen means no truncation",
			input:  "hello world",
			maxLen: 0,
			want:   "hello world",
		},
		{
			name:   "negative maxLen means no truncation",
			input:  "hello world",
			maxLen: -1,
			want:   "hello world",
		},
		{
			name:   "unicode character boundary preserved",
			input:  "こんにちは世界",
			maxLen: 5,
			want:   "こんにちは... [truncated]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := TruncateContent(tt.input, tt.maxLen); got != tt.want {
				t.Errorf("TruncateContent(%q, %d) = %q, want %q", tt.input, tt.maxLen, got, tt.want)
			}
		})
	}
}

func TestSanitizeSpanContent(t *testing.T) {
	raw := "User john.doe@example.com used API key sk-1234567890abcdef1234567890abcdef to ask a very long question with extra words"
	sanitized := SanitizeSpanContent(raw, 50)

	if strings.Contains(sanitized, "john.doe@example.com") {
		t.Errorf("sanitized content contains unredacted email")
	}
	if strings.Contains(sanitized, "sk-1234567890abcdef1234567890abcdef") {
		t.Errorf("sanitized content contains unredacted API key")
	}
	if !strings.HasSuffix(sanitized, "... [truncated]") {
		t.Errorf("sanitized content should be truncated, got %q", sanitized)
	}
}
