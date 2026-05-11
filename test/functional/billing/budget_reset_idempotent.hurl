# billing/budget_reset_idempotent.hurl
# Verifies that calling ResetSpend twice leaves the user's spend at $0
# and returns a 200 both times (idempotent operation).

POST {{host}}/candela.v1.UserService/ResetSpend
Content-Type: application/json
Authorization: Bearer {{admin_token}}
{
  "user_id": "{{user_id}}"
}

HTTP 200
[Asserts]
jsonpath "$.budget.spent_usd" == 0.0

# Second reset should also succeed (idempotent).
POST {{host}}/candela.v1.UserService/ResetSpend
Content-Type: application/json
Authorization: Bearer {{admin_token}}
{
  "user_id": "{{user_id}}"
}

HTTP 200
[Asserts]
jsonpath "$.budget.spent_usd" == 0.0
