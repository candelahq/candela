# admin/list_users_page_overflow.hurl
# CRIT-19 regression: crafted page_token beyond maxOffset should return
# 400 InvalidArgument, NOT a 500 or an expensive full-scan.
#
# Run against a live proxy:
#   hurl --variable host=http://localhost:8080 list_users_page_overflow.hurl

POST {{host}}/candela.v1.UserService/ListUsers
Content-Type: application/json
Authorization: Bearer {{admin_token}}
{
  "pagination": {
    "page_size": 50,
    "page_token": "999999999"
  }
}

HTTP 400
[Asserts]
jsonpath "$.code" == "invalid_argument"
jsonpath "$.message" contains "page_token"
