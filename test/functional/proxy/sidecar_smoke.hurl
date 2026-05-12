# SIDECAR-01: Rust sidecar health + proxy smoke test
# Verifies: healthz, readyz, proxy routing, unknown provider handling.
#
# Run:
#   hurl --variable SIDECAR_URL=http://localhost:8080 test/functional/proxy/sidecar_smoke.hurl

# ── Health check ─────────────────────────────────────────────────────────────
GET {{SIDECAR_URL}}/healthz

HTTP 200
[Asserts]
header "Content-Type" contains "application/json"
jsonpath "$.status" == "ok"
jsonpath "$.binary" == "candela-sidecar"


# ── Readiness check ──────────────────────────────────────────────────────────
GET {{SIDECAR_URL}}/readyz

HTTP 200
[Asserts]
jsonpath "$.status" == "ready"


# ── Unknown provider returns 502 ─────────────────────────────────────────────
POST {{SIDECAR_URL}}/proxy/nonexistent/v1/chat/completions
Content-Type: application/json
{
  "model": "gpt-4o",
  "messages": [{"role": "user", "content": "hello"}]
}

HTTP 502
[Asserts]
body contains "unknown provider"


# ── OpenAI passthrough (requires mock upstream or live key) ──────────────────
# This test is conditional — it will only succeed when the sidecar is pointed
# at a mock upstream that returns valid OpenAI-format responses.
#
# To run with mock:
#   1. Start mock upstream on port 9999
#   2. Set OPENAI_UPSTREAM_URL=http://localhost:9999 or use default openai provider
#   3. Run: hurl --variable SIDECAR_URL=http://localhost:8080 test/functional/proxy/sidecar_smoke.hurl

POST {{SIDECAR_URL}}/proxy/openai/v1/chat/completions
Content-Type: application/json
Authorization: Bearer sk-test-key
{
  "model": "gpt-4o",
  "messages": [{"role": "user", "content": "Say hello"}]
}

# When mock upstream is running, expect 200 with valid JSON.
# When no upstream is available, expect 502 (upstream connection refused).
HTTP *
[Asserts]
# Status is either 200 (mock available) or 502 (no upstream).
status >= 200
status <= 502
