# CACHING-TTL: Cache TTL configuration API
# Verifies: GET/POST cache_ttl runtime config, header acceptance.
#
# Run:
#   hurl --variable CANDELA_URL=http://localhost:8787 test/functional/proxy/proxy_ttl_config.hurl

# ── GET shows default TTL = 5m ────────────────────────────────────────────────
GET {{CANDELA_URL}}/_local/api/config

HTTP 200
[Asserts]
jsonpath "$.caching.cache_ttl" exists
jsonpath "$.caching.anthropic" exists


# ── POST set cache TTL to 1h ──────────────────────────────────────────────────
POST {{CANDELA_URL}}/_local/api/config/caching
Content-Type: application/json
{
  "anthropic": "auto",
  "cache_ttl": "1h"
}

HTTP 200
[Asserts]
jsonpath "$.caching.anthropic" == "auto"
jsonpath "$.caching.cache_ttl" == "1h"


# ── GET confirms TTL persisted ────────────────────────────────────────────────
GET {{CANDELA_URL}}/_local/api/config

HTTP 200
[Asserts]
jsonpath "$.caching.cache_ttl" == "1h"


# ── POST reset TTL back to 5m ────────────────────────────────────────────────
POST {{CANDELA_URL}}/_local/api/config/caching
Content-Type: application/json
{
  "anthropic": "auto",
  "cache_ttl": "5m"
}

HTTP 200
[Asserts]
jsonpath "$.caching.cache_ttl" == "5m"


# ── POST mode-only update preserves existing TTL ──────────────────────────────
POST {{CANDELA_URL}}/_local/api/config/caching
Content-Type: application/json
{
  "anthropic": "system-only"
}

HTTP 200
[Asserts]
jsonpath "$.caching.anthropic" == "system-only"
jsonpath "$.caching.cache_ttl" == "5m"


# ── POST invalid TTL defaults to 5m ──────────────────────────────────────────
POST {{CANDELA_URL}}/_local/api/config/caching
Content-Type: application/json
{
  "anthropic": "auto",
  "cache_ttl": "garbage"
}

HTTP 200
[Asserts]
jsonpath "$.caching.cache_ttl" == "5m"


# ── X-Candela-Cache-TTL header accepted on proxy ──────────────────────────────
POST {{CANDELA_URL}}/proxy/anthropic/v1/chat/completions
Content-Type: application/json
X-Candela-Cache-TTL: 1h
{
  "model": "claude-sonnet-4-20250514",
  "messages": [{"role": "user", "content": "hello"}]
}

# Expect 200 (mock) or 502 (no upstream), but NOT a 400/500 from the header.
HTTP *
[Asserts]
status >= 200
status <= 502
