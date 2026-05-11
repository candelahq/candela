# proxy/proxy_inactive_user_blocked.hurl
# CRIT-15 regression: a deactivated user must be blocked at the proxy gate
# even if they have remaining grant balance.
#
# Prerequisites (set up via admin API before running):
#   1. Create a user with a grant
#   2. Deactivate the user
#   3. Attempt a proxy call — expect 402 Payment Required

POST {{host}}/v1/messages
Content-Type: application/json
X-Candela-User-ID: {{inactive_user_id}}
Authorization: Bearer {{api_key}}
{
  "model": "claude-sonnet-4-20250514",
  "max_tokens": 10,
  "messages": [{"role": "user", "content": "hello"}]
}

HTTP 402
[Asserts]
# Body should indicate budget/access denial, not a provider error.
jsonpath "$.error.type" exists
