# ACCESS-01..08: Access tag gating — model-level access control
#
# These tests verify that the proxy enforces access_tags on users and
# required_access on model catalog entries. The proxy returns 403 with
# error type "access_denied" when a user lacks the required tags.
#
# Tests requiring pre-provisioned Firestore state (catalog entries with
# required_access + users with access_tags) are documented but commented
# out. Live tests cover the contract shape and unauthenticated behavior.
#
# Error shape (403 access denied):
# {
#   "error": {
#     "message": "model requires access tags: [pro]; user has: []",
#     "type": "access_denied",
#     "code": "403"
#   }
# }
#
# Prerequisites for commented tests:
#   1. Firestore catalog backend with model entries:
#      - openai/gpt-4o:         required_access: []       (open)
#      - openai/gpt-4.1:        required_access: ["pro"]
#      - openai/o3-pro:         required_access: ["enterprise"]
#      - openai/gemma-preview:  required_access: ["experimental"]
#   2. Users provisioned via admin API:
#      - basic-user:    access_tags: []
#      - pro-user:      access_tags: ["pro"]
#      - admin-user:    role: admin
#
# Run:
#   hurl --variable CANDELA_URL=http://localhost:8080 \
#        test/functional/proxy/proxy_access_gates.hurl


# ── ACCESS-01: Open model (no required_access) → always succeeds ─────────────
# Models with empty required_access are accessible to everyone.
# This works with the default config store (no catalog entries = ErrNotFound = skip gate).
POST {{CANDELA_URL}}/proxy/openai/v1/chat/completions
Authorization: Bearer sk-test
Content-Type: application/json
{"model":"gpt-4o","messages":[{"role":"user","content":"open model test"}]}

HTTP 200
[Asserts]
status == 200
jsonpath "$.choices" count == 1


# ── ACCESS-02: Unknown model (not in catalog) → gate skipped ──────────────────
# Models not in the catalog are treated as unmanaged — the gate is skipped.
# This verifies the ErrNotFound → skip behavior.
POST {{CANDELA_URL}}/proxy/openai/v1/chat/completions
Authorization: Bearer sk-test
Content-Type: application/json
{"model":"gpt-unknown-model","messages":[{"role":"user","content":"unknown model"}]}

HTTP 200
[Asserts]
status == 200


# ── ACCESS-03: User without matching tag → 403 ───────────────────────────────
# Requires: catalog with gpt-4.1 required_access: ["pro"],
#           authenticated user with access_tags: [] (or no tags)
#
# POST {{CANDELA_URL}}/proxy/openai/v1/chat/completions
# Authorization: Bearer basic-user-token
# Content-Type: application/json
# {"model":"gpt-4.1","messages":[{"role":"user","content":"access denied test"}]}
#
# HTTP 403
# [Asserts]
# header "Content-Type" contains "application/json"
# jsonpath "$.error.type" == "access_denied"
# jsonpath "$.error.code" == "403"
# jsonpath "$.error.message" contains "access tags"


# ── ACCESS-04: User with matching tag → 200 ──────────────────────────────────
# Requires: catalog with gpt-4.1 required_access: ["pro"],
#           authenticated user with access_tags: ["pro"]
#
# POST {{CANDELA_URL}}/proxy/openai/v1/chat/completions
# Authorization: Bearer pro-user-token
# Content-Type: application/json
# {"model":"gpt-4.1","messages":[{"role":"user","content":"access granted test"}]}
#
# HTTP 200
# [Asserts]
# status == 200
# jsonpath "$.choices" count == 1


# ── ACCESS-05: Admin bypasses access gate → 200 ──────────────────────────────
# Requires: catalog with o3-pro required_access: ["enterprise"],
#           admin user (role=admin, no enterprise tag needed)
#
# POST {{CANDELA_URL}}/proxy/openai/v1/chat/completions
# Authorization: Bearer admin-user-token
# Content-Type: application/json
# {"model":"o3-pro","messages":[{"role":"user","content":"admin bypass test"}]}
#
# HTTP 200
# [Asserts]
# status == 200
# jsonpath "$.choices" count == 1


# ── ACCESS-06: SA without UserRecord → 403 on gated model ────────────────────
# Requires: catalog with gpt-4.1 required_access: ["pro"],
#           SA email on allowlist but no UserRecord in Firestore
#
# POST {{CANDELA_URL}}/proxy/openai/v1/chat/completions
# Authorization: Bearer sa-no-record-token
# Content-Type: application/json
# {"model":"gpt-4.1","messages":[{"role":"user","content":"sa no record"}]}
#
# HTTP 403
# [Asserts]
# jsonpath "$.error.type" == "access_denied"


# ── ACCESS-07: SA with UserRecord + tags → 200 ───────────────────────────────
# Requires: catalog with gpt-4.1 required_access: ["pro"],
#           SA with UserRecord containing access_tags: ["pro"]
#
# POST {{CANDELA_URL}}/proxy/openai/v1/chat/completions
# Authorization: Bearer sa-with-tags-token
# Content-Type: application/json
# {"model":"gpt-4.1","messages":[{"role":"user","content":"sa with tags"}]}
#
# HTTP 200
# [Asserts]
# status == 200


# ── ACCESS-08: Catalog lookup error → 500 (fail-closed) ──────────────────────
# When the catalog store returns a transient error (not ErrNotFound),
# the proxy must return 500, NOT silently pass through.
# This is difficult to trigger in E2E — validated via unit tests in
# pkg/proxy/access_gates_test.go instead.
#
# Expected error shape:
# {
#   "error": {
#     "message": "catalog lookup failed",
#     "type": "internal",
#     "code": "500"
#   }
# }
