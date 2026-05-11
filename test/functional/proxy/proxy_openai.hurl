# PROXY-01: OpenAI chat completions round-trip
# Verifies: forwarding, response passthrough, X-Request-ID, span emission.
#
# Run:
#   hurl --variable CANDELA_URL=http://localhost:8080 test/functional/proxy/proxy_openai.hurl

# ── Basic round-trip ─────────────────────────────────────────────────────────
POST {{CANDELA_URL}}/proxy/openai/v1/chat/completions
Authorization: Bearer sk-test-key
Content-Type: application/json
{
  "model": "gpt-4o",
  "messages": [{"role": "user", "content": "Hello, world!"}]
}

HTTP 200
[Asserts]
header "Content-Type" contains "application/json"
header "X-Request-ID" exists
header "X-Request-ID" matches "^[a-fA-F0-9]{32}$"
jsonpath "$.object" == "chat.completion"
jsonpath "$.model" == "gpt-4o"
jsonpath "$.choices" count == 1
jsonpath "$.choices[0].message.role" == "assistant"
jsonpath "$.choices[0].message.content" isString
jsonpath "$.usage.prompt_tokens" isInteger
jsonpath "$.usage.completion_tokens" isInteger


# ── Client-provided X-Request-ID is echoed back ──────────────────────────────
POST {{CANDELA_URL}}/proxy/openai/v1/chat/completions
Authorization: Bearer sk-test-key
Content-Type: application/json
X-Request-ID: my-custom-request-id-1234
{
  "model": "gpt-4o",
  "messages": [{"role": "user", "content": "Echo my request ID"}]
}

HTTP 200
[Asserts]
header "X-Request-ID" == "my-custom-request-id-1234"


# ── Invalid X-Request-ID (contains spaces) → replaced with generated ID ──────
POST {{CANDELA_URL}}/proxy/openai/v1/chat/completions
Authorization: Bearer sk-test-key
Content-Type: application/json
X-Request-ID: this has spaces and is invalid!!
{
  "model": "gpt-4o",
  "messages": [{"role": "user", "content": "Bad request ID"}]
}

HTTP 200
[Asserts]
# Should be replaced with a valid 32-char hex generated ID (not the invalid one).
header "X-Request-ID" matches "^[a-fA-F0-9]{32}$"


# ── GET /proxy/openai/v1/models ───────────────────────────────────────────────
# Returns only models registered for this provider (may be empty if none configured).
GET {{CANDELA_URL}}/proxy/openai/v1/models
Authorization: Bearer sk-test-key

HTTP 200
[Asserts]
header "Content-Type" contains "application/json"
jsonpath "$.object" == "list"
jsonpath "$.data" isCollection
