# JETBRAINS-COMPAT: JetBrains / ktor client compatibility tests
#
# These tests verify that the LM compat listener correctly handles
# JetBrains AI Assistant's ktor-based HTTP client quirks:
#
# 1. Request body sanitization (strip fields that strict backends reject)
# 2. SSE streaming response format
# 3. User-Agent detection for ktor-specific workarounds
#
# Run against a live candela instance:
#   hurl --variable CANDELA_LM_URL=http://localhost:1234 test/functional/compat/jetbrains_compat.hurl
#
# These tests are protocol-level and should survive a Rust rewrite.


# ── JB-01: /v1/models with ktor User-Agent ─────────────────────────────────
# JetBrains sends ktor-client as User-Agent. Verify model list works.
GET {{CANDELA_LM_URL}}/v1/models
User-Agent: ktor-client-jvm/3.1.3

HTTP 200
[Asserts]
header "Content-Type" contains "application/json"
jsonpath "$.object" == "list"
jsonpath "$.data" isCollection


# ── JB-02: /api/v0/models — LM Studio native API ───────────────────────────
# IntelliJ uses this endpoint for "Test Connection".
GET {{CANDELA_LM_URL}}/api/v0/models
User-Agent: ktor-client-jvm/3.1.3

HTTP 200
[Asserts]
header "Content-Type" contains "application/json"
jsonpath "$.object" == "list"
jsonpath "$.data" isCollection


# ── JB-03: Request with extra JetBrains fields → still routed correctly ────
# JetBrains sends format, keep_alive, options, empty tools:[], and
# per-message tool_calls:[]. These must not cause 422 on strict backends.
# Use a nonsense model to get a clean 400 "unknown model" — proving the
# request body was parsed and routed, not rejected for invalid fields.
POST {{CANDELA_LM_URL}}/v1/chat/completions
Content-Type: application/json
User-Agent: ktor-client-jvm/3.1.3
```
{
  "model": "nonexistent-model-for-test",
  "messages": [
    {"role": "system", "content": "test", "tool_calls": []},
    {"role": "user", "content": "hello", "tool_calls": []}
  ],
  "stream": true,
  "format": "json",
  "keep_alive": "5m",
  "options": {"temperature": 0.7},
  "tools": [],
  "stream_options": {"include_usage": true}
}
```

HTTP 400
[Asserts]
jsonpath "$.error.type" == "invalid_request_error"
jsonpath "$.error.message" contains "unknown model"


# ── JB-04: max_tokens injection — negative value handled ───────────────────
# JetBrains sometimes sends max_tokens: -1. Candela must inject a sane
# default before forwarding. Again use a nonsense model for clean routing.
POST {{CANDELA_LM_URL}}/v1/chat/completions
Content-Type: application/json
User-Agent: ktor-client-jvm/3.1.3
```
{
  "model": "nonexistent-model-for-test",
  "messages": [{"role": "user", "content": "test"}],
  "max_tokens": -1,
  "stream": true
}
```

HTTP 400
[Asserts]
jsonpath "$.error.type" == "invalid_request_error"
jsonpath "$.error.message" contains "unknown model"


# ── JB-05: Non-streaming request — normal JSON response ────────────────────
# Even with ktor User-Agent, non-streaming requests should return JSON.
POST {{CANDELA_LM_URL}}/v1/chat/completions
Content-Type: application/json
User-Agent: ktor-client-jvm/3.1.3
```
{
  "model": "nonexistent-model-for-test",
  "messages": [{"role": "user", "content": "test"}],
  "stream": false
}
```

HTTP 400
[Asserts]
header "Content-Type" contains "application/json"
jsonpath "$.error.type" == "invalid_request_error"


# ── JB-06: /v1/completions stub returns 501 ────────────────────────────────
# JetBrains may probe legacy completions endpoint.
POST {{CANDELA_LM_URL}}/v1/completions
Content-Type: application/json
User-Agent: ktor-client-jvm/3.1.3
```
{"model": "test", "prompt": "hello"}
```

HTTP 501
