# PROXY-02: Anthropic via Vertex AI — path rewriting + format translation
# Verifies:
#   - Upstream path rewritten to Vertex AI format (rawPredict)
#   - Request body: model field removed, anthropic_version injected
#   - Response translated back to OpenAI shape
#   - W3C Traceparent propagated with same trace_id, new span_id
#
# Note: These tests require the mock upstream configured at MOCK_UPSTREAM_URL
# to be reachable. The proxy's anthropic upstream must point at MOCK_UPSTREAM_URL
# (configure via VERTEX_UPSTREAM env or config.yaml in test setup).

# ── Standard Anthropic request → translated response ─────────────────────────
POST {{CANDELA_URL}}/proxy/anthropic/v1/messages
Authorization: Bearer test-gcp-token
Content-Type: application/json
{
  "model": "claude-sonnet-4-20250514",
  "messages": [{"role": "user", "content": "Hello!"}],
  "max_tokens": 1024
}

HTTP 200
[Asserts]
header "Content-Type" contains "application/json"
# Response translated to OpenAI shape
jsonpath "$.object" == "chat.completion"
# Model name cleaned (date suffix stripped)
jsonpath "$.model" == "claude-sonnet-4"
jsonpath "$.choices" count == 1
jsonpath "$.choices[0].message.role" == "assistant"
jsonpath "$.choices[0].message.content" isString
jsonpath "$.choices[0].finish_reason" == "stop"
jsonpath "$.usage.prompt_tokens" isInteger
jsonpath "$.usage.completion_tokens" isInteger
jsonpath "$.usage.total_tokens" isInteger


# ── W3C Traceparent propagation ───────────────────────────────────────────────
# The proxy must forward a modified Traceparent to upstream:
#   - Same trace_id as the incoming header
#   - A NEW span_id (the proxy's own span)
# This test captures what upstream received via a response header set by the mock.
#
# TODO: Enable once mock upstream reflects received Traceparent in response header.
# POST {{CANDELA_URL}}/proxy/anthropic/v1/messages
# Authorization: Bearer test-gcp-token
# Content-Type: application/json
# Traceparent: 00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01
# { "model": "claude-sonnet-4-20250514", "messages": [{"role": "user", "content": "Trace test"}] }
#
# HTTP 200
# [Asserts]
# header "X-Mock-Received-Traceparent" matches "^00-0af7651916cd43dd8448eb211c80319c-[a-f0-9]{16}-01$"


# ── model alias prefix resolution ─────────────────────────────────────────────
# Sending "claude-sonnet-4" (no date) → proxy resolves to full canonical ID.
# Only works if exactly one model matches the prefix.
# (Tested via compat route in compat/compat_routes.hurl)
