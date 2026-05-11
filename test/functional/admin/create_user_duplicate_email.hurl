# admin/create_user_duplicate_email.hurl
# Verifies that creating a user with an already-registered email returns
# 409 AlreadyExists rather than 500 or silent overwrite.

POST {{host}}/candela.v1.UserService/CreateUser
Content-Type: application/json
Authorization: Bearer {{admin_token}}
{
  "email": "duplicate@example.com",
  "display_name": "First User"
}

HTTP 200

# Second create with same email should be rejected.
POST {{host}}/candela.v1.UserService/CreateUser
Content-Type: application/json
Authorization: Bearer {{admin_token}}
{
  "email": "duplicate@example.com",
  "display_name": "Second User (duplicate)"
}

HTTP 409
[Asserts]
jsonpath "$.code" == "already_exists"
