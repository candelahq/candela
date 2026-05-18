# CACHING-01: Runtime caching configuration API
# Verifies: GET/POST /_local/api/config/caching, header stripping, mode
# propagation.
#
# Run:
#   hurl --variable CANDELA_URL=http://localhost:8080 test/functional/proxy/proxy_caching_config.hurl

# ── GET default caching config ──────────────────────────────────────────────
GET {{CANDELA_URL}}/_local/api/config

HTTP 200
[Asserts]
header "Content-Type" contains "application/json"
jsonpath "$.caching.anthropic" exists


# ── POST set caching mode: off ──────────────────────────────────────────────
POST {{CANDELA_URL}}/_local/api/config/caching
Content-Type: application/json
{
  "anthropic": "off"
}

HTTP 200
[Asserts]
jsonpath "$.caching.anthropic" == "off"


# ── POST set caching mode: auto ─────────────────────────────────────────────
POST {{CANDELA_URL}}/_local/api/config/caching
Content-Type: application/json
{
  "anthropic": "auto"
}

HTTP 200
[Asserts]
jsonpath "$.caching.anthropic" == "auto"


# ── POST set caching mode: system-only ──────────────────────────────────────
POST {{CANDELA_URL}}/_local/api/config/caching
Content-Type: application/json
{
  "anthropic": "system-only"
}

HTTP 200
[Asserts]
jsonpath "$.caching.anthropic" == "system-only"


# ── GET confirms mode persisted ─────────────────────────────────────────────
GET {{CANDELA_URL}}/_local/api/config

HTTP 200
[Asserts]
jsonpath "$.caching.anthropic" == "system-only"


# ── POST backward compat: "true" → auto ─────────────────────────────────────
POST {{CANDELA_URL}}/_local/api/config/caching
Content-Type: application/json
{
  "anthropic": "true"
}

HTTP 200
[Asserts]
jsonpath "$.caching.anthropic" == "auto"


# ── POST backward compat: "false" → off ──────────────────────────────────────
POST {{CANDELA_URL}}/_local/api/config/caching
Content-Type: application/json
{
  "anthropic": "false"
}

HTTP 200
[Asserts]
jsonpath "$.caching.anthropic" == "off"


# ── POST bad JSON returns 400 ────────────────────────────────────────────────
POST {{CANDELA_URL}}/_local/api/config/caching
Content-Type: application/json
not-valid-json

HTTP 400
[Asserts]
jsonpath "$.error" == "invalid JSON"


# ── CACHE-07: Default (auto) → cache_control injected ────────────────────────
# With no X-Candela-Caching header, the default mode (auto) should inject
# cache_control blocks into the translated Anthropic request.
POST {{CANDELA_URL}}/proxy/anthropic/v1/chat/completions
Authorization: Bearer test-gcp-token
Content-Type: application/json
{
  "model": "claude-sonnet-4-20250514",
  "messages": [
    {"role": "system", "content": "You are a helpful assistant."},
    {"role": "user", "content": "Hello"}
  ]
}

HTTP 200
[Asserts]
header "X-Mock-Cache-Control-Present" == "true"
header "X-Mock-Candela-Header-Leaked" == "false"


# ── CACHE-08: X-Candela-Caching: off → no cache_control injected ─────────────
# Per-request override should suppress cache_control even though default is auto.
POST {{CANDELA_URL}}/proxy/anthropic/v1/chat/completions
Authorization: Bearer test-gcp-token
Content-Type: application/json
X-Candela-Caching: off
{
  "model": "claude-sonnet-4-20250514",
  "messages": [
    {"role": "system", "content": "You are a helpful assistant."},
    {"role": "user", "content": "Hello"}
  ]
}

HTTP 200
[Asserts]
header "X-Mock-Cache-Control-Present" == "false"
header "X-Mock-Candela-Header-Leaked" == "false"


# ── CACHE-09: X-Candela-Caching: auto → cache_control injected ───────────────
# Explicit auto should behave the same as default.
POST {{CANDELA_URL}}/proxy/anthropic/v1/chat/completions
Authorization: Bearer test-gcp-token
Content-Type: application/json
X-Candela-Caching: auto
{
  "model": "claude-sonnet-4-20250514",
  "messages": [
    {"role": "system", "content": "You are a helpful assistant."},
    {"role": "user", "content": "Hello"}
  ]
}

HTTP 200
[Asserts]
header "X-Mock-Cache-Control-Present" == "true"
header "X-Mock-Candela-Header-Leaked" == "false"


# ── CACHE-10: X-Candela-Caching header never leaks to non-Anthropic providers ─
# Even for providers without a FormatTranslator, the header must be stripped.
POST {{CANDELA_URL}}/proxy/openai/v1/chat/completions
Authorization: Bearer sk-test-key
Content-Type: application/json
X-Candela-Caching: auto
{
  "model": "gpt-4o",
  "messages": [{"role": "user", "content": "Hello"}]
}

HTTP 200
[Asserts]
header "X-Mock-Candela-Header-Leaked" == "false"

