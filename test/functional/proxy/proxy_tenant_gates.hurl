# TENANT-GATE-01..06: Tenant isolation — model-level tenant restrictions
#
# These tests verify that the proxy enforces allowed_tenants on model
# catalog entries. When a model has allowed_tenants set, only requests
# from those tenants are allowed. The proxy returns 403 with error type
# "tenant_access_denied" when the request's tenant is not in the list.
#
# Tenant ID is extracted from (in priority order):
#   1. W3C Baggage: candela.tenant_id=...
#   2. X-Candela-Tenant-Id header
#
# Error shape (403 tenant denied):
# {
#   "error": {
#     "message": "model restricted to tenants: [acme-corp]; request tenant: other-corp",
#     "type": "tenant_access_denied",
#     "code": "403"
#   }
# }
#
# Prerequisites for commented tests:
#   1. Firestore catalog backend with model entries:
#      - openai/gpt-4o:    allowed_tenants: []           (no restriction)
#      - openai/gpt-4.1:   allowed_tenants: ["acme-corp"]
#   2. Authenticated non-admin user
#
# Run:
#   hurl --variable CANDELA_URL=http://localhost:8080 \
#        test/functional/proxy/proxy_tenant_gates.hurl


# ── TENANT-GATE-01: Model with no tenant restriction → any tenant passes ─────
# Models with empty allowed_tenants accept requests from any tenant.
POST {{CANDELA_URL}}/proxy/openai/v1/chat/completions
Authorization: Bearer sk-test
Content-Type: application/json
X-Candela-Tenant-Id: any-tenant
{"model":"gpt-4o","messages":[{"role":"user","content":"no tenant restriction"}]}

HTTP 200
[Asserts]
status == 200


# ── TENANT-GATE-02: Model with no tenant restriction + no tenant header ───────
# Requests without a tenant ID also pass when the model has no restriction.
POST {{CANDELA_URL}}/proxy/openai/v1/chat/completions
Authorization: Bearer sk-test
Content-Type: application/json
{"model":"gpt-4o","messages":[{"role":"user","content":"no tenant header"}]}

HTTP 200
[Asserts]
status == 200


# ── TENANT-GATE-03: Wrong tenant → 403 ───────────────────────────────────────
# Requires: catalog with gpt-4.1 allowed_tenants: ["acme-corp"],
#           authenticated non-admin user
#
# POST {{CANDELA_URL}}/proxy/openai/v1/chat/completions
# Authorization: Bearer user-token
# Content-Type: application/json
# X-Candela-Tenant-Id: other-corp
# {"model":"gpt-4.1","messages":[{"role":"user","content":"wrong tenant test"}]}
#
# HTTP 403
# [Asserts]
# header "Content-Type" contains "application/json"
# jsonpath "$.error.type" == "tenant_access_denied"
# jsonpath "$.error.code" == "403"
# jsonpath "$.error.message" contains "tenants"


# ── TENANT-GATE-04: Correct tenant → 200 ─────────────────────────────────────
# Requires: catalog with gpt-4.1 allowed_tenants: ["acme-corp"]
#
# POST {{CANDELA_URL}}/proxy/openai/v1/chat/completions
# Authorization: Bearer user-token
# Content-Type: application/json
# X-Candela-Tenant-Id: acme-corp
# {"model":"gpt-4.1","messages":[{"role":"user","content":"correct tenant test"}]}
#
# HTTP 200
# [Asserts]
# status == 200


# ── TENANT-GATE-05: Tenant via Baggage header → 403 ──────────────────────────
# Requires: catalog with gpt-4.1 allowed_tenants: ["acme-corp"]
# Verifies that tenant extracted from Baggage is also enforced by the gate.
#
# POST {{CANDELA_URL}}/proxy/openai/v1/chat/completions
# Authorization: Bearer user-token
# Content-Type: application/json
# Baggage: candela.tenant_id=wrong-tenant
# {"model":"gpt-4.1","messages":[{"role":"user","content":"baggage tenant denied"}]}
#
# HTTP 403
# [Asserts]
# jsonpath "$.error.type" == "tenant_access_denied"


# ── TENANT-GATE-06: Admin bypasses tenant gate → 200 ─────────────────────────
# Requires: catalog with gpt-4.1 allowed_tenants: ["acme-corp"],
#           admin user with different tenant
#
# POST {{CANDELA_URL}}/proxy/openai/v1/chat/completions
# Authorization: Bearer admin-token
# Content-Type: application/json
# X-Candela-Tenant-Id: other-corp
# {"model":"gpt-4.1","messages":[{"role":"user","content":"admin bypasses tenant"}]}
#
# HTTP 200
# [Asserts]
# status == 200
