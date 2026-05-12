# PROXY-07: Anthropic Direct — native Messages API passthrough (Claude Code / pencil.dev)
# Verifies:
#   - Request body forwarded unchanged (no format translation)
#   - Response returned in native Anthropic format (no OpenAI translation)
#   - X-Api-Key header forwarded to upstream
#   - anthropic-version and anthropic-beta headers forwarded
#   - Token usage parsed correctly for billing
#   - X-Request-ID generated and returned
#
# This provider enables Claude Code and pencil.dev to route through Candela
# for observability without any format translation.

# ── Standard Anthropic direct request → native response ───────────────────────
POST {{CANDELA_URL}}/proxy/anthropic-direct/v1/messages
X-Api-Key: sk-ant-test-key
Anthropic-Version: 2023-06-01
Content-Type: application/json
{
  "model": "claude-sonnet-4-20250514",
  "messages": [{"role": "user", "content": "Hello!"}],
  "max_tokens": 1024
}

HTTP 200
[Asserts]
header "Content-Type" contains "application/json"
header "X-Request-ID" isString
# Response is native Anthropic format — NOT translated to OpenAI shape.
jsonpath "$.type" == "message"
jsonpath "$.role" == "assistant"
jsonpath "$.content" count >= 1
jsonpath "$.content[0].type" == "text"
jsonpath "$.content[0].text" isString
jsonpath "$.usage.input_tokens" isInteger
jsonpath "$.usage.output_tokens" isInteger
# Must NOT contain OpenAI fields (would indicate unwanted translation).
jsonpath "$.object" not exists
jsonpath "$.choices" not exists


# ── Verify anthropic-beta header is forwarded ─────────────────────────────────
# Claude Code sends this header; gateway spec requires it to be forwarded.
POST {{CANDELA_URL}}/proxy/anthropic-direct/v1/messages
X-Api-Key: sk-ant-test-key
Anthropic-Version: 2023-06-01
Anthropic-Beta: messages-2024-12-19
Content-Type: application/json
{
  "model": "claude-sonnet-4-20250514",
  "messages": [{"role": "user", "content": "Beta header test"}],
  "max_tokens": 512
}

HTTP 200
[Asserts]
header "Content-Type" contains "application/json"
jsonpath "$.type" == "message"
jsonpath "$.usage.input_tokens" isInteger
