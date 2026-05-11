# billing/discount_zero_cost.hurl
# CRIT-17 documentation test: a model configured with discount_percent=1.0
# should result in $0 billed (allowed through, but cost = $0.00).
# This test verifies the request is ALLOWED (not blocked), not that billing is $0
# (which requires inspecting internal state).
#
# Note: requires the proxy to be configured with a pricing override:
#   models:
#     - provider: openai
#       model: gpt-4o-mini
#       discount_percent: 1.0

POST {{host}}/v1/chat/completions
Content-Type: application/json
X-Candela-User-ID: {{user_id}}
Authorization: Bearer {{api_key}}
{
  "model": "gpt-4o-mini",
  "max_tokens": 5,
  "messages": [{"role": "user", "content": "hi"}]
}

HTTP 200
[Asserts]
# Request passes through — discount model is not blocked by pricing gate.
jsonpath "$.choices" count > 0
