# PROXY-03: SSE streaming — event-stream forwarding, [DONE] terminator
# Verifies:
#   - Content-Type is text/event-stream
#   - Response contains SSE data lines
#   - Stream ends with data: [DONE] (OpenAI compat) or message_stop (raw Anthropic)

# ── OpenAI streaming ──────────────────────────────────────────────────────────
POST {{CANDELA_URL}}/proxy/openai/v1/chat/completions
Authorization: Bearer sk-test-key
Content-Type: application/json
{
  "model": "gpt-4o",
  "messages": [{"role": "user", "content": "Stream this"}],
  "stream": true
}

HTTP 200
[Asserts]
header "Content-Type" contains "text/event-stream"
# Body should contain SSE data lines
body contains "data:"


# ── Anthropic streaming via Vertex AI ────────────────────────────────────────
POST {{CANDELA_URL}}/proxy/anthropic/v1/messages
Authorization: Bearer test-gcp-token
Content-Type: application/json
{
  "model": "claude-sonnet-4-20250514",
  "messages": [{"role": "user", "content": "Stream this"}],
  "stream": true
}

HTTP 200
[Asserts]
header "Content-Type" contains "text/event-stream"
body contains "data:"
# Stream must end with [DONE] (proxy translates Anthropic message_stop → OpenAI [DONE])
body contains "data: [DONE]"
