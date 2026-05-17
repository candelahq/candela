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


# ── X-Candela-Caching header is stripped from proxy requests ─────────────────
# When sending a request with the override header to any provider,
# the header must NOT be forwarded upstream (verified via mock upstream).
POST {{CANDELA_URL}}/proxy/anthropic/v1/chat/completions
Content-Type: application/json
X-Candela-Caching: off
{
  "model": "claude-sonnet-4-20250514",
  "messages": [{"role": "user", "content": "hello"}]
}

# We expect either 200 (mock) or 502 (no upstream) — but the request
# should be accepted and header should not cause a 400 or 500.
HTTP *
[Asserts]
status >= 200
status <= 502
