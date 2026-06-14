# HEALTH-01: healthz returns correct JSON format
GET {{CANDELA_URL}}/healthz
HTTP 200
[Asserts]
header "Content-Type" contains "application/json"
jsonpath "$.status" == "ok"

# HEALTH-02: readyz returns correct JSON format
GET {{CANDELA_URL}}/readyz
HTTP 200
[Asserts]
header "Content-Type" contains "application/json"
jsonpath "$.status" == "ready"
