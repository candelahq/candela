# FORECAST-01: budget-forecast endpoint — unauthenticated → 401
#
# Requires: candela-server running with a UserStore configured.
# The budget-forecast endpoint requires authentication.
#
# In CI, this test validates the HTTP contract shape.
# Manually: start server with `CANDELA_MODE=team candela-server`

# ── Unauthenticated → 401 ────────────────────────────────────────────────
# No Authorization header → should return 401.
# GET {{CANDELA_URL}}/api/v1/users/test-user/budget-forecast
#
# HTTP 401
# [Asserts]
# body contains "authentication required"

# ── Method not allowed → 405 ─────────────────────────────────────────────
# POST method → should return 405 Method Not Allowed.
# POST {{CANDELA_URL}}/api/v1/users/test-user/budget-forecast
# Content-Type: application/json
# {}
#
# HTTP 405

# ── Missing userID → 400 ─────────────────────────────────────────────────
# Missing userID segment → 400 Bad Request.
# GET {{CANDELA_URL}}/api/v1/users//budget-forecast
# Authorization: Bearer {{AUTH_TOKEN}}
#
# HTTP 400
# [Asserts]
# body contains "missing user_id"

# ── Authenticated self-service → 200 ─────────────────────────────────────
# Authenticated user requesting own forecast.
# GET {{CANDELA_URL}}/api/v1/users/{{USER_ID}}/budget-forecast
# Authorization: Bearer {{AUTH_TOKEN}}
#
# HTTP 200
# [Asserts]
# header "Content-Type" contains "application/json"
# jsonpath "$.burn_rate_usd_per_hour" >= 0
# jsonpath "$.projected_eod_spend_usd" >= 0
# jsonpath "$.days_until_exhaustion" >= -1
# jsonpath "$.spend_history" isCollection
# jsonpath "$.will_exceed_budget" isBoolean

# ── Non-admin accessing other user → 404 ─────────────────────────────────
# Regular user trying to access another user's forecast.
# GET {{CANDELA_URL}}/api/v1/users/other-user-id/budget-forecast
# Authorization: Bearer {{AUTH_TOKEN}}
#
# HTTP 404
# [Asserts]
# body contains "not found"
