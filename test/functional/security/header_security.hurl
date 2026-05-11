# SEC-01..05: Security — header injection, X-User-Id bypass, session ID validation
# These tests verify security hardening around request ID and session ID handling.

# ── SEC-01: X-Request-ID with injection chars → replaced ─────────────────────
# Characters like newlines, semicolons could enable log injection.
# The proxy must validate and replace any non-[a-zA-Z0-9-] request IDs.
POST {{CANDELA_URL}}/proxy/openai/v1/chat/completions
Authorization: Bearer sk-test
Content-Type: application/json
X-Request-ID: injected\nHeader: evil
{"model":"gpt-4o","messages":[{"role":"user","content":"injection test"}]}

HTTP 200
[Asserts]
# Response ID must be a valid generated hex ID, NOT the injected value.
header "X-Request-ID" matches "^[a-fA-F0-9]{32}$"


# ── SEC-02: X-Session-Id with injection chars → silently dropped ──────────────
POST {{CANDELA_URL}}/proxy/openai/v1/chat/completions
Authorization: Bearer sk-test
Content-Type: application/json
X-Session-Id: evil!!!session@id
{"model":"gpt-4o","messages":[{"role":"user","content":"session injection test"}]}

# Proxy must accept the request (not 400) — invalid session ID is just dropped.
HTTP 200


# ── SEC-03: Malformed Traceparent → no crash, request succeeds ───────────────
POST {{CANDELA_URL}}/proxy/openai/v1/chat/completions
Authorization: Bearer sk-test
Content-Type: application/json
Traceparent: this-is-not-a-valid-traceparent
{"model":"gpt-4o","messages":[{"role":"user","content":"bad traceparent test"}]}

HTTP 200
[Asserts]
# Must not crash. Request proceeds normally without trace correlation.
status == 200


# ── SEC-05: X-User-Id header is IGNORED — user_id from auth context only ──────
# A client cannot impersonate another user by sending X-User-Id.
# The proxy must read identity exclusively from the verified auth context (IAP/JWT).
#
# This test verifies the proxy doesn't 400 on the header (it should ignore it silently).
POST {{CANDELA_URL}}/proxy/openai/v1/chat/completions
Authorization: Bearer sk-test
Content-Type: application/json
X-User-Id: attacker@evil.com
{"model":"gpt-4o","messages":[{"role":"user","content":"spoof attempt"}]}

HTTP 200
# Span user_id must NOT be "attacker@evil.com".
# (Verified in Rust integration tests which can inspect the submitted span.)


# ── SEC-04: Missing Authorization forwarded as-is (proxy is auth-agnostic) ───
# The proxy does not require clients to have a bearer token — it forwards
# whatever auth the client sends. Upstream may reject it.
POST {{CANDELA_URL}}/proxy/openai/v1/chat/completions
Content-Type: application/json
{"model":"gpt-4o","messages":[{"role":"user","content":"no auth header"}]}

HTTP *
[Asserts]
# Must NOT be 500 — the proxy should forward without auth, not crash.
status != 500
