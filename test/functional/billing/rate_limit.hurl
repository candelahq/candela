# BILLING-05: Rate limiting — 429 + Retry-After header
#
# Rate limiting is per-user, per-minute. The default limit is 100 req/min.
# These tests document the expected HTTP contract.
#
# Full rate-limit testing requires a stateful mock UserStore that can count
# requests — covered in Rust integration tests.

# ── Rate limit response shape documentation ───────────────────────────────────
# When rate limit is exceeded, the proxy returns:
#
# HTTP 429
# Retry-After: 60
# Content-Type: application/json
# {
#   "error": {
#     "message": "rate limit exceeded (101/100 requests/min)",
#     "type": "rate_limit_exceeded",
#     "code": 429
#   }
# }
#
# Verified in Rust integration tests (proxy/proxy_test.go TestRateLimit_Exceeded).


# ── Normal request (below limit) is not rate-limited ─────────────────────────
POST {{CANDELA_URL}}/proxy/openai/v1/chat/completions
Authorization: Bearer sk-test
Content-Type: application/json
{"model":"gpt-4o","messages":[{"role":"user","content":"single request"}]}

HTTP *
[Asserts]
# A single request must never be rate-limited.
status != 429
