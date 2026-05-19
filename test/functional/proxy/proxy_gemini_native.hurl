# GEMINI-NATIVE-01: Gemini native generateContent route
# Verifies:
#   - Native Google API response format (candidates, usageMetadata)
#   - Token counting (promptTokenCount, candidatesTokenCount, thoughtsTokenCount)
#   - Implicit cache tokens (cachedContentTokenCount) surfaced in response
#   - X-Request-ID generated and returned
#   - X-Candela-Caching header never leaks to upstream
#   - modelVersion echoed from upstream
#
# Requires mock upstream running:
#   go run test/functional/mock/upstream.go
#
# Run:
#   hurl --variable CANDELA_URL=http://localhost:8080 test/functional/proxy/proxy_gemini_native.hurl

# ── Standard Gemini native request → Google format response ───────────────────
POST {{CANDELA_URL}}/proxy/google/v1beta/models/gemini-2.5-flash:generateContent
Content-Type: application/json
{
  "contents": [
    {"role": "user", "parts": [{"text": "Hello!"}]}
  ],
  "generationConfig": {"temperature": 0.7}
}

HTTP 200
[Asserts]
header "Content-Type" contains "application/json"
header "X-Request-ID" isString
# Response is native Google format — NOT OpenAI shape.
jsonpath "$.candidates" count >= 1
jsonpath "$.candidates[0].content.parts[0].text" isString
jsonpath "$.candidates[0].content.role" == "model"
# Usage metadata present.
jsonpath "$.usageMetadata.promptTokenCount" isInteger
jsonpath "$.usageMetadata.candidatesTokenCount" isInteger
jsonpath "$.usageMetadata.totalTokenCount" isInteger
# Implicit caching — cachedContentTokenCount present.
jsonpath "$.usageMetadata.cachedContentTokenCount" isInteger
# Thinking tokens present (Gemini 2.5+).
jsonpath "$.usageMetadata.thoughtsTokenCount" isInteger
# Model version echoed.
jsonpath "$.modelVersion" isString
# Must NOT contain OpenAI fields.
jsonpath "$.choices" not exists
jsonpath "$.object" not exists
jsonpath "$.usage" not exists


# ── X-Candela-Caching header never leaks to Gemini upstream ──────────────────
POST {{CANDELA_URL}}/proxy/google/v1beta/models/gemini-2.5-flash:generateContent
Content-Type: application/json
X-Candela-Caching: auto
{
  "contents": [
    {"role": "user", "parts": [{"text": "Header leak test"}]}
  ]
}

HTTP 200
[Asserts]
header "X-Mock-Candela-Header-Leaked" == "false"


# ── Streaming endpoint (streamGenerateContent) ───────────────────────────────
POST {{CANDELA_URL}}/proxy/google/v1beta/models/gemini-2.5-pro:streamGenerateContent
Content-Type: application/json
{
  "contents": [
    {"role": "user", "parts": [{"text": "Stream test"}]}
  ]
}

# Mock returns JSON array for streaming. Expect 200 with content.
HTTP 200
[Asserts]
header "Content-Type" contains "application/json"


# ── Gemini 3.x model routing ─────────────────────────────────────────────────
# Verify that Gemini 3.x models route through the same handler.
POST {{CANDELA_URL}}/proxy/google/v1beta/models/gemini-3.1-flash:generateContent
Content-Type: application/json
{
  "contents": [
    {"role": "user", "parts": [{"text": "Gemini 3.1 test"}]}
  ]
}

HTTP 200
[Asserts]
jsonpath "$.candidates" count >= 1
jsonpath "$.modelVersion" == "gemini-3.1-flash"
jsonpath "$.usageMetadata.promptTokenCount" isInteger
