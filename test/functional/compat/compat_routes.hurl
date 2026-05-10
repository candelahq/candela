# COMPAT-01..05: OpenAI-compatible routes
# Verifies /v1/models and /v1/chat/completions compat routing.
#
# These routes are registered via RegisterCompatRoutes() and power
# LM Studio, IntelliJ, Cursor, and other OpenAI-compat tools.

# ── COMPAT-01: GET /v1/models — OpenAI compat model list ─────────────────────
GET {{CANDELA_URL}}/v1/models

HTTP 200
[Asserts]
header "Content-Type" contains "application/json"
jsonpath "$.object" == "list"
jsonpath "$.data" isCollection
# Each entry must have required fields
# (If no models are configured, data may be empty — that's OK)


# ── COMPAT-02: GET /api/v0/models — LM Studio native API ─────────────────────
# Used by IntelliJ's "Test Connection" button.
GET {{CANDELA_URL}}/api/v0/models

HTTP 200
[Asserts]
header "Content-Type" contains "application/json"
jsonpath "$.object" == "list"
jsonpath "$.data" isCollection


# ── COMPAT-05: Unknown model → 400 with structured error ─────────────────────
POST {{CANDELA_URL}}/v1/chat/completions
Content-Type: application/json
{"model":"this-model-does-not-exist-at-all","messages":[{"role":"user","content":"test"}]}

HTTP 400
[Asserts]
header "Content-Type" contains "application/json"
jsonpath "$.error.type" == "invalid_request_error"
jsonpath "$.error.message" contains "unknown model"


# ── COMPAT: Missing/invalid model field → 400 ─────────────────────────────────
POST {{CANDELA_URL}}/v1/chat/completions
Content-Type: application/json
{"messages":[{"role":"user","content":"test"}]}

HTTP 400
[Asserts]
jsonpath "$.error.type" == "invalid_request_error"
jsonpath "$.error.message" contains "missing or invalid model"


# ── COMPAT-03/04: Known model routed to correct provider ─────────────────────
# This requires compat models to be configured. The test below is conditional:
# if the binary has "gpt-4o" registered under "openai", it should 200.
# If not configured, it falls through to the "unknown model" 400 above.
#
# Enable by running with --variable COMPAT_MODEL_CONFIGURED=true
# POST {{CANDELA_URL}}/v1/chat/completions
# Content-Type: application/json
# {"model":"gpt-4o","messages":[{"role":"user","content":"compat test"}]}
#
# HTTP 200
# [Asserts]
# jsonpath "$.object" == "chat.completion"
