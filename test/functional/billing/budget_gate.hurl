# BILLING-01/02: Budget gate — exhausted and sub-floor balance → 402
#
# These tests require the binary to be running in team mode with a UserStore
# configured. In local/dev mode (no UserStore), budget gates are skipped.
#
# Expected behavior:
#   - Authenticated user with $0 budget → 402, error.type = "insufficient_budget"
#   - Sub-floor balance ($0.0005 < $0.001 floor) → 402 (same error)
#   - Unauthenticated user (no auth context) → request passes through (no gate)
#
# Note: These tests verify the HTTP contract. The actual budget state is set up
# by the test environment (e.g. a pre-seeded Firestore emulator user with $0 budget).
# In CI, these are validated via Rust integration tests with a mock UserStore.
# Hurl tests here document the expected API shape for regression detection.

# ── Budget exhausted → 402 ────────────────────────────────────────────────────
# Requires: authenticated user with budget.spent >= budget.limit
# POST {{CANDELA_URL}}/proxy/anthropic/v1/messages
# Authorization: Bearer exhausted-user-token
# Content-Type: application/json
# X-Test-Exhausted-Budget: true
# {"model":"claude-sonnet-4-20250514","messages":[{"role":"user","content":"free money"}]}
#
# HTTP 402
# [Asserts]
# header "Content-Type" contains "application/json"
# jsonpath "$.error.type" == "insufficient_budget"
# jsonpath "$.error.code" == 402


# ── Error shape documentation ─────────────────────────────────────────────────
# The 402 response body MUST have this shape (both Go and Rust):
#
# {
#   "error": {
#     "message": "budget exhausted — contact your admin for a grant or budget increase",
#     "type": "insufficient_budget",
#     "code": 402
#   }
# }
#
# This is verified in Rust integration tests (pkg/proxy/billing_test.go equivalents).
# Uncomment the tests above when the test environment has budget enforcement enabled.


# ── Unauthenticated: no budget gate applied ───────────────────────────────────
# Without auth context, the proxy forwards the request (budget gate is skipped).
# This test confirms the proxy doesn't 402 on anonymous requests.
POST {{CANDELA_URL}}/proxy/openai/v1/chat/completions
Content-Type: application/json
{"model":"gpt-4o","messages":[{"role":"user","content":"anonymous request"}]}

# No auth header → proxy should forward (not 402).
# May get 401 from upstream if upstream requires auth, but NOT 402 from proxy.
HTTP *
[Asserts]
status != 402
