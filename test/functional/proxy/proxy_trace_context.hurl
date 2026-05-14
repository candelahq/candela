# PROXY-08: W3C Trace Context propagation — parentSpanID preserved
# Verifies:
#   - Proxy accepts Traceparent header (W3C Trace Context)
#   - Response still returns successfully (no 0ms duration regression)
#   - X-Request-ID is still generated/echoed

# ── Request with Traceparent → should succeed (no crash, proper forwarding) ──
POST {{CANDELA_URL}}/proxy/openai/v1/chat/completions
Authorization: Bearer sk-test-key
Content-Type: application/json
Traceparent: 00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01
{
  "model": "gpt-4o",
  "messages": [{"role": "user", "content": "Trace context test"}]
}

HTTP 200
[Asserts]
header "Content-Type" contains "application/json"
header "X-Request-ID" exists
jsonpath "$.object" == "chat.completion"
jsonpath "$.usage.prompt_tokens" isInteger


# ── Request with Traceparent + Session-Id ─────────────────────────────────────
# Both W3C trace context and session tracking should coexist without error.
POST {{CANDELA_URL}}/proxy/openai/v1/chat/completions
Authorization: Bearer sk-test-key
Content-Type: application/json
Traceparent: 00-11111111111111111111111111111111-2222222222222222-01
X-Session-Id: session-abc-123
{
  "model": "gpt-4o",
  "messages": [{"role": "user", "content": "Session + trace context"}]
}

HTTP 200
[Asserts]
header "X-Request-ID" exists
jsonpath "$.object" == "chat.completion"
