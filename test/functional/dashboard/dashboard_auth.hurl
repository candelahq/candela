# DASH-01..06: Dashboard ConnectRPC auth and endpoint validation
# These tests verify the candela-analysis DashboardService endpoints
# used by Candela Desktop in Team mode.
#
# NOTE: Auth failures return plain HTTP JSON from the auth middleware
# (pkg/auth/{firebase,iap}.go writeError), NOT ConnectRPC error format.
# The middleware uses {"error": "..."}, while ConnectRPC uses {"message": "..."}.
# These assertions use $.error because auth is rejected BEFORE reaching ConnectRPC.

# ── DASH-01: GetModelBreakdown without auth → 401 ──────────────────────────
POST {{ANALYSIS_URL}}/candela.v1.DashboardService/GetModelBreakdown
Content-Type: application/json
Connect-Protocol-Version: 1
{}

HTTP 401
[Asserts]
jsonpath "$.error" == "missing authentication"


# ── DASH-02: GetUsageSummary without auth → 401 ────────────────────────────
POST {{ANALYSIS_URL}}/candela.v1.DashboardService/GetUsageSummary
Content-Type: application/json
Connect-Protocol-Version: 1
{}

HTTP 401
[Asserts]
jsonpath "$.error" == "missing authentication"


# ── DASH-03: GetMyUsage without auth → 401 ─────────────────────────────────
POST {{ANALYSIS_URL}}/candela.v1.DashboardService/GetMyUsage
Content-Type: application/json
Connect-Protocol-Version: 1
{}

HTTP 401
[Asserts]
jsonpath "$.error" == "missing authentication"


# ── DASH-04: Invalid Bearer token → 401 ────────────────────────────────────
POST {{ANALYSIS_URL}}/candela.v1.DashboardService/GetModelBreakdown
Authorization: Bearer invalid-garbage-token
Content-Type: application/json
Connect-Protocol-Version: 1
{}

HTTP 401
[Asserts]
jsonpath "$.error" == "invalid authentication token"


# ── DASH-05: Malformed Authorization header → 401 ──────────────────────────
POST {{ANALYSIS_URL}}/candela.v1.DashboardService/GetModelBreakdown
Authorization: Basic dXNlcjpwYXNz
Content-Type: application/json
Connect-Protocol-Version: 1
{}

HTTP 401
[Asserts]
jsonpath "$.error" == "missing authentication"


# ── DASH-06: Health check bypasses auth ─────────────────────────────────────
GET {{ANALYSIS_URL}}/healthz

HTTP 200
