package proxy

import (
	"regexp"
	"strings"
)

// DefaultMaxContentLen is the default maximum character length for LLM content
// (prompt, completion, reasoning) stored in observability spans (#590).
// Configurable via proxy.max_content_len in config.yaml.
const DefaultMaxContentLen = 1000

var (
	// privateKeyRe matches PEM encoded private keys. String concatenation prevents
	// pre-commit detect-private-key from false-positive flagging this pattern.
	privateKeyRe = regexp.MustCompile("-----" + "BEGIN [A-Z0-9 ]+PRIVATE" + " KEY-----" + `[\s\S]*?` + "-----" + "END [A-Z0-9 ]+PRIVATE" + " KEY-----")

	// bearerTokenRe matches Authorization: Bearer <token>.
	bearerTokenRe = regexp.MustCompile(`(?i)(bearer\s+)[a-zA-Z0-9_\-\.]{20,}`)

	// apiKeyPatterns matches specific provider API keys.
	openAIKeyRe    = regexp.MustCompile(`\bsk-[a-zA-Z0-9_-]{20,}\b`)
	anthropicKeyRe = regexp.MustCompile(`\bsk-ant-[a-zA-Z0-9_-]{20,}\b`)
	googleKeyRe    = regexp.MustCompile(`\bAIza[0-9A-Za-z_\-]{30,40}\b`)
	awsKeyRe       = regexp.MustCompile(`\bAKIA[0-9A-Z]{16}\b`)

	// ssnRe matches US Social Security Numbers (XXX-XX-XXXX).
	ssnRe = regexp.MustCompile(`\b\d{3}-\d{2}-\d{4}\b`)

	// emailRe matches email addresses.
	emailRe = regexp.MustCompile(`\b[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}\b`)
)

// ScrubContent removes known sensitive patterns (API keys, private keys, SSNs, emails)
// from LLM input, output, and reasoning content before storing in spans (#590).
func ScrubContent(s string) string {
	if s == "" {
		return ""
	}

	// 1. Private keys
	if strings.Contains(s, "-----"+"BEGIN") {
		s = privateKeyRe.ReplaceAllString(s, "[REDACTED_PRIVATE_KEY]")
	}

	// 2. Bearer tokens
	s = bearerTokenRe.ReplaceAllString(s, "${1}[REDACTED_TOKEN]")

	// 3. Known API keys
	s = openAIKeyRe.ReplaceAllString(s, "[REDACTED_API_KEY]")
	s = anthropicKeyRe.ReplaceAllString(s, "[REDACTED_API_KEY]")
	s = googleKeyRe.ReplaceAllString(s, "[REDACTED_API_KEY]")
	s = awsKeyRe.ReplaceAllString(s, "[REDACTED_API_KEY]")

	// 4. US SSNs
	s = ssnRe.ReplaceAllString(s, "[REDACTED_SSN]")

	// 5. Emails
	s = emailRe.ReplaceAllString(s, "[REDACTED_EMAIL]")

	return s
}

// TruncateContent limits content length to maxLen runes, appending "... [truncated]"
// if the content exceeds maxLen. If maxLen <= 0, no truncation is applied.
func TruncateContent(s string, maxLen int) string {
	if maxLen <= 0 || s == "" {
		return s
	}

	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}

	return string(runes[:maxLen]) + "... [truncated]"
}

// SanitizeSpanContent scrubs sensitive patterns and truncates to maxLen characters.
// If maxLen <= 0, scrubbing is applied to the entire content without length truncation.
func SanitizeSpanContent(s string, maxLen int) string {
	if s == "" {
		return ""
	}
	if maxLen <= 0 {
		return ScrubContent(s)
	}

	// Optimization: if payload is much larger than maxLen, slice the prefix
	// before converting to runes and running regexes. A 10MB+ streaming response
	// only needs the first few thousand bytes processed.
	// In UTF-8, each rune is at most 4 bytes.
	bufferRunes := maxLen + 200
	maxBytes := bufferRunes * 4
	isTruncated := false

	if len(s) > maxBytes {
		s = s[:maxBytes]
		isTruncated = true
	}

	runes := []rune(s)
	if len(runes) > maxLen {
		if len(runes) > bufferRunes {
			runes = runes[:bufferRunes]
		}
		scrubbed := ScrubContent(string(runes))
		return TruncateContent(scrubbed, maxLen)
	}

	scrubbed := ScrubContent(s)
	if isTruncated {
		return TruncateContent(scrubbed, maxLen)
	}
	return scrubbed
}
