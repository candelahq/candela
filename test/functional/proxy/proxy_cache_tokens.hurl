# PROXY-07: Cache token passthrough — Anthropic cache fields visible in response
# Verifies:
#   - Anthropic cache_read_input_tokens and cache_creation_input_tokens
#     are present in the upstream response forwarded by the proxy.
#   - OpenAI prompt_tokens_details.cached_tokens is forwarded.
#   - Proxy does not strip provider-specific cache metadata.
#
# Note: The proxy also extracts these into span attributes
# (gen_ai.usage.cache_read_tokens, gen_ai.usage.cache_creation_tokens),
# but those are only visible in the trace detail API, not in the proxied response.

# ── Anthropic direct — cache tokens in response ──────────────────────────────
POST {{CANDELA_URL}}/proxy/anthropic-direct/v1/messages
x-api-key: sk-ant-test-key
Content-Type: application/json
{
  "model": "claude-sonnet-4-20250514",
  "messages": [{"role": "user", "content": "Cache test"}],
  "max_tokens": 1024
}

HTTP 200
[Asserts]
header "Content-Type" contains "application/json"
jsonpath "$.usage.input_tokens" isInteger
jsonpath "$.usage.output_tokens" isInteger
jsonpath "$.usage.cache_read_input_tokens" isInteger
jsonpath "$.usage.cache_read_input_tokens" >= 0
jsonpath "$.usage.cache_creation_input_tokens" isInteger
jsonpath "$.usage.cache_creation_input_tokens" >= 0


# ── OpenAI — cached tokens in prompt_tokens_details ──────────────────────────
POST {{CANDELA_URL}}/proxy/openai/v1/chat/completions
Authorization: Bearer sk-test-key
Content-Type: application/json
{
  "model": "gpt-4o",
  "messages": [{"role": "user", "content": "Cache test"}]
}

HTTP 200
[Asserts]
header "Content-Type" contains "application/json"
jsonpath "$.usage.prompt_tokens" isInteger
jsonpath "$.usage.completion_tokens" isInteger
jsonpath "$.usage.prompt_tokens_details.cached_tokens" isInteger
jsonpath "$.usage.prompt_tokens_details.cached_tokens" >= 0
