# PROXY-09: Streaming with cache tokens — Anthropic SSE stream contains cache metadata
# Verifies:
#   - Anthropic streaming response includes cache tokens in message_start event
#   - Stream format is correct text/event-stream
#   - Proxy forwards cache-bearing SSE events without stripping them

# ── Anthropic streaming with cache tokens ────────────────────────────────────
POST {{CANDELA_URL}}/proxy/anthropic/v1/messages
Authorization: Bearer test-gcp-token
Content-Type: application/json
{
  "model": "claude-sonnet-4-20250514",
  "messages": [{"role": "user", "content": "Stream with cache"}],
  "stream": true
}

HTTP 200
[Asserts]
header "Content-Type" contains "text/event-stream"
# Stream should contain SSE data lines
body contains "data:"
# The message_start event should include cache token counts
body contains "cache_read_input_tokens"
# Stream must end with [DONE] (proxy translates Anthropic message_stop → OpenAI [DONE])
body contains "data: [DONE]"
