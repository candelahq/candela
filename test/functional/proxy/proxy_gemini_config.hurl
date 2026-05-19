# GEMINI-CONFIG-01: Gemini section in caching config API
# Verifies: GET/POST /_local/api/config returns Gemini caching status,
# and POST accepts gemini_cache_discount override.
#
# Run:
#   hurl --variable CANDELA_URL=http://localhost:8080 test/functional/proxy/proxy_gemini_config.hurl

# ── GET config returns Gemini section ────────────────────────────────────────
GET {{CANDELA_URL}}/_local/api/config

HTTP 200
[Asserts]
header "Content-Type" contains "application/json"
# Anthropic section still present.
jsonpath "$.caching.anthropic" exists
jsonpath "$.caching.cache_ttl" exists
# Gemini section is always present.
jsonpath "$.caching.gemini" exists
jsonpath "$.caching.gemini.mode" == "implicit"
jsonpath "$.caching.gemini.cache_discount" isString
jsonpath "$.caching.gemini.info" isString


# ── POST Anthropic-only update preserves Gemini section ──────────────────────
POST {{CANDELA_URL}}/_local/api/config/caching
Content-Type: application/json
{
  "anthropic": "auto"
}

HTTP 200
[Asserts]
jsonpath "$.caching.anthropic" == "auto"
# Gemini section must still be present after Anthropic-only update.
jsonpath "$.caching.gemini.mode" == "implicit"
jsonpath "$.caching.gemini.cache_discount" isString


# ── POST system-only + verify Gemini unaffected ──────────────────────────────
POST {{CANDELA_URL}}/_local/api/config/caching
Content-Type: application/json
{
  "anthropic": "system-only"
}

HTTP 200
[Asserts]
jsonpath "$.caching.anthropic" == "system-only"
jsonpath "$.caching.gemini.mode" == "implicit"


# ── POST gemini_cache_discount override ──────────────────────────────────────
# Enterprise customers with negotiated pricing can override the cache discount.
# This only takes effect if the server has a calc instance wired in.
POST {{CANDELA_URL}}/_local/api/config/caching
Content-Type: application/json
{
  "gemini_cache_discount": 0.15
}

# Expect 200 (accepted) — the discount is applied if calc is available.
# Gemini section remains implicit regardless of discount override.
HTTP 200
[Asserts]
jsonpath "$.caching.gemini.mode" == "implicit"


# ── POST bad JSON returns 400 ────────────────────────────────────────────────
POST {{CANDELA_URL}}/_local/api/config/caching
Content-Type: application/json
not-valid-json

HTTP 400
[Asserts]
jsonpath "$.error" == "invalid JSON"


# ── Reset to defaults ────────────────────────────────────────────────────────
POST {{CANDELA_URL}}/_local/api/config/caching
Content-Type: application/json
{
  "anthropic": "off",
  "cache_ttl": "5m"
}

HTTP 200
[Asserts]
jsonpath "$.caching.anthropic" == "off"
jsonpath "$.caching.cache_ttl" == "5m"
jsonpath "$.caching.gemini.mode" == "implicit"
