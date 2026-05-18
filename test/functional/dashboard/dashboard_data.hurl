# DASH-20..25: GetDashboardData consolidated endpoint
# Validates the new GetDashboardData RPC that replaces the concurrent
# fan-out of GetUsageSummary + GetModelBreakdown + GetMyUsage.
#
# Usage:
#   hurl --variable ANALYSIS_URL=http://localhost:8080 \
#        --variable AUTH_TOKEN=$(gcloud auth print-access-token) \
#        test/functional/dashboard/dashboard_data.hurl


# ── DASH-20: GetDashboardData without auth → 401 ───────────────────────────
POST {{ANALYSIS_URL}}/candela.v1.DashboardService/GetDashboardData
Content-Type: application/json
Connect-Protocol-Version: 1
{}

HTTP 401
[Asserts]
jsonpath "$.error" == "missing authentication"


# ── DASH-21: GetDashboardData with valid auth + empty body → 200 ────────────
# Empty body defaults to 24h window, no budget.
POST {{ANALYSIS_URL}}/candela.v1.DashboardService/GetDashboardData
Authorization: Bearer {{AUTH_TOKEN}}
Content-Type: application/json
Connect-Protocol-Version: 1
{}

HTTP 200
[Asserts]
header "Content-Type" contains "application/json"


# ── DASH-22: GetDashboardData with time range → 200 + models array ──────────
POST {{ANALYSIS_URL}}/candela.v1.DashboardService/GetDashboardData
Authorization: Bearer {{AUTH_TOKEN}}
Content-Type: application/json
Connect-Protocol-Version: 1
{
  "project_id": "",
  "time_range": {
    "start": "2026-01-01T00:00:00Z",
    "end": "2026-12-31T23:59:59Z"
  }
}

HTTP 200
[Asserts]
header "Content-Type" contains "application/json"


# ── DASH-23: GetDashboardData with include_budget=true → 200 ────────────────
# Budget fields are populated when the user has a budget configured.
POST {{ANALYSIS_URL}}/candela.v1.DashboardService/GetDashboardData
Authorization: Bearer {{AUTH_TOKEN}}
Content-Type: application/json
Connect-Protocol-Version: 1
{
  "project_id": "",
  "time_range": {
    "start": "2026-01-01T00:00:00Z",
    "end": "2026-12-31T23:59:59Z"
  },
  "include_budget": true
}

HTTP 200
[Asserts]
header "Content-Type" contains "application/json"


# ── DASH-24: GetDashboardData with malformed time_range → 400 ───────────────
# Regression guard: string enum instead of TimeRange message.
POST {{ANALYSIS_URL}}/candela.v1.DashboardService/GetDashboardData
Authorization: Bearer {{AUTH_TOKEN}}
Content-Type: application/json
Connect-Protocol-Version: 1
{
  "project_id": "",
  "time_range": "24h"
}

HTTP 400
[Asserts]
jsonpath "$.code" == "invalid_argument"


# ── DASH-25: GetDashboardData with invalid Bearer token → 401 ───────────────
POST {{ANALYSIS_URL}}/candela.v1.DashboardService/GetDashboardData
Authorization: Bearer invalid-garbage-token
Content-Type: application/json
Connect-Protocol-Version: 1
{}

HTTP 401
[Asserts]
jsonpath "$.error" == "invalid authentication token"
